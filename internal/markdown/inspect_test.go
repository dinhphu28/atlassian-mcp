package markdown

import (
	"strings"
	"testing"
)

func TestUntranslatedMacrosIgnoresWhatTheConverterHandles(t *testing.T) {
	storage := `<p>hi</p>` +
		`<ac:structured-macro ac:name="code"><ac:parameter ac:name="language">go</ac:parameter>` +
		`<ac:plain-text-body><![CDATA[x := 1]]></ac:plain-text-body></ac:structured-macro>` +
		`<ac:image ac:align="center"><ri:attachment ri:filename="a.png"/></ac:image>` +
		`<ac:image><ri:url ri:value="https://e/x.png"/></ac:image>`

	if got := UntranslatedMacros(storage); got != nil {
		t.Errorf("expected no losses for a faithfully converted page, got %v", got)
	}
}

func TestUntranslatedMacrosNamesMacrosAndElements(t *testing.T) {
	storage := `<ac:structured-macro ac:name="info"><ac:rich-text-body><p>note</p></ac:rich-text-body></ac:structured-macro>` +
		`<ac:structured-macro ac:name="toc"><ac:parameter ac:name="maxLevel">3</ac:parameter></ac:structured-macro>` +
		`<ac:task-list><ac:task><ac:task-status>incomplete</ac:task-status></ac:task></ac:task-list>` +
		`<ac:link><ri:user ri:userkey="abc"/></ac:link>`

	got := UntranslatedMacros(storage)
	joined := strings.Join(got, ",")
	for _, want := range []string{"info", "toc", "ac:task-list", "ac:link", "ri:user"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected %q among the losses, got %v", want, got)
		}
	}
	// Macro plumbing must not be reported as a loss of its own.
	for _, unwanted := range []string{"ac:structured-macro", "ac:parameter", "ac:rich-text-body"} {
		if strings.Contains(joined, unwanted) {
			t.Errorf("reported macro plumbing %q as a loss: %v", unwanted, got)
		}
	}
}

// An ac:image the converter's strict regexes cannot match (it carries a caption)
// must be reported, because that image silently disappears from the Markdown.
func TestUntranslatedMacrosReportsUnmatchedImage(t *testing.T) {
	storage := `<ac:image><ri:attachment ri:filename="a.png"/><ac:caption><p>fig 1</p></ac:caption></ac:image>`

	got := UntranslatedMacros(storage)
	if strings.Join(got, ",") == "" || !strings.Contains(strings.Join(got, ","), "ac:image") {
		t.Errorf("expected ac:image to be reported, got %v", got)
	}
}
