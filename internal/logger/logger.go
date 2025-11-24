package logger

import (
	"io"
	"log/slog"
	"os"

	"aka-webgui/internal/config"

	"github.com/natefinch/lumberjack"
)

func Setup(cfg *config.Config) {
	fileWriter := &lumberjack.Logger{
		Filename:   cfg.LogFile,
		MaxSize:    cfg.LogMaxSizeMB,
		MaxBackups: cfg.LogMaxBackups,
		MaxAge:     cfg.LogMaxAgeDays,
	}

	// Write to both file and stdout
	multiWriter := io.MultiWriter(os.Stdout, fileWriter)

	logger := slog.New(slog.NewJSONHandler(multiWriter, nil))
	slog.SetDefault(logger)
}
