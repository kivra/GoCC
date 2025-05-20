package logging

import (
	"github.com/google/go-cmp/cmp"
	"log/slog"
	"testing"
)

func TestLevelStringToLevelValue(t *testing.T) {

	if diff := cmp.Diff(levelStringToLevelValue("DEBUG"), slog.LevelDebug); diff != "" {
		t.Errorf("levelStringToLevelValue() mismatch (-want +got):\n%s", diff)
	}

	if diff := cmp.Diff(levelStringToLevelValue("debug"), slog.LevelDebug); diff != "" {
		t.Errorf("levelStringToLevelValue() mismatch (-want +got):\n%s", diff)
	}

	if diff := cmp.Diff(levelStringToLevelValue("INFO"), slog.LevelInfo); diff != "" {
		t.Errorf("levelStringToLevelValue() mismatch (-want +got):\n%s", diff)
	}

	if diff := cmp.Diff(levelStringToLevelValue("info"), slog.LevelInfo); diff != "" {
		t.Errorf("levelStringToLevelValue() mismatch (-want +got):\n%s", diff)
	}

	if diff := cmp.Diff(levelStringToLevelValue("ERROR"), slog.LevelError); diff != "" {
		t.Errorf("levelStringToLevelValue() mismatch (-want +got):\n%s", diff)
	}

	if diff := cmp.Diff(levelStringToLevelValue("error"), slog.LevelError); diff != "" {
		t.Errorf("levelStringToLevelValue() mismatch (-want +got):\n%s", diff)
	}

	if diff := cmp.Diff(levelStringToLevelValue("WARN"), slog.LevelWarn); diff != "" {
		t.Errorf("levelStringToLevelValue() mismatch (-want +got):\n%s", diff)
	}

	if diff := cmp.Diff(levelStringToLevelValue("warn"), slog.LevelWarn); diff != "" {
		t.Errorf("levelStringToLevelValue() mismatch (-want +got):\n%s", diff)
	}
}

func TestConfigureDefaultLogger(t *testing.T) {

	ConfigureDefaultLogger("system-default", "INFO", false)
	slog.Info("This is system-default")

	ConfigureDefaultLogger("text", "INFO", false)
	slog.Info("This is text")

	ConfigureDefaultLogger("json", "INFO", false)
	slog.Info("This is json")
}
