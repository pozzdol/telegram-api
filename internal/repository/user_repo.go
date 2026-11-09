package repository

import (
	"database/sql"

	"github.com/telegram-storage-api/internal/model"
)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(username, hashedPassword string) (*model.User, error) {
	var user model.User
	err := r.db.QueryRow(
		`INSERT INTO users (username, password) VALUES ($1, $2)
		 RETURNING id, username, password, created_at`,
		username, hashedPassword,
	).Scan(&user.ID, &user.Username, &user.Password, &user.CreatedAt)
	return &user, err
}

func (r *UserRepository) FindByUsername(username string) (*model.User, error) {
	var user model.User
	err := r.db.QueryRow(
		`SELECT id, username, password, created_at FROM users WHERE username = $1`,
		username,
	).Scan(&user.ID, &user.Username, &user.Password, &user.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &user, err
}
