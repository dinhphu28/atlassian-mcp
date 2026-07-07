package markdown

import (
	"strings"
	"testing"
)

func TestToMarkdownHeadingAndList(t *testing.T) {
	got, err := ToMarkdown("<h2>Title</h2><ul><li>a</li><li>b</li></ul>")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "## Title") {
		t.Errorf("heading missing: %q", got)
	}
	if !strings.Contains(got, "- a") {
		t.Errorf("list missing: %q", got)
	}
}

func TestToMarkdownCodeMacro(t *testing.T) {
	storage := CodeMacro("go", "fmt.Println(1)")
	got, err := ToMarkdown("<p>x</p>" + storage)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "```") || !strings.Contains(got, "fmt.Println(1)") {
		t.Errorf("code fence missing: %q", got)
	}
}

func TestToMarkdownAttachmentImage(t *testing.T) {
	got, err := ToMarkdown(AttachmentImage("pic.png"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "(pic.png)") {
		t.Errorf("attachment image not converted: %q", got)
	}
}

func TestToMarkdownURLImage(t *testing.T) {
	got, err := ToMarkdown(URLImage("https://x/y.png"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "(https://x/y.png)") {
		t.Errorf("url image not converted: %q", got)
	}
}

func TestRoundTripCommonSubset(t *testing.T) {
	src := "## Title\n\n- a\n- b\n\n**bold** and [link](https://x)\n"
	storage, _, err := ToStorage(src)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ToMarkdown(storage)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## Title", "- a", "- b", "**bold**", "[link](https://x)"} {
		if !strings.Contains(back, want) {
			t.Errorf("round-trip lost %q; got %q", want, back)
		}
	}
}
