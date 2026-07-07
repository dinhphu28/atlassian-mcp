// Package mermaid renders Mermaid diagram source to PNG by shelling out to the
// mermaid-cli (mmdc) binary. It is optional: callers must handle Render errors
// by falling back to a code block.
package mermaid

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Available reports whether the mmdc binary is on PATH.
func Available() bool {
	_, err := exec.LookPath("mmdc")
	return err == nil
}

// Render converts Mermaid source to PNG bytes using mmdc. It returns an error
// if mmdc is unavailable or rendering fails.
func Render(source string) ([]byte, error) {
	if !Available() {
		return nil, fmt.Errorf("mmdc not found on PATH")
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

	cmd := exec.Command("mmdc", "-i", in, "-o", out)
	if combined, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("mmdc failed: %v: %s", err, string(combined))
	}

	return os.ReadFile(out)
}
