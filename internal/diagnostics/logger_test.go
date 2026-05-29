package diagnostics

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tuipcli/tuip/internal/config"
)

const testVersion = "test-version"

func TestNewLoggerRotatesOversizedLogAndAddsRunFields(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.yaml")

	logPath, err := config.LogPath(configPath)
	if err != nil {
		t.Fatalf("LogPath() error = %v", err)
	}

	err = os.MkdirAll(filepath.Dir(logPath), logDirPerm)
	if err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	oversizedLog := strings.Repeat("x", maxLogBytes+1)

	err = os.WriteFile(logPath, []byte(oversizedLog), logFilePerm)
	if err != nil {
		t.Fatalf("WriteFile(current) error = %v", err)
	}

	logger, closer, gotPath, err := NewLogger(configPath, "debug", testVersion)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}

	if gotPath != logPath {
		t.Fatalf("path = %q, want %q", gotPath, logPath)
	}

	logger.Debug("test_event")

	err = closer.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	rotated, err := os.ReadFile(rotatedLogPath(logPath, 1))
	if err != nil {
		t.Fatalf("ReadFile(rotated) error = %v", err)
	}

	if len(rotated) != len(oversizedLog) {
		t.Fatalf("rotated log size = %d, want %d", len(rotated), len(oversizedLog))
	}

	// #nosec G304 -- test path is created under t.TempDir().
	current, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(current) error = %v", err)
	}

	for _, want := range [][]byte{
		[]byte(`"run_id"`),
		[]byte(`"pid"`),
		[]byte(`"version":"` + testVersion + `"`),
		[]byte(`"msg":"diagnostics_logger_started"`),
		[]byte(`"msg":"test_event"`),
	} {
		if !bytes.Contains(current, want) {
			t.Fatalf("current log missing %s:\n%s", want, current)
		}
	}
}

func TestRotateLogIfNeededKeepsThreeBackups(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logPath := filepath.Join(dir, "tuip.jsonl")

	files := map[string]string{
		logPath:                    strings.Repeat("x", maxLogBytes+1),
		rotatedLogPath(logPath, 1): "old-one",
		rotatedLogPath(logPath, 2): "old-two",
		rotatedLogPath(logPath, 3): "old-three",
	}

	for path, content := range files {
		err := os.WriteFile(path, []byte(content), logFilePerm)
		if err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
	}

	err := rotateLogIfNeeded(logPath)
	if err != nil {
		t.Fatalf("rotateLogIfNeeded() error = %v", err)
	}

	assertFileContains(t, rotatedLogPath(logPath, 1), []byte("xxx"))
	assertFileContains(t, rotatedLogPath(logPath, 2), []byte("old-one"))
	assertFileContains(t, rotatedLogPath(logPath, 3), []byte("old-two"))
}

func TestNewRunIDIncludesTimestampAndPID(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 29, 3, 30, 12, 123, time.UTC)
	got := newRunID(now, 42)
	want := "20260529T033012.000000123Z-42"

	if got != want {
		t.Fatalf("newRunID() = %q, want %q", got, want)
	}
}

func assertFileContains(t *testing.T, path string, want []byte) {
	t.Helper()

	// #nosec G304 -- test helper reads paths created under t.TempDir().
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}

	if !bytes.Contains(data, want) {
		t.Fatalf("%s missing %q", path, want)
	}
}
