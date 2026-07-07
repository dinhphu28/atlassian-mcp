# Markdown ↔ Confluence Bridge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the AI author and read Confluence pages as Markdown instead of verbose storage XHTML, converting inside the Go server, with local Mermaid-to-image rendering.

**Architecture:** Two pure conversion packages (`internal/markdown`, `internal/mermaid`) do string/byte transforms with no Confluence coupling. New orchestration methods on the existing `confluence.Client` convert Markdown, render+upload Mermaid diagrams as attachments, and reuse the existing storage-format `CreatePage`/`UpdatePage`/`UploadAttachment` HTTP methods. The mcpserver layer adds `representation="markdown"` (the new default), a `file_path` write input, and an `output_path` read input.

**Tech Stack:** Go 1.26.2, `github.com/mark3labs/mcp-go`, `github.com/yuin/goldmark` (+ GFM extension), `github.com/JohannesKaufmann/html-to-markdown`, external `mmdc` binary (optional).

## Global Constraints

- Module path: `dinhphu28/atlassian-mcp` — all internal imports use this prefix.
- Go version floor: `go 1.26.2` (do not lower `go.mod`).
- Follow existing patterns: HTTP client methods return `(string, error)` raw JSON; MCP handlers return errors via `mcp.NewToolResultError(err.Error()), nil`; pretty JSON via `jsonResult`.
- New runtime dependencies limited to `goldmark` and `html-to-markdown`. Mermaid rendering is an **optional** shell-out to `mmdc`; its absence must never fail a write (fall back to a code macro).
- Default `representation` for Confluence page/comment reads and writes becomes `"markdown"`. `"storage"` and `"wiki"` remain valid explicit values.
- Mermaid is one-way: reads return the rendered image reference, not the source.

---

## File Structure

- `internal/markdown/macros.go` — shared storage-format builders (`CodeMacro`, `AttachmentImage`, `URLImage`). Pure.
- `internal/markdown/tostorage.go` — `ToStorage`: Markdown → storage XHTML + extracted Mermaid diagrams. Pure.
- `internal/markdown/tomarkdown.go` — `ToMarkdown`: storage XHTML → Markdown. Pure.
- `internal/markdown/*_test.go` — table-driven + round-trip tests.
- `internal/mermaid/mermaid.go` — `Available`, `Render` (shell out to `mmdc`).
- `internal/mermaid/mermaid_test.go` — availability + fallback tests.
- `internal/confluence/markdown_pages.go` — `CreatePageMarkdown`, `UpdatePageMarkdown`, `GetPageMarkdown`, `GetCommentsMarkdown`, `MermaidRenderer` type, diagram-apply/upload helpers.
- `internal/confluence/markdown_pages_test.go` — httptest-backed orchestration tests.
- `internal/mcpserver/confluence_write.go` — wire `representation`/`file_path` into write tools (modify).
- `internal/mcpserver/confluence_read.go` — wire `representation`/`output_path` into read tools (modify).
- `README.md` — document the Markdown workflow (modify).

---

## Task 1: Shared storage-format macro builders

**Files:**
- Create: `internal/markdown/macros.go`
- Test: `internal/markdown/macros_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `func CodeMacro(lang, source string) string`
  - `func AttachmentImage(filename string) string`
  - `func URLImage(url string) string`

- [ ] **Step 1: Add the two new dependencies**

Run:
```bash
cd /home/dinhphu28/tmp/cads/confluence-mcp
go get github.com/yuin/goldmark@latest
go get github.com/JohannesKaufmann/html-to-markdown@latest
```
Expected: `go.mod` gains `require github.com/yuin/goldmark ...` and `github.com/JohannesKaufmann/html-to-markdown ...`; `go.sum` updated.

- [ ] **Step 2: Write the failing test**

Create `internal/markdown/macros_test.go`:
```go
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
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/markdown/`
Expected: FAIL — `undefined: CodeMacro` (package does not compile yet).

- [ ] **Step 4: Write the implementation**

Create `internal/markdown/macros.go`:
```go
// Package markdown converts between Markdown and Confluence storage format.
package markdown

import "strings"

// CodeMacro renders source as a Confluence code macro. An empty lang omits the
// language parameter.
func CodeMacro(lang, source string) string {
	var b strings.Builder
	b.WriteString(`<ac:structured-macro ac:name="code">`)
	if lang != "" {
		b.WriteString(`<ac:parameter ac:name="language">`)
		b.WriteString(lang)
		b.WriteString(`</ac:parameter>`)
	}
	b.WriteString(`<ac:plain-text-body><![CDATA[`)
	b.WriteString(source)
	b.WriteString(`]]></ac:plain-text-body></ac:structured-macro>`)
	return b.String()
}

// AttachmentImage renders a storage-format image referencing a page attachment.
func AttachmentImage(filename string) string {
	return `<ac:image><ri:attachment ri:filename="` + filename + `"/></ac:image>`
}

// URLImage renders a storage-format image referencing an external URL.
func URLImage(url string) string {
	return `<ac:image><ri:url ri:value="` + url + `"/></ac:image>`
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/markdown/`
Expected: PASS (ok).

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/markdown/macros.go internal/markdown/macros_test.go
git commit --no-gpg-sign -m "feat(markdown): add storage-format macro builders"
```

---

## Task 2: Markdown → storage converter (`ToStorage`)

**Files:**
- Create: `internal/markdown/tostorage.go`
- Test: `internal/markdown/tostorage_test.go`

**Interfaces:**
- Consumes: `CodeMacro`, `AttachmentImage`, `URLImage` (Task 1).
- Produces:
  - `type Diagram struct { Placeholder string; Source string }`
  - `func ToStorage(md string) (storage string, diagrams []Diagram, err error)`
  - Placeholder format: `<!--mermaid:N-->` where N is the zero-based diagram index.

- [ ] **Step 1: Write the failing test**

Create `internal/markdown/tostorage_test.go`:
```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/markdown/ -run ToStorage`
Expected: FAIL — `undefined: ToStorage`.

- [ ] **Step 3: Write the implementation**

Create `internal/markdown/tostorage.go`:
```go
package markdown

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// Diagram is a Mermaid block extracted from Markdown. Placeholder is the exact
// token left in the storage output for the caller to substitute.
type Diagram struct {
	Placeholder string
	Source      string
}

// ToStorage converts Markdown to Confluence storage XHTML. Fenced ```mermaid
// blocks are replaced with placeholders and returned as diagrams for the caller
// to render and substitute.
func ToStorage(md string) (string, []Diagram, error) {
	r := &storageRenderer{}
	gm := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRenderer(
			renderer.NewRenderer(
				renderer.WithNodeRenderers(
					util.Prioritized(html.NewRenderer(html.WithUnsafe()), 1000),
					util.Prioritized(r, 100),
				),
			),
		),
	)
	var buf bytes.Buffer
	if err := gm.Convert([]byte(md), &buf); err != nil {
		return "", nil, err
	}
	return buf.String(), r.diagrams, nil
}

type storageRenderer struct {
	diagrams []Diagram
}

func (r *storageRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindFencedCodeBlock, r.renderFenced)
	reg.Register(ast.KindCodeBlock, r.renderIndentedCode)
	reg.Register(ast.KindImage, r.renderImage)
}

func (r *storageRenderer) renderFenced(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	fc := n.(*ast.FencedCodeBlock)
	lang := string(fc.Language(source))
	code := nodeLines(n, source)
	if lang == "mermaid" {
		d := Diagram{
			Placeholder: fmt.Sprintf("<!--mermaid:%d-->", len(r.diagrams)),
			Source:      code,
		}
		r.diagrams = append(r.diagrams, d)
		_, _ = w.WriteString(d.Placeholder)
		return ast.WalkSkipChildren, nil
	}
	_, _ = w.WriteString(CodeMacro(lang, code))
	return ast.WalkSkipChildren, nil
}

func (r *storageRenderer) renderIndentedCode(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	_, _ = w.WriteString(CodeMacro("", nodeLines(n, source)))
	return ast.WalkSkipChildren, nil
}

func (r *storageRenderer) renderImage(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	img := n.(*ast.Image)
	dest := string(img.Destination)
	if strings.Contains(dest, "://") {
		_, _ = w.WriteString(URLImage(dest))
	} else {
		_, _ = w.WriteString(AttachmentImage(dest))
	}
	return ast.WalkSkipChildren, nil
}

// nodeLines concatenates the raw text lines of a code node.
func nodeLines(n ast.Node, source []byte) string {
	var b strings.Builder
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		b.Write(seg.Value(source))
	}
	return b.String()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/markdown/ -run ToStorage -v`
Expected: PASS for all five `TestToStorage*` tests. If the fenced-code language test fails because `fc.Language` returns unexpected bytes, print `got` and adjust — the expected substring is `ac:name="code"` and the `go` language parameter.

- [ ] **Step 5: Commit**

```bash
git add internal/markdown/tostorage.go internal/markdown/tostorage_test.go
git commit --no-gpg-sign -m "feat(markdown): convert Markdown to storage format with Mermaid extraction"
```

---

## Task 3: Storage → Markdown converter (`ToMarkdown`)

**Files:**
- Create: `internal/markdown/tomarkdown.go`
- Test: `internal/markdown/tomarkdown_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks (independent).
- Produces: `func ToMarkdown(storage string) (string, error)`

- [ ] **Step 1: Write the failing test**

Create `internal/markdown/tomarkdown_test.go`:
```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/markdown/ -run ToMarkdown`
Expected: FAIL — `undefined: ToMarkdown`.

- [ ] **Step 3: Write the implementation**

Create `internal/markdown/tomarkdown.go`:
```go
package markdown

import (
	"regexp"

	htmltomd "github.com/JohannesKaufmann/html-to-markdown"
)

var (
	// Confluence code macro -> <pre><code>. Captures optional language and the
	// CDATA body. (?s) makes . match newlines.
	reCodeMacro = regexp.MustCompile(
		`(?s)<ac:structured-macro[^>]*ac:name="code".*?<ac:plain-text-body><!\[CDATA\[(.*?)\]\]></ac:plain-text-body>.*?</ac:structured-macro>`)
	reCodeLang = regexp.MustCompile(`<ac:parameter ac:name="language">(.*?)</ac:parameter>`)

	// Storage images -> <img>.
	reAttachImg = regexp.MustCompile(`<ac:image[^>]*>\s*<ri:attachment ri:filename="([^"]*)"[^>]*/>\s*</ac:image>`)
	reURLImg    = regexp.MustCompile(`<ac:image[^>]*>\s*<ri:url ri:value="([^"]*)"[^>]*/>\s*</ac:image>`)
)

// ToMarkdown converts Confluence storage XHTML to Markdown. Known macros (code,
// images) are pre-normalized to plain HTML; unknown macros fall through to the
// generic HTML-to-Markdown pass on a best-effort basis.
func ToMarkdown(storage string) (string, error) {
	html := preprocessStorage(storage)
	conv := htmltomd.NewConverter("", true, nil)
	return conv.ConvertString(html)
}

func preprocessStorage(s string) string {
	s = reCodeMacro.ReplaceAllStringFunc(s, func(m string) string {
		lang := ""
		if lm := reCodeLang.FindStringSubmatch(m); lm != nil {
			lang = lm[1]
		}
		body := reCodeMacro.FindStringSubmatch(m)[1]
		open := "<pre><code>"
		if lang != "" {
			open = `<pre><code class="language-` + lang + `">`
		}
		return open + htmlEscape(body) + "</code></pre>"
	})
	s = reAttachImg.ReplaceAllString(s, `<img src="$1" alt="$1"/>`)
	s = reURLImg.ReplaceAllString(s, `<img src="$1"/>`)
	return s
}

func htmlEscape(s string) string {
	r := s
	r = replaceAll(r, "&", "&amp;")
	r = replaceAll(r, "<", "&lt;")
	r = replaceAll(r, ">", "&gt;")
	return r
}

func replaceAll(s, old, new string) string {
	out := ""
	for {
		i := indexOf(s, old)
		if i < 0 {
			return out + s
		}
		out += s[:i] + new
		s = s[i+len(old):]
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```

Note: `htmlEscape` must run before `&` is double-encoded — it replaces `&` first, which is correct here because the CDATA body is raw text. (`strings` is intentionally avoided in this file to keep the escape order explicit; if you prefer, replace `htmlEscape`/`replaceAll`/`indexOf` with `html.EscapeString` from `"html"` and import it.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/markdown/ -run 'ToMarkdown|RoundTrip' -v`
Expected: PASS. If `TestToMarkdownCodeMacro` fails because the regex over-consumes across two macros, confirm the body test uses a single macro (it does) and that `.*?` is non-greedy.

- [ ] **Step 5: Simplify escape helpers (optional cleanup) and re-run**

If you took the `html.EscapeString` option, replace the three helpers with:
```go
import "html"
// ...
return open + html.EscapeString(body) + "</code></pre>"
```
Run: `go test ./internal/markdown/ -v`
Expected: PASS (all markdown tests).

- [ ] **Step 6: Commit**

```bash
git add internal/markdown/tomarkdown.go internal/markdown/tomarkdown_test.go
git commit --no-gpg-sign -m "feat(markdown): convert storage format back to Markdown"
```

---

## Task 4: Mermaid renderer (`internal/mermaid`)

**Files:**
- Create: `internal/mermaid/mermaid.go`
- Test: `internal/mermaid/mermaid_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `func Available() bool`
  - `func Render(source string) ([]byte, error)`

- [ ] **Step 1: Write the failing test**

Create `internal/mermaid/mermaid_test.go`:
```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/mermaid/`
Expected: FAIL — `undefined: Available` / `undefined: Render`.

- [ ] **Step 3: Write the implementation**

Create `internal/mermaid/mermaid.go`:
```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/mermaid/ -v`
Expected: PASS. On a machine without `mmdc`, `TestRenderProducesPNGWhenAvailable` is skipped and the unavailable-path test runs; with `mmdc`, it renders a real PNG.

- [ ] **Step 5: Commit**

```bash
git add internal/mermaid/mermaid.go internal/mermaid/mermaid_test.go
git commit --no-gpg-sign -m "feat(mermaid): render diagrams to PNG via mmdc"
```

---

## Task 5: Confluence write orchestration (Markdown create/update)

**Files:**
- Create: `internal/confluence/markdown_pages.go`
- Test: `internal/confluence/markdown_pages_test.go`

**Interfaces:**
- Consumes: `markdown.ToStorage`, `markdown.Diagram`, `markdown.CodeMacro`, `markdown.AttachmentImage` (Tasks 1–2); existing `Client.CreatePage`, `Client.UpdatePage`, `Client.UploadAttachment` (`internal/confluence/pages.go`, `attachments.go`).
- Produces:
  - `type MermaidRenderer func(source string) ([]byte, error)`
  - `func (c *Client) CreatePageMarkdown(spaceKey, title, md, parentID string, render MermaidRenderer) (string, error)`
  - `func (c *Client) UpdatePageMarkdown(pageID, md, title string, render MermaidRenderer) (string, error)`
  - `func (c *Client) AddCommentMarkdown(pageID, md string) (string, error)`
  - `func (c *Client) ReplyToCommentMarkdown(parentCommentID, md string) (string, error)`

Note: Mermaid inside a **comment** is always stored as a code macro (no image rendering/upload) — comments keep the simple Markdown→storage path.

- [ ] **Step 1: Write the failing test**

Create `internal/confluence/markdown_pages_test.go`:
```go
package confluence

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestClient points a Client at an httptest server.
func newTestClient(h http.Handler) (*Client, *httptest.Server) {
	srv := httptest.NewServer(h)
	return NewClient(srv.URL, "tok"), srv
}

func TestUpdatePageMarkdownConvertsAndPuts(t *testing.T) {
	var putBody string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			// version/space fetch performed by UpdatePage
			io.WriteString(w, `{"title":"T","space":{"key":"DEV"},"version":{"number":3}}`)
		case r.Method == http.MethodPut:
			b, _ := io.ReadAll(r.Body)
			putBody = string(b)
			io.WriteString(w, `{"id":"123"}`)
		}
	})
	c, srv := newTestClient(h)
	defer srv.Close()

	_, err := c.UpdatePageMarkdown("123", "## Hi\n\n- x\n", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(putBody, "<h2>Hi</h2>") {
		t.Errorf("storage not in PUT body: %s", putBody)
	}
}

func TestUpdatePageMarkdownMermaidFallbackWhenNoRenderer(t *testing.T) {
	var putBody string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			io.WriteString(w, `{"title":"T","space":{"key":"DEV"},"version":{"number":1}}`)
			return
		}
		b, _ := io.ReadAll(r.Body)
		putBody = string(b)
		io.WriteString(w, `{"id":"1"}`)
	})
	c, srv := newTestClient(h)
	defer srv.Close()

	_, err := c.UpdatePageMarkdown("1", "```mermaid\ngraph TD;A-->B;\n```\n", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(putBody, `ac:name="code"`) {
		t.Errorf("expected mermaid code-macro fallback, got: %s", putBody)
	}
	if strings.Contains(putBody, "<!--mermaid:0-->") {
		t.Errorf("placeholder leaked into body: %s", putBody)
	}
}

func TestUpdatePageMarkdownMermaidUploadsAndReferences(t *testing.T) {
	var putBody string
	var uploaded bool
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/child/attachment"):
			uploaded = true
			io.WriteString(w, `{"results":[{"id":"att1"}]}`)
		case r.Method == http.MethodGet:
			io.WriteString(w, `{"title":"T","space":{"key":"DEV"},"version":{"number":1}}`)
		case r.Method == http.MethodPut:
			b, _ := io.ReadAll(r.Body)
			putBody = string(b)
			io.WriteString(w, `{"id":"1"}`)
		}
	})
	c, srv := newTestClient(h)
	defer srv.Close()

	render := func(string) ([]byte, error) { return []byte("\x89PNG..."), nil }
	_, err := c.UpdatePageMarkdown("1", "```mermaid\ngraph TD;A-->B;\n```\n", "", render)
	if err != nil {
		t.Fatal(err)
	}
	if !uploaded {
		t.Error("expected an attachment upload")
	}
	if !strings.Contains(putBody, `<ri:attachment ri:filename="mermaid-0.png"/>`) {
		t.Errorf("expected image reference to uploaded diagram, got: %s", putBody)
	}
}

func TestAddCommentMarkdownConverts(t *testing.T) {
	var postBody string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		postBody = string(b)
		io.WriteString(w, `{"id":"c1"}`)
	})
	c, srv := newTestClient(h)
	defer srv.Close()

	if _, err := c.AddCommentMarkdown("1", "**hi** there"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(postBody, "<strong>hi</strong>") {
		t.Errorf("comment not converted to storage: %s", postBody)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/confluence/ -run Markdown`
Expected: FAIL — `undefined: (*Client).UpdatePageMarkdown`.

- [ ] **Step 3: Write the implementation**

Create `internal/confluence/markdown_pages.go`:
```go
package confluence

import (
	"encoding/json"
	"fmt"

	"dinhphu28/atlassian-mcp/internal/markdown"
)

// MermaidRenderer renders Mermaid source to image bytes (PNG). A nil renderer,
// or one that returns an error, causes the diagram to fall back to a code macro.
type MermaidRenderer func(source string) ([]byte, error)

// CreatePageMarkdown creates a page from Markdown. Mermaid diagrams are rendered
// and uploaded as attachments; if that is not possible they degrade to code
// macros. Because attachments require an existing page, a page with diagrams is
// created first (with code-macro fallbacks) and then patched to reference the
// uploaded images.
func (c *Client) CreatePageMarkdown(spaceKey, title, md, parentID string, render MermaidRenderer) (string, error) {
	storage, diagrams, err := markdown.ToStorage(md)
	if err != nil {
		return "", err
	}

	initial := applyDiagrams(storage, diagrams, nil)
	raw, err := c.CreatePage(spaceKey, title, initial, parentID, "storage")
	if err != nil {
		return "", err
	}
	if len(diagrams) == 0 || render == nil {
		return raw, nil
	}

	pageID := parseID(raw)
	if pageID == "" {
		return raw, nil
	}
	filenames := renderAndUpload(c, pageID, diagrams, render)
	if len(filenames) == 0 {
		return raw, nil
	}
	final := applyDiagrams(storage, diagrams, filenames)
	return c.UpdatePage(pageID, final, title, "storage")
}

// UpdatePageMarkdown updates a page from Markdown, rendering and uploading any
// Mermaid diagrams to the existing page before the version bump.
func (c *Client) UpdatePageMarkdown(pageID, md, title string, render MermaidRenderer) (string, error) {
	storage, diagrams, err := markdown.ToStorage(md)
	if err != nil {
		return "", err
	}
	filenames := map[string]string{}
	if len(diagrams) > 0 && render != nil {
		filenames = renderAndUpload(c, pageID, diagrams, render)
	}
	final := applyDiagrams(storage, diagrams, filenames)
	return c.UpdatePage(pageID, final, title, "storage")
}

// applyDiagrams substitutes each diagram placeholder with either an image
// reference (when a filename was uploaded) or a code-macro fallback.
func applyDiagrams(storage string, diagrams []markdown.Diagram, filenames map[string]string) string {
	for _, d := range diagrams {
		var repl string
		if fn, ok := filenames[d.Placeholder]; ok {
			repl = markdown.AttachmentImage(fn)
		} else {
			repl = markdown.CodeMacro("mermaid", d.Source)
		}
		storage = replaceOnce(storage, d.Placeholder, repl)
	}
	return storage
}

// renderAndUpload renders each diagram and uploads successful renders as
// attachments, returning placeholder -> filename for those that succeeded.
func renderAndUpload(c *Client, pageID string, diagrams []markdown.Diagram, render MermaidRenderer) map[string]string {
	filenames := map[string]string{}
	for i, d := range diagrams {
		png, err := render(d.Source)
		if err != nil {
			continue
		}
		name := fmt.Sprintf("mermaid-%d.png", i)
		if _, err := c.UploadAttachment(pageID, name, png); err != nil {
			continue
		}
		filenames[d.Placeholder] = name
	}
	return filenames
}

// AddCommentMarkdown posts a comment authored in Markdown. Mermaid blocks are
// stored as code macros (comments do not render diagrams to images).
func (c *Client) AddCommentMarkdown(pageID, md string) (string, error) {
	storage, err := markdownComment(md)
	if err != nil {
		return "", err
	}
	return c.AddComment(pageID, storage, "storage")
}

// ReplyToCommentMarkdown replies to a comment using Markdown.
func (c *Client) ReplyToCommentMarkdown(parentCommentID, md string) (string, error) {
	storage, err := markdownComment(md)
	if err != nil {
		return "", err
	}
	return c.ReplyToComment(parentCommentID, storage, "storage")
}

// markdownComment converts comment Markdown to storage, degrading any Mermaid
// blocks to code macros (no image rendering).
func markdownComment(md string) (string, error) {
	storage, diagrams, err := markdown.ToStorage(md)
	if err != nil {
		return "", err
	}
	return applyDiagrams(storage, diagrams, nil), nil
}

func parseID(raw string) string {
	var v struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal([]byte(raw), &v)
	return v.ID
}

func replaceOnce(s, old, new string) string {
	i := indexOf(s, old)
	if i < 0 {
		return s
	}
	return s[:i] + new + s[i+len(old):]
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/confluence/ -run Markdown -v`
Expected: PASS for all three tests.

- [ ] **Step 5: Commit**

```bash
git add internal/confluence/markdown_pages.go internal/confluence/markdown_pages_test.go
git commit --no-gpg-sign -m "feat(confluence): create/update pages from Markdown with Mermaid upload"
```

---

## Task 6: Confluence read orchestration (Markdown get page / comments)

**Files:**
- Modify: `internal/confluence/markdown_pages.go`
- Test: `internal/confluence/markdown_pages_test.go`

**Interfaces:**
- Consumes: `markdown.ToMarkdown` (Task 3); existing `Client.get` via `GetPage`/`GetComments` raw JSON shape.
- Produces:
  - `type PageMarkdown struct { ID, Title, Space string; Version int; Markdown string }`
  - `func (c *Client) GetPageMarkdown(pageID string) (*PageMarkdown, error)`
  - `func (c *Client) GetCommentsMarkdown(pageID string, limit int) (string, error)`

- [ ] **Step 1: Write the failing test**

Append to `internal/confluence/markdown_pages_test.go`:
```go
func TestGetPageMarkdownConverts(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"id":"7","title":"T","space":{"key":"DEV"},"version":{"number":4},"body":{"storage":{"value":"<h2>Hi</h2>"}}}`)
	})
	c, srv := newTestClient(h)
	defer srv.Close()

	p, err := c.GetPageMarkdown("7")
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "7" || p.Title != "T" || p.Space != "DEV" || p.Version != 4 {
		t.Errorf("bad metadata: %+v", p)
	}
	if !strings.Contains(p.Markdown, "## Hi") {
		t.Errorf("body not converted: %q", p.Markdown)
	}
}

func TestGetCommentsMarkdownConverts(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"results":[{"body":{"storage":{"value":"<p>hello</p>"}}}]}`)
	})
	c, srv := newTestClient(h)
	defer srv.Close()

	got, err := c.GetCommentsMarkdown("7", 25)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "hello") {
		t.Errorf("comment body not converted: %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/confluence/ -run 'GetPageMarkdown|GetCommentsMarkdown'`
Expected: FAIL — `undefined: (*Client).GetPageMarkdown`.

- [ ] **Step 3: Write the implementation**

Append to `internal/confluence/markdown_pages.go` (add `"strings"` to the import block):
```go
// PageMarkdown is a page rendered as Markdown plus the metadata needed to
// update it without a second fetch.
type PageMarkdown struct {
	ID       string
	Title    string
	Space    string
	Version  int
	Markdown string
}

// GetPageMarkdown fetches a page and returns its body converted to Markdown.
func (c *Client) GetPageMarkdown(pageID string) (*PageMarkdown, error) {
	raw, err := c.GetPage(pageID)
	if err != nil {
		return nil, err
	}

	var p struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Space struct {
			Key string `json:"key"`
		} `json:"space"`
		Version struct {
			Number int `json:"number"`
		} `json:"version"`
		Body struct {
			Storage struct {
				Value string `json:"value"`
			} `json:"storage"`
		} `json:"body"`
	}
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return nil, fmt.Errorf("cannot parse page %s: %w", pageID, err)
	}

	md, err := markdown.ToMarkdown(p.Body.Storage.Value)
	if err != nil {
		return nil, err
	}

	return &PageMarkdown{
		ID:       p.ID,
		Title:    p.Title,
		Space:    p.Space.Key,
		Version:  p.Version.Number,
		Markdown: md,
	}, nil
}

// GetCommentsMarkdown fetches a page's comments and returns them as a Markdown
// list, each comment's body converted from storage format.
func (c *Client) GetCommentsMarkdown(pageID string, limit int) (string, error) {
	raw, err := c.GetComments(pageID, limit)
	if err != nil {
		return "", err
	}

	var resp struct {
		Results []struct {
			Body struct {
				Storage struct {
					Value string `json:"value"`
				} `json:"storage"`
			} `json:"body"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return "", fmt.Errorf("cannot parse comments for %s: %w", pageID, err)
	}

	var b strings.Builder
	for i, cm := range resp.Results {
		md, err := markdown.ToMarkdown(cm.Body.Storage.Value)
		if err != nil {
			return "", err
		}
		if i > 0 {
			b.WriteString("\n\n---\n\n")
		}
		b.WriteString(md)
	}
	return b.String(), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/confluence/ -v`
Expected: PASS (all confluence tests, including Task 5's).

- [ ] **Step 5: Commit**

```bash
git add internal/confluence/markdown_pages.go internal/confluence/markdown_pages_test.go
git commit --no-gpg-sign -m "feat(confluence): read pages and comments as Markdown"
```

---

## Task 7: Wire Markdown into the write tools

**Files:**
- Modify: `internal/mcpserver/confluence_write.go`

**Interfaces:**
- Consumes: `confluence.CreatePageMarkdown`, `confluence.UpdatePageMarkdown`, `confluence.MermaidRenderer` (Task 5); `mermaid.Available`, `mermaid.Render` (Task 4).
- Produces: no new exported Go symbols; changes tool behavior — `representation` defaults to `"markdown"`, new optional `file_path` on `confluence_create_page` and `confluence_update_page`.

- [ ] **Step 1: Add a Mermaid renderer helper**

At the top of `internal/mcpserver/confluence_write.go`, add the `mermaid` import and a helper after the imports:
```go
import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"dinhphu28/atlassian-mcp/internal/confluence"
	"dinhphu28/atlassian-mcp/internal/mermaid"
)

// mermaidRenderer returns a renderer when mmdc is available, else nil so that
// diagrams degrade to code macros.
func mermaidRenderer() confluence.MermaidRenderer {
	if !mermaid.Available() {
		return nil
	}
	return mermaid.Render
}

// readContent returns the body to publish: the file contents when filePath is
// set, otherwise the inline content.
func readContent(inline, filePath string) (string, error) {
	if filePath == "" {
		return inline, nil
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
```

- [ ] **Step 2: Update `confluence_create_page` registration**

Replace the `createPageTool := mcp.NewTool(...)` definition and its handler (lines 16–45 of the current file) with:
```go
	createPageTool := mcp.NewTool(
		"confluence_create_page",
		mcp.WithDescription("Create a new Confluence page. Body is Markdown by default; "+
			"```mermaid blocks are rendered to images when mmdc is installed."),
		mcp.WithString("space_key", mcp.Required(), mcp.Description("Key of the space to create the page in")),
		mcp.WithString("title", mcp.Required(), mcp.Description("Page title")),
		mcp.WithString("content", mcp.Description("Page body in the given representation (Markdown by default). Ignored when file_path is set.")),
		mcp.WithString("file_path", mcp.Description("Optional path to a local Markdown file to publish instead of inline content")),
		mcp.WithString("parent_id", mcp.Description("Optional parent page ID to nest under")),
		mcp.WithString("representation", mcp.Description("Body format: 'markdown' (default), 'storage', or 'wiki'")),
	)

	s.AddTool(createPageTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		spaceKey, err := request.RequireString("space_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		title, err := request.RequireString("title")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		content, err := readContent(request.GetString("content", ""), request.GetString("file_path", ""))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		representation := request.GetString("representation", "markdown")
		parentID := request.GetString("parent_id", "")
		if representation == "markdown" {
			return jsonResult(client.CreatePageMarkdown(spaceKey, title, content, parentID, mermaidRenderer()))
		}
		return jsonResult(client.CreatePage(spaceKey, title, content, parentID, representation))
	})
```

- [ ] **Step 3: Update `confluence_update_page` registration**

Replace the `updatePageTool := mcp.NewTool(...)` definition and its handler (lines 47–71 of the current file) with:
```go
	updatePageTool := mcp.NewTool(
		"confluence_update_page",
		mcp.WithDescription("Update an existing Confluence page (version is bumped automatically). "+
			"Body is Markdown by default; ```mermaid blocks are rendered to images when mmdc is installed."),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("Confluence page ID")),
		mcp.WithString("content", mcp.Description("New page body in the given representation (Markdown by default). Ignored when file_path is set.")),
		mcp.WithString("file_path", mcp.Description("Optional path to a local Markdown file to publish instead of inline content")),
		mcp.WithString("title", mcp.Description("New title (keeps the existing title if omitted)")),
		mcp.WithString("representation", mcp.Description("Body format: 'markdown' (default), 'storage', or 'wiki'")),
	)

	s.AddTool(updatePageTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		content, err := readContent(request.GetString("content", ""), request.GetString("file_path", ""))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		representation := request.GetString("representation", "markdown")
		title := request.GetString("title", "")
		if representation == "markdown" {
			return jsonResult(client.UpdatePageMarkdown(pageID, content, title, mermaidRenderer()))
		}
		return jsonResult(client.UpdatePage(pageID, content, title, representation))
	})
```

- [ ] **Step 4: Update `confluence_add_comment` registration**

Replace the `addCommentTool := mcp.NewTool(...)` definition and its handler (lines 73–95 of the current file) with:
```go
	addCommentTool := mcp.NewTool(
		"confluence_add_comment",
		mcp.WithDescription("Add a comment to a Confluence page. Body is Markdown by default."),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("Confluence page ID to comment on")),
		mcp.WithString("content", mcp.Required(), mcp.Description("Comment body in the given representation (Markdown by default)")),
		mcp.WithString("representation", mcp.Description("Body format: 'markdown' (default), 'storage', or 'wiki'")),
	)

	s.AddTool(addCommentTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		content, err := request.RequireString("content")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		representation := request.GetString("representation", "markdown")
		if representation == "markdown" {
			return jsonResult(client.AddCommentMarkdown(pageID, content))
		}
		return jsonResult(client.AddComment(pageID, content, representation))
	})
```

- [ ] **Step 5: Update `confluence_reply_to_comment` registration**

Replace the `replyToCommentTool := mcp.NewTool(...)` definition and its handler (lines 181–203 of the current file) with:
```go
	replyToCommentTool := mcp.NewTool(
		"confluence_reply_to_comment",
		mcp.WithDescription("Reply to an existing Confluence comment. Body is Markdown by default."),
		mcp.WithString("parent_comment_id", mcp.Required(), mcp.Description("ID of the comment to reply to")),
		mcp.WithString("content", mcp.Required(), mcp.Description("Reply body in the given representation (Markdown by default)")),
		mcp.WithString("representation", mcp.Description("Body format: 'markdown' (default), 'storage', or 'wiki'")),
	)

	s.AddTool(replyToCommentTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		parentCommentID, err := request.RequireString("parent_comment_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		content, err := request.RequireString("content")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		representation := request.GetString("representation", "markdown")
		if representation == "markdown" {
			return jsonResult(client.ReplyToCommentMarkdown(parentCommentID, content))
		}
		return jsonResult(client.ReplyToComment(parentCommentID, content, representation))
	})
```

- [ ] **Step 6: Build and run the full suite**

Run: `go build ./... && go test ./...`
Expected: build succeeds; all tests pass. (`filepath` is imported because the existing `uploadAttachmentTool` handler already uses `filepath.Base`; keep it.)

- [ ] **Step 7: Vet**

Run: `go vet ./internal/mcpserver/`
Expected: no diagnostics.

- [ ] **Step 8: Commit**

```bash
git add internal/mcpserver/confluence_write.go
git commit --no-gpg-sign -m "feat(mcp): Markdown-default writes (pages + comments) with file_path"
```

---

## Task 8: Wire Markdown into the read tools

**Files:**
- Modify: `internal/mcpserver/confluence_read.go`

**Interfaces:**
- Consumes: `confluence.GetPageMarkdown`, `confluence.GetCommentsMarkdown` (Task 6).
- Produces: no new exported Go symbols; `confluence_get_page` and `confluence_get_comments` return Markdown by default, add `representation` (`"storage"` opt-out) and `output_path`.

- [ ] **Step 1: Update `confluence_get_page` registration**

Replace the `getPageTool := mcp.NewTool(...)` definition and its handler (lines 40–53 of the current file) with:
```go
	getPageTool := mcp.NewTool(
		"confluence_get_page",
		mcp.WithDescription("Get a Confluence page by ID. Returns the body as Markdown by default."),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("Confluence page ID")),
		mcp.WithString("representation", mcp.Description("Body format: 'markdown' (default) or 'storage' (raw JSON)")),
		mcp.WithString("output_path", mcp.Description("Optional path to write the Markdown to; returns metadata instead of the body")),
	)

	s.AddTool(getPageTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if request.GetString("representation", "markdown") == "storage" {
			return jsonResult(client.GetPage(pageID))
		}

		page, err := client.GetPageMarkdown(pageID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if out := request.GetString("output_path", ""); out != "" {
			if err := os.WriteFile(out, []byte(page.Markdown), 0o644); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf(
				"Wrote page %s (title %q, space %s, version %d) to %s",
				page.ID, page.Title, page.Space, page.Version, out)), nil
		}

		return mcp.NewToolResultText(page.Markdown), nil
	})
```

- [ ] **Step 2: Update `confluence_get_comments` registration**

Replace the `getCommentsTool := mcp.NewTool(...)` definition and its handler (lines 71–85 of the current file) with:
```go
	getCommentsTool := mcp.NewTool(
		"confluence_get_comments",
		mcp.WithDescription("Get the comments on a Confluence page. Returns Markdown by default."),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("Confluence page ID")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of comments (default 25)")),
		mcp.WithString("representation", mcp.Description("Body format: 'markdown' (default) or 'storage' (raw JSON)")),
	)

	s.AddTool(getCommentsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		limit := request.GetInt("limit", 25)

		if request.GetString("representation", "markdown") == "storage" {
			return jsonResult(client.GetComments(pageID, limit))
		}

		md, err := client.GetCommentsMarkdown(pageID, limit)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(md), nil
	})
```

- [ ] **Step 3: Confirm imports**

The handler uses `os` and `fmt`, both already imported in `confluence_read.go` (`fmt` is present; add `"os"` to the import block if the build reports it missing).

- [ ] **Step 4: Build and run the full suite**

Run: `go build ./... && go test ./...`
Expected: build succeeds; all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/mcpserver/confluence_read.go
git commit --no-gpg-sign -m "feat(mcp): Markdown-default reads with storage opt-out and output_path"
```

---

## Task 9: Document the Markdown workflow

**Files:**
- Modify: `README.md`

**Interfaces:**
- Consumes: behavior from Tasks 7–8.
- Produces: user-facing documentation.

- [ ] **Step 1: Read the current README to find the Confluence tools section**

Run: `grep -n "confluence_" README.md | head`
Expected: locate the Confluence tool listing.

- [ ] **Step 2: Add a "Markdown workflow" subsection**

Add a subsection near the Confluence tools documenting:
```markdown
### Markdown workflow (token-efficient)

Confluence page and comment tools use **Markdown** by default instead of raw
storage XHTML, cutting token usage on both reads and writes.

- **Write inline:** pass Markdown as `content` to `confluence_create_page` /
  `confluence_update_page`.
- **Write from a file:** pass `file_path` to publish a local `.md` file. Ideal
  for large pages — edit the file surgically, then push.
- **Read to a file:** pass `output_path` to `confluence_get_page` to write the
  Markdown to disk (returns metadata only), so large pages never fill context.
- **Exact fidelity:** pass `representation="storage"` on reads/writes to bypass
  Markdown conversion.

**Images:** `![alt](name.png)` references a page attachment (upload it first
with `confluence_upload_attachment`); `![alt](https://…)` embeds an external URL.

**Mermaid:** ` ```mermaid ` blocks are rendered to PNG and uploaded as
attachments **if the `mmdc` CLI (`@mermaid-js/mermaid-cli`) is on PATH**. Without
it, the block is stored as a code macro. Rendering is one-way — reads return the
image, not the Mermaid source.
```

- [ ] **Step 3: Verify the docs build/read cleanly**

Run: `grep -n "Markdown workflow" README.md`
Expected: the new subsection is present.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit --no-gpg-sign -m "docs: document the Markdown Confluence workflow"
```

---

## Final verification

- [ ] **Full build and test**

Run: `go build ./... && go test ./... && go vet ./...`
Expected: all pass, no vet diagnostics.

- [ ] **Confirm token-loop end to end (manual, requires a live instance + mmdc optional)**

1. `confluence_get_page(page_id=<real>, output_path="/tmp/p.md")` → writes Markdown, returns metadata.
2. Edit `/tmp/p.md`.
3. `confluence_update_page(page_id=<real>, file_path="/tmp/p.md")` → page updated; verify in the UI.
