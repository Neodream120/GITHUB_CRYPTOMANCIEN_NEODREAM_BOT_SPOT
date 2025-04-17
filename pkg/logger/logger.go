package logger

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

// LogLevel définit le niveau de log
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

// LogFormat définit le format de sortie des logs
type LogFormat string

const (
	FormatText LogFormat = "text"
	FormatJSON LogFormat = "json"
)

// LogConfig contient la configuration du logger
type LogConfig struct {
	Level  string
	Format string
}

// Logger représente un logger personnalisé
type Logger struct {
	level  LogLevel
	format LogFormat
	logger *log.Logger
}

// NewLogger crée une nouvelle instance de Logger
func NewLogger(config LogConfig) *Logger {
	// Déterminer le niveau de log
	var level LogLevel
	switch strings.ToLower(config.Level) {
	case "debug":
		level = LevelDebug
	case "info":
		level = LevelInfo
	case "warn":
		level = LevelWarn
	case "error":
		level = LevelError
	default:
		level = LevelInfo
	}

	// Déterminer le format
	format := FormatText
	if strings.ToLower(config.Format) == "json" {
		format = FormatJSON
	}

	// Créer le logger interne
	logger := log.New(os.Stdout, "", 0)

	return &Logger{
		level:  level,
		format: format,
		logger: logger,
	}
}

// formatMessage formate un message selon le format configuré
func (l *Logger) formatMessage(level, format string, args ...interface{}) string {
	message := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	if l.format == FormatJSON {
		return fmt.Sprintf("{\"time\":\"%s\",\"level\":\"%s\",\"message\":\"%s\"}",
			timestamp, level, message)
	}

	return fmt.Sprintf("[%s] [%s] %s", timestamp, level, message)
}

// Debug enregistre un message de niveau debug
func (l *Logger) Debug(format string, args ...interface{}) {
	if l.level <= LevelDebug {
		l.logger.Println(l.formatMessage("DEBUG", format, args...))
	}
}

// Info enregistre un message de niveau info
func (l *Logger) Info(format string, args ...interface{}) {
	if l.level <= LevelInfo {
		l.logger.Println(l.formatMessage("INFO", format, args...))
	}
}

// Warn enregistre un message de niveau warning
func (l *Logger) Warn(format string, args ...interface{}) {
	if l.level <= LevelWarn {
		l.logger.Println(l.formatMessage("WARN", format, args...))
	}
}

// Error enregistre un message de niveau error
func (l *Logger) Error(format string, args ...interface{}) {
	if l.level <= LevelError {
		l.logger.Println(l.formatMessage("ERROR", format, args...))
	}
}
