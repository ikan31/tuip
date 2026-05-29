package diagnostics

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tuipcli/tuip/internal/config"
)

const (
	EnvLogLevel = "TUIP_LOG_LEVEL"

	maxLogBytes        = 5 * 1024 * 1024
	maxRotatedLogFiles = 3
	logDirPerm         = 0o750
	logFilePerm        = 0o600
)

type noopCloser struct{}

func (noopCloser) Close() error { return nil }

// NewLogger creates a structured JSONL logger for tuip diagnostics. Logging is
// disabled when levelValue and TUIP_LOG_LEVEL are empty or set to "off".
func NewLogger(configPath, levelValue, appVersion string) (*slog.Logger, io.Closer, string, error) {
	level, enabled, err := parseLevel(firstNonEmpty(levelValue, os.Getenv(EnvLogLevel)))
	if err != nil {
		return nil, nil, "", err
	}

	if !enabled {
		return slog.New(slog.DiscardHandler), noopCloser{}, "", nil
	}

	path, err := config.LogPath(configPath)
	if err != nil {
		return nil, nil, "", fmt.Errorf("resolve log path: %w", err)
	}

	err = os.MkdirAll(filepath.Dir(path), logDirPerm)
	if err != nil {
		return nil, nil, "", fmt.Errorf("create log directory: %w", err)
	}

	err = rotateLogIfNeeded(path)
	if err != nil {
		return nil, nil, "", fmt.Errorf("rotate log file: %w", err)
	}

	// #nosec G304 -- path is derived from tuip's configured runtime directory.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, logFilePerm)
	if err != nil {
		return nil, nil, "", fmt.Errorf("open log file: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(file, &slog.HandlerOptions{Level: level})).With(
		slog.String("run_id", newRunID(time.Now().UTC(), os.Getpid())),
		slog.Int("pid", os.Getpid()),
		slog.String("version", appVersion),
	)
	logger.Debug(
		"diagnostics_logger_started",
		slog.String("path", path),
		slog.String("log_level", level.String()),
		slog.Int64("max_log_bytes", maxLogBytes),
		slog.Int("max_rotated_files", maxRotatedLogFiles),
	)

	return logger, file, path, nil
}

func rotateLogIfNeeded(path string) error {
	stat, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("stat current log: %w", err)
	}

	if stat.Size() < maxLogBytes {
		return nil
	}

	oldestPath := rotatedLogPath(path, maxRotatedLogFiles)

	err = os.Remove(oldestPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove oldest rotated log %s: %w", oldestPath, err)
	}

	for idx := maxRotatedLogFiles - 1; idx >= 1; idx-- {
		sourcePath := rotatedLogPath(path, idx)
		targetPath := rotatedLogPath(path, idx+1)

		err = renameIfExists(sourcePath, targetPath)
		if err != nil {
			return err
		}
	}

	targetPath := rotatedLogPath(path, 1)

	err = os.Rename(path, targetPath)
	if err != nil {
		return fmt.Errorf("rotate current log to %s: %w", targetPath, err)
	}

	return nil
}

func renameIfExists(sourcePath, targetPath string) error {
	err := os.Rename(sourcePath, targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("rotate log %s to %s: %w", sourcePath, targetPath, err)
	}

	return nil
}

func rotatedLogPath(path string, index int) string {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(filepath.Base(path), ext)

	return filepath.Join(filepath.Dir(path), fmt.Sprintf("%s.%d%s", base, index, ext))
}

func newRunID(now time.Time, pid int) string {
	return fmt.Sprintf("%s-%d", now.Format("20060102T150405.000000000Z"), pid)
}

func parseLevel(value string) (slog.Level, bool, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || value == "off" || value == "none" || value == "false" || value == "0" {
		return slog.LevelInfo, false, nil
	}

	switch value {
	case "debug":
		return slog.LevelDebug, true, nil
	case "info":
		return slog.LevelInfo, true, nil
	case "warn", "warning":
		return slog.LevelWarn, true, nil
	case "error":
		return slog.LevelError, true, nil
	default:
		return slog.LevelInfo, false, fmt.Errorf("unsupported log level %q; use off, debug, info, warn, or error", value)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}

	return ""
}
