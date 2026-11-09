package handler

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/telegram-storage-api/internal/model"
	"github.com/telegram-storage-api/internal/service"
)

type FileHandler struct {
	fileService *service.FileService
}

func NewFileHandler(fileService *service.FileService) *FileHandler {
	return &FileHandler{fileService: fileService}
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

	tmpDir := os.TempDir()
	tmpPath := filepath.Join(tmpDir, fmt.Sprintf("tg_upload_%d_%s", userID, file.Filename))
	if err := c.SaveUploadedFile(file, tmpPath); err != nil {
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
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", file.FileName))

	if _, err := h.fileService.StreamFilePublic(c.Request.Context(), fileID, c.Writer); err != nil {
		c.JSON(http.StatusInternalServerError, model.APIResponse{
			Success: false,
			Message: err.Error(),
		})
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
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", file.FileName))

	if _, err := h.fileService.StreamFilePublic(c.Request.Context(), fileID, c.Writer); err != nil {
		c.JSON(http.StatusInternalServerError, model.APIResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}
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
