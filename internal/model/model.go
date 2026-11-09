package model

import "time"

type User struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Password  string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

type File struct {
	ID              int64     `json:"id"`
	UserID          *int64    `json:"user_id"`
	FileName        string    `json:"file_name"`
	FileSize        int64     `json:"file_size"`
	MimeType        string    `json:"mime_type"`
	TelegramMsgID   int       `json:"telegram_msg_id"`
	TelegramFileRef string    `json:"-"`
	CreatedAt       time.Time `json:"created_at"`
}

type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3"`
	Password string `json:"password" binding:"required,min=6"`
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

type FileResponse struct {
	File      File   `json:"file"`
	StreamURL string `json:"stream_url"`
	DownloadURL string `json:"download_url"`
}

type ListFilesResponse struct {
	Files []File `json:"files"`
	Total int    `json:"total"`
}

type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}
