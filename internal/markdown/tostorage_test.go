package markdown

import (
	"strings"
	"testing"
)

func TestToStorageHeadingAndList(t *testing.T) {
	got, diagrams, err := ToStorage("## Title\n\n- a\n- b\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(diagrams) != 0 {
		t.Fatalf("expected no diagrams, got %d", len(diagrams))
	}
	if !strings.Contains(got, "<h2>Title</h2>") {
		t.Errorf("missing heading in %q", got)
	}
	if !strings.Contains(got, "<li>a</li>") || !strings.Contains(got, "<li>b</li>") {
		t.Errorf("missing list items in %q", got)
	}
}

func TestToStorageFencedCode(t *testing.T) {
	got, _, err := ToStorage("```go\nfmt.Println(1)\n```\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `ac:name="code"`) ||
		!strings.Contains(got, `<ac:parameter ac:name="language">go</ac:parameter>`) ||
		!strings.Contains(got, "fmt.Println(1)") {
		t.Errorf("code macro not emitted: %q", got)
	}
}

func TestToStorageImages(t *testing.T) {
	got, _, err := ToStorage("![a](pic.png) and ![b](https://x/y.png)\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `<ri:attachment ri:filename="pic.png"/>`) {
		t.Errorf("attachment image missing: %q", got)
	}
	if !strings.Contains(got, `<ri:url ri:value="https://x/y.png"/>`) {
		t.Errorf("url image missing: %q", got)
	}
}

func TestToStorageMermaidExtracted(t *testing.T) {
	got, diagrams, err := ToStorage("```mermaid\ngraph TD;A-->B;\n```\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(diagrams) != 1 {
		t.Fatalf("expected 1 diagram, got %d", len(diagrams))
	}
	if diagrams[0].Placeholder != "<!--mermaid:0-->" {
		t.Errorf("bad placeholder %q", diagrams[0].Placeholder)
	}
	if !strings.Contains(diagrams[0].Source, "graph TD") {
		t.Errorf("source not captured: %q", diagrams[0].Source)
	}
	if !strings.Contains(got, "<!--mermaid:0-->") {
		t.Errorf("placeholder not in output: %q", got)
	}
	if strings.Contains(got, `ac:name="code"`) {
		t.Errorf("mermaid should not become a code macro: %q", got)
	}
}

func TestToStorageTable(t *testing.T) {
	got, _, err := ToStorage("| h |\n|---|\n| c |\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "<table>") || !strings.Contains(got, "<td>c</td>") {
		t.Errorf("table not rendered: %q", got)
	}
}
