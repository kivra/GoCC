package logctx

import (
	"context"
	"log/slog"
	"testing"
)

func TestAdd(t *testing.T) {
	ctx := context.Background()
	ctx = Add(ctx, "key", "value")
	ctx = Add(ctx, "key", "value2")
	result := GetAll(ctx)
	// Check if the result is correct
	if len(result) != 1 {
		t.Errorf("Expected 1, got %d", len(result))
	}

	// The contents should be correct
	if result[0].(slog.Attr).Value.String() != "value2" {
		t.Errorf("Expected value2, got %s", result[0].(slog.Attr).Value.String())
	}
}

func TestAddDifferentValues(t *testing.T) {
	ctx := context.Background()
	ctx = Add(ctx, "key", "value")
	ctx = Add(ctx, "key2", "value2")
	result := GetAll(ctx)
	// Check if the result is correct
	if len(result) != 2 {
		t.Errorf("Expected 2, got %d", len(result))
	}

	// The contents should be correct
	if result[0].(slog.Attr).Value.String() != "value" {
		t.Errorf("Expected value, got %s", result[0].(slog.Attr).Value.String())
	}
	if result[1].(slog.Attr).Value.String() != "value2" {
		t.Errorf("Expected value2, got %s", result[1].(slog.Attr).Value.String())
	}
}
