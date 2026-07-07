package mermaid

import (
	"os/exec"
	"testing"
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
