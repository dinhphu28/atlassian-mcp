package markdown

import "testing"

func TestCodeMacroWithLanguage(t *testing.T) {
	got := CodeMacro("go", "fmt.Println(1)")
	want := `<ac:structured-macro ac:name="code"><ac:parameter ac:name="language">go</ac:parameter><ac:plain-text-body><![CDATA[fmt.Println(1)]]></ac:plain-text-body></ac:structured-macro>`
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestCodeMacroNoLanguage(t *testing.T) {
	got := CodeMacro("", "plain")
	want := `<ac:structured-macro ac:name="code"><ac:plain-text-body><![CDATA[plain]]></ac:plain-text-body></ac:structured-macro>`
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestAttachmentImage(t *testing.T) {
	got := AttachmentImage("diagram.png")
	want := `<ac:image><ri:attachment ri:filename="diagram.png"/></ac:image>`
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestURLImage(t *testing.T) {
	got := URLImage("https://x/y.png")
	want := `<ac:image><ri:url ri:value="https://x/y.png"/></ac:image>`
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}
