package repository

import (
	"database/sql"

	"github.com/telegram-storage-api/internal/model"
)

type FileRepository struct {
	db *sql.DB
}

func NewFileRepository(db *sql.DB) *FileRepository {
	return &FileRepository{db: db}
}

func (r *FileRepository) Create(file *model.File) (*model.File, error) {
	err := r.db.QueryRow(
		`INSERT INTO files (user_id, file_name, file_size, mime_type, telegram_msg_id, telegram_file_ref)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, created_at`,
		file.UserID, file.FileName, file.FileSize, file.MimeType,
		file.TelegramMsgID, file.TelegramFileRef,
	).Scan(&file.ID, &file.CreatedAt)
	return file, err
}

func (r *FileRepository) FindByIDAndUser(id, userID int64) (*model.File, error) {
	var file model.File
	err := r.db.QueryRow(
		`SELECT id, user_id, file_name, file_size, mime_type, telegram_msg_id, telegram_file_ref, created_at
		 FROM files WHERE id = $1 AND user_id = $2`,
		id, userID,
	).Scan(&file.ID, &file.UserID, &file.FileName, &file.FileSize, &file.MimeType,
		&file.TelegramMsgID, &file.TelegramFileRef, &file.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &file, err
}

func (r *FileRepository) ListByUser(userID int64, limit, offset int) ([]model.File, int, error) {
	var total int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM files WHERE user_id = $1`, userID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.db.Query(
		`SELECT id, user_id, file_name, file_size, mime_type, telegram_msg_id, telegram_file_ref, created_at
		 FROM files WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var files []model.File
	for rows.Next() {
		var f model.File
		if err := rows.Scan(&f.ID, &f.UserID, &f.FileName, &f.FileSize, &f.MimeType,
			&f.TelegramMsgID, &f.TelegramFileRef, &f.CreatedAt); err != nil {
			return nil, 0, err
		}
		files = append(files, f)
	}
	return files, total, nil
}

func (r *FileRepository) Delete(id, userID int64) error {
	_, err := r.db.Exec(`DELETE FROM files WHERE id = $1 AND (user_id = $2 OR user_id IS NULL)`, id, userID)
	return err
}

func (r *FileRepository) DeleteByID(id int64) error {
	_, err := r.db.Exec(`DELETE FROM files WHERE id = $1`, id)
	return err
}

func (r *FileRepository) ListAll(limit, offset int) ([]model.File, int, error) {
	var total int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM files`).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.db.Query(
		`SELECT id, user_id, file_name, file_size, mime_type, telegram_msg_id, telegram_file_ref, created_at
		 FROM files ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var files []model.File
	for rows.Next() {
		var f model.File
		if err := rows.Scan(&f.ID, &f.UserID, &f.FileName, &f.FileSize, &f.MimeType,
			&f.TelegramMsgID, &f.TelegramFileRef, &f.CreatedAt); err != nil {
			return nil, 0, err
		}
		files = append(files, f)
	}
	return files, total, nil
}

func (r *FileRepository) ExistsByMsgID(msgID int) (bool, error) {
	var exists bool
	err := r.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM files WHERE telegram_msg_id = $1)`, msgID).Scan(&exists)
	return exists, err
}

func (r *FileRepository) LastMsgID() (int, error) {
	var msgID int
	err := r.db.QueryRow(`SELECT COALESCE(MAX(telegram_msg_id), 0) FROM files`).Scan(&msgID)
	return msgID, err
}

func (r *FileRepository) FindByID(id int64) (*model.File, error) {
	var file model.File
	err := r.db.QueryRow(
		`SELECT id, user_id, file_name, file_size, mime_type, telegram_msg_id, telegram_file_ref, created_at
		 FROM files WHERE id = $1`, id,
	).Scan(&file.ID, &file.UserID, &file.FileName, &file.FileSize, &file.MimeType,
		&file.TelegramMsgID, &file.TelegramFileRef, &file.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &file, err
}
