package confluence

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dinhphu28/atlassian-mcp/internal/markdown"
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

	_, _, err := c.UpdatePageMarkdown("123", "## Hi\n\n- x\n", "", nil)
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

	_, _, err := c.UpdatePageMarkdown("1", "```mermaid\ngraph TD;A-->B;\n```\n", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	// putBody is JSON, so storage-format double quotes are escaped as \".
	if !strings.Contains(putBody, `ac:name=\"code\"`) {
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
	_, _, err := c.UpdatePageMarkdown("1", "```mermaid\ngraph TD;A-->B;\n```\n", "", render)
	if err != nil {
		t.Fatal(err)
	}
	if !uploaded {
		t.Error("expected an attachment upload")
	}
	// Attachment name is a content hash of the diagram source, not its index.
	_, diagrams, _ := markdown.ToStorage("```mermaid\ngraph TD;A-->B;\n```\n")
	want := diagramFilename(diagrams[0].Source)
	if !strings.Contains(putBody, `<ri:attachment ri:filename=\"`+want+`\"/>`) {
		t.Errorf("expected image reference to %s, got: %s", want, putBody)
	}
}

func TestRenderAndUploadWarnsOnRenderError(t *testing.T) {
	c, srv := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"results":[{"id":"att1"}]}`)
	}))
	defer srv.Close()

	diagrams := []markdown.Diagram{{Placeholder: "<!--mermaid:0-->", Source: "graph TD;A-->B;"}}
	render := func(string) ([]byte, error) { return nil, fmt.Errorf("boom") }

	filenames, warnings := renderAndUpload(c, "1", diagrams, render, nil)
	if len(filenames) != 0 {
		t.Errorf("expected no image on render error, got %v", filenames)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "render failed") {
		t.Errorf("expected a render-failed warning, got %v", warnings)
	}
}

func TestRenderAndUploadWarnsOnUploadError(t *testing.T) {
	c, srv := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, `denied`)
	}))
	defer srv.Close()

	diagrams := []markdown.Diagram{{Placeholder: "<!--mermaid:0-->", Source: "graph TD;A-->B;"}}
	render := func(string) ([]byte, error) { return []byte("\x89PNG"), nil }

	filenames, warnings := renderAndUpload(c, "1", diagrams, render, nil)
	if len(filenames) != 0 {
		t.Errorf("expected no image on upload error, got %v", filenames)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "upload failed") {
		t.Errorf("expected an upload-failed warning, got %v", warnings)
	}
}

// A transient render failure on update must not downgrade a page that already
// has a working image: the existing attachment is reused instead.
func TestUpdatePageMarkdownReusesExistingImageOnRenderFailure(t *testing.T) {
	md := "```mermaid\ngraph TD;A-->B;\n```\n"
	_, diagrams, _ := markdown.ToStorage(md)
	name := diagramFilename(diagrams[0].Source)

	var putBody string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/child/attachment"):
			// The page already carries the image for this diagram.
			io.WriteString(w, `{"results":[{"title":"`+name+`"}]}`)
		case r.Method == http.MethodGet:
			io.WriteString(w, `{"title":"T","space":{"key":"DEV"},"version":{"number":3}}`)
		case r.Method == http.MethodPut:
			b, _ := io.ReadAll(r.Body)
			putBody = string(b)
			io.WriteString(w, `{"id":"1"}`)
		}
	})
	c, srv := newTestClient(h)
	defer srv.Close()

	failing := func(string) ([]byte, error) { return nil, fmt.Errorf("mmdc blew up") }
	_, warnings, err := c.UpdatePageMarkdown("1", md, "", failing)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(putBody, `<ri:attachment ri:filename=\"`+name+`\"/>`) {
		t.Errorf("expected existing image preserved, got: %s", putBody)
	}
	if strings.Contains(putBody, `ac:name=\"code\"`) {
		t.Errorf("render failure downgraded page to a code macro: %s", putBody)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "reused existing attachment") {
		t.Errorf("expected a reuse warning, got %v", warnings)
	}
}

// Filenames follow diagram content, not position: swapping the order of two
// diagrams keeps each source mapped to the same filename.
func TestDiagramFilenamesFollowContentNotOrder(t *testing.T) {
	graph := "```mermaid\ngraph TD;A-->B;\n```"
	seq := "```mermaid\nsequenceDiagram\nX->>Y: hi\n```"

	_, first, _ := markdown.ToStorage(graph + "\n\n" + seq + "\n")
	_, second, _ := markdown.ToStorage(seq + "\n\n" + graph + "\n")

	graphName := diagramFilename(first[0].Source) // graph at index 0
	seqName := diagramFilename(first[1].Source)   // sequence at index 1

	if graphName == seqName {
		t.Fatal("distinct diagrams produced the same filename")
	}
	// After reordering, index 0 is the sequence diagram and index 1 the graph;
	// the filenames must track the source, not the index.
	if got := diagramFilename(second[0].Source); got != seqName {
		t.Errorf("sequence diagram filename changed with position: %s vs %s", got, seqName)
	}
	if got := diagramFilename(second[1].Source); got != graphName {
		t.Errorf("graph diagram filename changed with position: %s vs %s", got, graphName)
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
