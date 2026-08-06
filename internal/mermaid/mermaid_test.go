package mermaid

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestAvailableMatchesLookPath(t *testing.T) {
	_, err := exec.LookPath("mmdc")
	want := err == nil
	if Available() != want {
		t.Errorf("Available()=%v want %v", Available(), want)
	}
}

func TestRenderErrorsWhenUnavailable(t *testing.T) {
	if Available() {
		t.Skip("mmdc installed; unavailable path not exercised")
	}
	if _, err := Render("graph TD;A-->B;"); err == nil {
		t.Error("expected error when mmdc is unavailable")
	}
}

// A hung mmdc must not block forever: renderContext honors the context
// deadline. A fake binary that sleeps stands in for a wedged headless Chromium.
func TestRenderContextRespectsDeadline(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake sleep binary is POSIX")
	}
	fake := filepath.Join(t.TempDir(), "mmdc")
	// exec so the tracked process is the sleeper itself (no orphaned child
	// holding the output pipe open past the kill).
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexec sleep 5\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MERMAID_CLI_PATH", fake)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := renderContext(ctx, "graph TD;A-->B;")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error when the render exceeds the deadline")
	}
	if elapsed > 3*time.Second {
		t.Errorf("render ignored the deadline, took %s", elapsed)
	}
}

func TestRenderProducesPNGWhenAvailable(t *testing.T) {
	if !Available() {
		t.Skip("mmdc not installed")
	}
	png, err := Render("graph TD;A-->B;")
	if err != nil {
		t.Fatal(err)
	}
	if len(png) < 8 || string(png[1:4]) != "PNG" {
		t.Errorf("output is not a PNG (len=%d)", len(png))
	}
}
