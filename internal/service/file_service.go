package service

import (
	"context"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"strings"

	"github.com/telegram-storage-api/internal/model"
	"github.com/telegram-storage-api/internal/repository"
	tgclient "github.com/telegram-storage-api/internal/telegram"
)

type FileService struct {
	fileRepo *repository.FileRepository
	tg       *tgclient.Client
}

func NewFileService(fileRepo *repository.FileRepository, tg *tgclient.Client) *FileService {
	return &FileService{fileRepo: fileRepo, tg: tg}
}

func (s *FileService) Upload(ctx context.Context, userID int64, filePath, originalName string, fileSize int64) (*model.File, error) {
	msgID, err := s.tg.UploadFile(ctx, filePath)
	if err != nil {
		return nil, fmt.Errorf("telegram upload: %w", err)
	}

	mimeType := DetectMimeType(originalName)

	file := &model.File{
		UserID:        &userID,
		FileName:      originalName,
		FileSize:      fileSize,
		MimeType:      mimeType,
		TelegramMsgID: msgID,
	}

	return s.fileRepo.Create(file)
}

// Go's built-in MIME table has no video types; without /etc/mime.types (e.g. slim containers)
// videos would be stored as application/octet-stream and skipped by the upload workflow.
var videoMimeTypes = map[string]string{
	".mp4":  "video/mp4",
	".m4v":  "video/x-m4v",
	".mov":  "video/quicktime",
	".webm": "video/webm",
	".mkv":  "video/x-matroska",
}

func DetectMimeType(fileName string) string {
	ext := strings.ToLower(filepath.Ext(fileName))
	if t := mime.TypeByExtension(ext); t != "" {
		return t
	}
	if t, ok := videoMimeTypes[ext]; ok {
		return t
	}
	return "application/octet-stream"
}

func (s *FileService) FileNameExists(fileName string) (bool, error) {
	return s.fileRepo.ExistsByFileName(fileName)
}

func (s *FileService) List(userID int64, limit, offset int) (*model.ListFilesResponse, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	files, total, err := s.fileRepo.ListByUser(userID, limit, offset)
	if err != nil {
		return nil, err
	}

	if files == nil {
		files = []model.File{}
	}

	return &model.ListFilesResponse{Files: files, Total: total}, nil
}

func (s *FileService) GetFile(userID, fileID int64) (*model.File, error) {
	file, err := s.fileRepo.FindByIDAndUser(fileID, userID)
	if err != nil {
		return nil, err
	}
	if file == nil {
		file, err = s.fileRepo.FindByID(fileID)
		if err != nil {
			return nil, err
		}
	}
	if file == nil {
		return nil, fmt.Errorf("file not found")
	}
	return file, nil
}

func (s *FileService) StreamFile(ctx context.Context, userID, fileID int64, w io.Writer) (*model.File, error) {
	file, err := s.GetFile(userID, fileID)
	if err != nil {
		return nil, err
	}

	if err := s.tg.DownloadFile(ctx, file.TelegramMsgID, w); err != nil {
		return nil, fmt.Errorf("download from telegram: %w", err)
	}

	return file, nil
}

func (s *FileService) Delete(ctx context.Context, userID, fileID int64) error {
	file, err := s.GetFile(userID, fileID)
	if err != nil {
		return err
	}

	if err := s.tg.DeleteMessage(ctx, file.TelegramMsgID); err != nil {
		return fmt.Errorf("delete from telegram: %w", err)
	}

	return s.fileRepo.Delete(fileID, userID)
}

func (s *FileService) ListAll(limit, offset int) (*model.ListFilesResponse, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	files, total, err := s.fileRepo.ListAll(limit, offset)
	if err != nil {
		return nil, err
	}

	if files == nil {
		files = []model.File{}
	}

	return &model.ListFilesResponse{Files: files, Total: total}, nil
}

func (s *FileService) GetFileByID(fileID int64) (*model.File, error) {
	file, err := s.fileRepo.FindByID(fileID)
	if err != nil {
		return nil, err
	}
	if file == nil {
		return nil, fmt.Errorf("file not found")
	}
	return file, nil
}

func (s *FileService) StreamFilePublic(ctx context.Context, fileID int64, w io.Writer) (*model.File, error) {
	file, err := s.GetFileByID(fileID)
	if err != nil {
		return nil, err
	}

	if err := s.tg.DownloadFile(ctx, file.TelegramMsgID, w); err != nil {
		return nil, fmt.Errorf("download from telegram: %w", err)
	}

	return file, nil
}
