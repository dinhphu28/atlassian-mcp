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
	_, err := c.UpdatePageMarkdown("1", "```mermaid\ngraph TD;A-->B;\n```\n", "", render)
	if err != nil {
		t.Fatal(err)
	}
	if !uploaded {
		t.Error("expected an attachment upload")
	}
	// putBody is JSON, so storage-format double quotes are escaped as \".
	if !strings.Contains(putBody, `<ri:attachment ri:filename=\"mermaid-0.png\"/>`) {
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
