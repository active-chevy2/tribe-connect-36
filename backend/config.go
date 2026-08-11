package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port           string
	DSN            string
	JWTSecret      []byte
	WebDir         string
	FeedWorker     bool
	RefreshMinutes int
	PublicBaseURL  string
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func loadConfig() Config {
	dsn := os.Getenv("DATABASE_DSN")
	if dsn == "" {
		host := getenv("DB_HOST", "127.0.0.1")
		port := getenv("DB_PORT", "3306")
		user := getenv("DB_USER", "feeduser")
		pass := getenv("DB_PASSWORD", "feedpass")
		name := getenv("DB_NAME", "feedsocial")
		dsn = fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4&loc=UTC",
			user, pass, host, port, name)
	}

	refresh, _ := strconv.Atoi(getenv("FEED_REFRESH_MINUTES", "15"))
	if refresh <= 0 {
		refresh = 15
	}

	return Config{
		Port:           getenv("PORT", "8080"),
		DSN:            dsn,
		JWTSecret:      []byte(getenv("JWT_SECRET", "dev-insecure-secret-change-me")),
		WebDir:         getenv("WEB_DIR", "./web"),
		FeedWorker:     strings.ToLower(getenv("FEED_WORKER", "on")) != "off",
		RefreshMinutes: refresh,
		PublicBaseURL:  os.Getenv("PUBLIC_BASE_URL"),
	}
}
