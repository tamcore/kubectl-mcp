package tools

import (
	"context"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestApplySafetyDelay_ZeroDelay(t *testing.T) {
	req := mcp.CallToolRequest{}
	start := time.Now()
	if err := applySafetyDelay(context.Background(), req, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed := time.Since(start); elapsed >= 50*time.Millisecond {
		t.Fatalf("zero delay took too long: %v", elapsed)
	}
}

func TestApplySafetyDelay_NegativeDelay(t *testing.T) {
	req := mcp.CallToolRequest{}
	start := time.Now()
	if err := applySafetyDelay(context.Background(), req, -1*time.Second); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed := time.Since(start); elapsed >= 50*time.Millisecond {
		t.Fatalf("negative delay took too long: %v", elapsed)
	}
}

func TestApplySafetyDelay_SleepsForDuration(t *testing.T) {
	req := mcp.CallToolRequest{}
	delay := 200 * time.Millisecond
	start := time.Now()
	if err := applySafetyDelay(context.Background(), req, delay); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < delay {
		t.Fatalf("elapsed %v < delay %v", elapsed, delay)
	}
	if elapsed > delay+200*time.Millisecond {
		t.Fatalf("elapsed %v too far over delay %v", elapsed, delay)
	}
}

func TestApplySafetyDelay_ContextCancellation(t *testing.T) {
	req := mcp.CallToolRequest{}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	err := applySafetyDelay(ctx, req, 5*time.Second)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected error on cancellation, got nil")
	}
	if elapsed >= 200*time.Millisecond {
		t.Fatalf("cancellation too slow: %v", elapsed)
	}
}

func TestApplySafetyDelay_NoOpProgressWithoutToken(t *testing.T) {
	// No progressToken — helper must sleep without panicking.
	req := mcp.CallToolRequest{}
	delay := 150 * time.Millisecond
	if err := applySafetyDelay(context.Background(), req, delay); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
