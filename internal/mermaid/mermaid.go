// Package mermaid renders Mermaid diagram source to PNG by shelling out to the
// mermaid-cli (mmdc) binary. It is optional: callers must handle Render errors
// by falling back to a code block.
package mermaid

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// renderTimeout bounds a single mmdc invocation. mmdc drives a headless
// Chromium, which can hang indefinitely (sandbox, no display, font lookups), so
// a render that exceeds this is treated as a failure rather than blocking the
// MCP call forever.
const renderTimeout = 60 * time.Second

// binaryPath locates the mmdc binary: the MERMAID_CLI_PATH override when set,
// otherwise the first "mmdc" on PATH. The override matters because the MCP
// server is a long-lived process whose PATH may not include the user's shell
// additions (e.g. /home/linuxbrew/.linuxbrew/bin).
func binaryPath() (string, error) {
	if p := os.Getenv("MERMAID_CLI_PATH"); p != "" {
		return p, nil
	}
	return exec.LookPath("mmdc")
}

// Available reports whether an mmdc binary can be located (via MERMAID_CLI_PATH
// or PATH).
func Available() bool {
	_, err := binaryPath()
	return err == nil
}

// background returns the render background color (MERMAID_BACKGROUND, default
// "white"). Confluence shows the attachment on both light and dark themes, so a
// transparent background renders poorly.
func background() string {
	if b := os.Getenv("MERMAID_BACKGROUND"); b != "" {
		return b
	}
	return "white"
}

// scale returns the render scale factor (MERMAID_SCALE, default "2"). 1x text is
// blurry on Confluence.
func scale() string {
	if s := os.Getenv("MERMAID_SCALE"); s != "" {
		return s
	}
	return "2"
}

// Render converts Mermaid source to PNG bytes using mmdc. It returns an error if
// mmdc is unavailable, the render times out, or rendering fails.
func Render(source string) ([]byte, error) {
	return renderContext(context.Background(), source)
}

// renderContext is Render with a caller-supplied context so the timeout is
// testable; the returned context is additionally bounded by renderTimeout.
func renderContext(ctx context.Context, source string) ([]byte, error) {
	bin, err := binaryPath()
	if err != nil {
		return nil, fmt.Errorf("mmdc not found (set MERMAID_CLI_PATH or add it to PATH): %w", err)
	}

	dir, err := os.MkdirTemp("", "mermaid-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	in := filepath.Join(dir, "in.mmd")
	out := filepath.Join(dir, "out.png")
	if err := os.WriteFile(in, []byte(source), 0o600); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, renderTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "-i", in, "-o", out, "-b", background(), "-s", scale())
	// After the context kills mmdc, bound the extra wait for any child processes
	// (headless Chromium) that inherited its output pipes but linger.
	cmd.WaitDelay = 5 * time.Second
	combined, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("mmdc timed out after %s", renderTimeout)
	}
	if err != nil {
		return nil, fmt.Errorf("mmdc failed: %v: %s", err, string(combined))
	}

	return os.ReadFile(out)
}
