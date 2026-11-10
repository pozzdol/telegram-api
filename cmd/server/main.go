package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/telegram-storage-api/internal/config"
	"github.com/telegram-storage-api/internal/database"
	"github.com/telegram-storage-api/internal/handler"
	"github.com/telegram-storage-api/internal/middleware"
	"github.com/telegram-storage-api/internal/model"
	"github.com/telegram-storage-api/internal/repository"
	"github.com/telegram-storage-api/internal/service"
	"github.com/telegram-storage-api/internal/sheets"
	tgclient "github.com/telegram-storage-api/internal/telegram"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load config:", err)
	}

	db, err := database.Connect(cfg.DatabaseURL())
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	if err := database.Migrate(db); err != nil {
		log.Fatal("Failed to run migrations:", err)
	}
	log.Println("Database migrated successfully")

	tg := tgclient.NewClient(cfg.TelegramAPIID, cfg.TelegramAPIHash, cfg.TelegramChannelID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := tg.Start(ctx, cfg.TelegramPhone); err != nil {
			log.Println("Telegram client error:", err)
		}
	}()

	// Wait for Telegram client to be ready
	for i := 0; i < 30; i++ {
		if tg.IsReady() {
			break
		}
		time.Sleep(time.Second)
	}
	if !tg.IsReady() {
		log.Println("WARNING: Telegram client not ready after 30s, continuing anyway...")
	}

	userRepo := repository.NewUserRepository(db)
	fileRepo := repository.NewFileRepository(db)

	authService := service.NewAuthService(userRepo, cfg.JWTSecret)
	fileService := service.NewFileService(fileRepo, tg)

	var sheetWriter handler.SheetWriter
	if cfg.GoogleServiceAccountFile != "" && cfg.GoogleSheetID != "" {
		sheetClient, err := sheets.New(ctx, cfg.GoogleServiceAccountFile, cfg.GoogleSheetID, cfg.GoogleSheetName)
		if err != nil {
			log.Fatal("Failed to init Google Sheets client:", err)
		}
		sheetWriter = sheetClient
		log.Printf("Google Sheet metadata enabled (sheet %q)", cfg.GoogleSheetName)
	} else {
		log.Println("Google Sheet metadata disabled: GOOGLE_SERVICE_ACCOUNT_FILE / GOOGLE_SHEET_ID not set")
	}

	authHandler := handler.NewAuthHandler(authService)
	fileHandler := handler.NewFileHandler(fileService, sheetWriter)

	// Start background sync: auto-detect files uploaded directly to Telegram
	if tg.IsReady() {
		lastMsgID, _ := fileRepo.LastMsgID()
		go tg.StartSync(ctx, 10*time.Second, lastMsgID, func(f tgclient.SyncedFile) error {
			exists, err := fileRepo.ExistsByMsgID(f.MsgID)
			if err != nil {
				return err
			}
			if exists {
				return nil
			}
			_, err = fileRepo.Create(&model.File{
				UserID:        nil,
				FileName:      f.FileName,
				FileSize:      f.FileSize,
				MimeType:      f.MimeType,
				TelegramMsgID: f.MsgID,
			})
			return err
		})
	}

	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":         "ok",
			"telegram_ready": tg.IsReady(),
		})
	})

	auth := r.Group("/api/auth")
	{
		auth.POST("/register", authHandler.Register)
		auth.POST("/login", authHandler.Login)
	}

	// Public endpoints (no auth) — includes synced files from Telegram
	public := r.Group("/api/public")
	{
		public.GET("/files", fileHandler.PublicList)
		public.GET("/files/:id/stream", fileHandler.PublicStream)
		public.HEAD("/files/:id/stream", fileHandler.PublicStream)
		public.GET("/files/:id/download", fileHandler.PublicDownload)
		public.HEAD("/files/:id/download", fileHandler.PublicDownload)
	}

	// Authenticated endpoints
	files := r.Group("/api/files")
	files.Use(middleware.JWTAuth(cfg.JWTSecret))
	{
		files.POST("/upload", fileHandler.Upload)
		files.POST("/upload-video", fileHandler.UploadVideo)
		files.GET("", fileHandler.List)
		files.GET("/:id/stream", fileHandler.Stream)
		files.GET("/:id/download", fileHandler.Download)
		files.DELETE("/:id", fileHandler.Delete)
	}

	srv := &http.Server{
		Addr:    ":" + cfg.ServerPort,
		Handler: r,
	}

	go func() {
		log.Printf("Server starting on port %s", cfg.ServerPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Server error:", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	srv.Shutdown(shutdownCtx)
}
