package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	ServerPort        string
	JWTSecret         string
	DBHost            string
	DBPort            string
	DBUser            string
	DBPassword        string
	DBName            string
	DBSSLMode         string
	TelegramAPIID     int
	TelegramAPIHash   string
	TelegramPhone     string
	TelegramChannelID int64
}

func Load() (*Config, error) {
	godotenv.Load()

	apiID, err := strconv.Atoi(getEnv("TELEGRAM_API_ID", "0"))
	if err != nil {
		return nil, err
	}

	channelID, err := strconv.ParseInt(getEnv("TELEGRAM_CHANNEL_ID", "0"), 10, 64)
	if err != nil {
		return nil, err
	}

	return &Config{
		ServerPort:        getEnv("SERVER_PORT", "8080"),
		JWTSecret:         getEnv("JWT_SECRET", "change-me"),
		DBHost:            getEnv("DB_HOST", "localhost"),
		DBPort:            getEnv("DB_PORT", "5432"),
		DBUser:            getEnv("DB_USER", "postgres"),
		DBPassword:        getEnv("DB_PASSWORD", "postgres"),
		DBName:            getEnv("DB_NAME", "telegram_storage"),
		DBSSLMode:         getEnv("DB_SSLMODE", "disable"),
		TelegramAPIID:     apiID,
		TelegramAPIHash:   getEnv("TELEGRAM_API_HASH", ""),
		TelegramPhone:     getEnv("TELEGRAM_PHONE", ""),
		TelegramChannelID: channelID,
	}, nil
}

func (c *Config) DatabaseURL() string {
	return "postgres://" + c.DBUser + ":" + c.DBPassword +
		"@" + c.DBHost + ":" + c.DBPort +
		"/" + c.DBName + "?sslmode=" + c.DBSSLMode
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
