package handler

import (
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/telegram-storage-api/internal/model"
	"github.com/telegram-storage-api/internal/service"
	"github.com/telegram-storage-api/internal/sheets"
)

type SheetWriter interface {
	Upsert(ctx context.Context, row sheets.Row) error
}

type FileHandler struct {
	fileService *service.FileService
	sheets      SheetWriter
}

// sheetWriter may be nil; POST /api/files/upload-video then responds 503.
func NewFileHandler(fileService *service.FileService, sheetWriter SheetWriter) *FileHandler {
	return &FileHandler{fileService: fileService, sheets: sheetWriter}
}

func saveTempUpload(c *gin.Context, userID int64, file *multipart.FileHeader) (string, error) {
	tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("tg_upload_%d_%s", userID, file.Filename))
	if err := c.SaveUploadedFile(file, tmpPath); err != nil {
		return "", err
	}
	return tmpPath, nil
}

func (h *FileHandler) Upload(c *gin.Context) {
	userID := c.GetInt64("user_id")

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, model.APIResponse{
			Success: false,
			Message: "file is required",
		})
		return
	}

	tmpPath, err := saveTempUpload(c, userID, file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.APIResponse{
			Success: false,
			Message: "failed to save uploaded file",
		})
		return
	}
	defer os.Remove(tmpPath)

	result, err := h.fileService.Upload(c.Request.Context(), userID, tmpPath, file.Filename, file.Size)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.APIResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, model.APIResponse{
		Success: true,
		Message: "file uploaded",
		Data: model.FileResponse{
			File:        *result,
			StreamURL:   fmt.Sprintf("/api/files/%d/stream", result.ID),
			DownloadURL: fmt.Sprintf("/api/files/%d/download", result.ID),
		},
	})
}

const (
	maxTitleRunes   = 100  // YouTube title limit
	maxCaptionRunes = 2200 // Instagram caption limit; caption = judul + deskripsi + hashtag
)

func validateVideoMetadata(fileName, title, description, hashtag string) string {
	if !strings.HasPrefix(service.DetectMimeType(fileName), "video/") {
		return "file must be a video (.mp4, .mov, .m4v, .webm, .mkv)"
	}
	if title == "" {
		return "judul is required"
	}
	if utf8.RuneCountInString(title) > maxTitleRunes {
		return fmt.Sprintf("judul must be at most %d characters", maxTitleRunes)
	}
	if strings.ContainsAny(title, "<>") {
		return "judul must not contain < or >"
	}
	parts := []string{title}
	for _, p := range []string{description, hashtag} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if n := utf8.RuneCountInString(strings.Join(parts, "\n\n")); n > maxCaptionRunes {
		return fmt.Sprintf("judul + deskripsi + hashtag must be at most %d characters (got %d)", maxCaptionRunes, n)
	}
	return ""
}

// UploadVideo stores a video in Telegram and upserts its metadata row in the Google Sheet.
func (h *FileHandler) UploadVideo(c *gin.Context) {
	if h.sheets == nil {
		c.JSON(http.StatusServiceUnavailable, model.APIResponse{
			Success: false,
			Message: "google sheet is not configured on the server",
		})
		return
	}

	userID := c.GetInt64("user_id")

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, model.APIResponse{Success: false, Message: "file is required"})
		return
	}

	title := strings.TrimSpace(c.PostForm("judul"))
	description := strings.TrimSpace(c.PostForm("deskripsi"))
	hashtag := strings.TrimSpace(c.PostForm("hashtag"))

	if msg := validateVideoMetadata(file.Filename, title, description, hashtag); msg != "" {
		c.JSON(http.StatusBadRequest, model.APIResponse{Success: false, Message: msg})
		return
	}

	// The sheet is keyed by file_name, so a duplicate name would share (and overwrite) metadata.
	exists, err := h.fileService.FileNameExists(file.Filename)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.APIResponse{Success: false, Message: err.Error()})
		return
	}
	if exists {
		c.JSON(http.StatusConflict, model.APIResponse{
			Success: false,
			Message: fmt.Sprintf("a file named %q already exists; rename the video and try again", file.Filename),
		})
		return
	}

	tmpPath, err := saveTempUpload(c, userID, file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.APIResponse{Success: false, Message: "failed to save uploaded file"})
		return
	}
	defer os.Remove(tmpPath)

	result, err := h.fileService.Upload(c.Request.Context(), userID, tmpPath, file.Filename, file.Size)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.APIResponse{Success: false, Message: err.Error()})
		return
	}

	resp := model.UploadVideoResponse{
		FileResponse: model.FileResponse{
			File:        *result,
			StreamURL:   fmt.Sprintf("/api/public/files/%d/stream", result.ID),
			DownloadURL: fmt.Sprintf("/api/public/files/%d/download", result.ID),
		},
		Title:       title,
		Description: description,
		Hashtag:     hashtag,
	}

	// Detached from the request: the video is already stored, so the row must still be written
	// even if the client disconnects.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err = h.sheets.Upsert(ctx, sheets.Row{FileName: result.FileName, Title: title, Description: description, Hashtag: hashtag})

	message := "video uploaded and sheet updated"
	if err != nil {
		// 201 even on sheet failure: retrying the request would upload the video twice.
		resp.SheetError = err.Error()
		message = "video uploaded, but updating the sheet failed; add the row manually"
	} else {
		resp.SheetSynced = true
	}

	c.JSON(http.StatusCreated, model.APIResponse{Success: true, Message: message, Data: resp})
}

func (h *FileHandler) List(c *gin.Context) {
	userID := c.GetInt64("user_id")

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	result, err := h.fileService.List(userID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.APIResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, model.APIResponse{
		Success: true,
		Data:    result,
	})
}

func (h *FileHandler) Stream(c *gin.Context) {
	userID := c.GetInt64("user_id")
	fileID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, model.APIResponse{
			Success: false,
			Message: "invalid file id",
		})
		return
	}

	file, err := h.fileService.GetFile(userID, fileID)
	if err != nil {
		c.JSON(http.StatusNotFound, model.APIResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	c.Header("Content-Type", file.MimeType)
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", file.FileName))

	if _, err := h.fileService.StreamFile(c.Request.Context(), userID, fileID, c.Writer); err != nil {
		c.JSON(http.StatusInternalServerError, model.APIResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}
}

func (h *FileHandler) Download(c *gin.Context) {
	userID := c.GetInt64("user_id")
	fileID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, model.APIResponse{
			Success: false,
			Message: "invalid file id",
		})
		return
	}

	file, err := h.fileService.GetFile(userID, fileID)
	if err != nil {
		c.JSON(http.StatusNotFound, model.APIResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", file.FileName))

	if _, err := h.fileService.StreamFile(c.Request.Context(), userID, fileID, c.Writer); err != nil {
		c.JSON(http.StatusInternalServerError, model.APIResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}
}

func (h *FileHandler) PublicList(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	result, err := h.fileService.ListAll(limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.APIResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, model.APIResponse{
		Success: true,
		Data:    result,
	})
}

func (h *FileHandler) PublicStream(c *gin.Context) {
	fileID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, model.APIResponse{
			Success: false,
			Message: "invalid file id",
		})
		return
	}

	file, err := h.fileService.GetFileByID(fileID)
	if err != nil {
		c.JSON(http.StatusNotFound, model.APIResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	c.Header("Content-Type", file.MimeType)
	h.servePublicFile(c, file, "inline")
}

func (h *FileHandler) servePublicFile(c *gin.Context, file *model.File, disposition string) {
	c.Header("Content-Disposition", fmt.Sprintf("%s; filename=\"%s\"", disposition, file.FileName))
	// Synced photos store the largest variant's size but download the "y" thumbnail, so their size is unreliable.
	if file.FileSize > 0 && !strings.HasPrefix(file.MimeType, "image/") {
		c.Header("Content-Length", strconv.FormatInt(file.FileSize, 10))
	}

	if c.Request.Method == http.MethodHead {
		c.Status(http.StatusOK)
		return
	}

	if _, err := h.fileService.StreamFilePublic(c.Request.Context(), file.ID, c.Writer); err != nil {
		if !c.Writer.Written() {
			c.Writer.Header().Del("Content-Length")
			c.JSON(http.StatusInternalServerError, model.APIResponse{
				Success: false,
				Message: err.Error(),
			})
		}
		return
	}
}

func (h *FileHandler) PublicDownload(c *gin.Context) {
	fileID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, model.APIResponse{
			Success: false,
			Message: "invalid file id",
		})
		return
	}

	file, err := h.fileService.GetFileByID(fileID)
	if err != nil {
		c.JSON(http.StatusNotFound, model.APIResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	c.Header("Content-Type", "application/octet-stream")
	h.servePublicFile(c, file, "attachment")
}

func (h *FileHandler) Delete(c *gin.Context) {
	userID := c.GetInt64("user_id")
	fileID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, model.APIResponse{
			Success: false,
			Message: "invalid file id",
		})
		return
	}

	if err := h.fileService.Delete(c.Request.Context(), userID, fileID); err != nil {
		c.JSON(http.StatusInternalServerError, model.APIResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, model.APIResponse{
		Success: true,
		Message: "file deleted",
	})
}
