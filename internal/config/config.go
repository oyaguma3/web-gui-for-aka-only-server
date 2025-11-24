package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	ListenAddr     string
	AuthUsername   string
	AuthPassword   string
	SessionMinutes int
	LogFile        string
	LogMaxSizeMB   int
	LogMaxBackups  int
	LogMaxAgeDays  int
	AKABaseURL     string
}

func Load() (*Config, error) {
	_ = godotenv.Load("webgui.env") // Load if exists, ignore if not

	sessionMin, _ := strconv.Atoi(getEnv("WEBGUI_SESSION_MINUTES", "1440"))
	logMaxSize, _ := strconv.Atoi(getEnv("WEBGUI_LOG_MAX_SIZE_MB", "10"))
	logMaxBackups, _ := strconv.Atoi(getEnv("WEBGUI_LOG_MAX_BACKUPS", "3"))
	logMaxAge, _ := strconv.Atoi(getEnv("WEBGUI_LOG_MAX_AGE_DAYS", "28"))

	return &Config{
		ListenAddr:     getEnv("WEBGUI_LISTEN_ADDR", "localhost:9999"),
		AuthUsername:   getEnv("WEBGUI_AUTH_USERNAME", "admin"),
		AuthPassword:   getEnv("WEBGUI_AUTH_PASSWORD", "admin"),
		SessionMinutes: sessionMin,
		LogFile:        getEnv("WEBGUI_LOG_FILE", "webgui.log"),
		LogMaxSizeMB:   logMaxSize,
		LogMaxBackups:  logMaxBackups,
		LogMaxAgeDays:  logMaxAge,
		AKABaseURL:     getEnv("AKA_API_BASE_URL", "http://localhost:8080/api/v1"),
	}, nil
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
