package confluence

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

const macroStorage = `<ac:structured-macro ac:name="info"><ac:rich-text-body><p>a &amp; b</p></ac:rich-text-body></ac:structured-macro>`

// GetPageStorage must hand back the XHTML itself, not the JSON-escaped string
// the REST envelope carries, so a caller can edit it directly.
func TestGetPageStorageDecodesBody(t *testing.T) {
	envelope, _ := json.Marshal(map[string]any{
		"id":      "42",
		"title":   "T",
		"space":   map[string]any{"key": "DEV"},
		"version": map[string]any{"number": 3},
		"body":    map[string]any{"storage": map[string]any{"value": macroStorage}},
	})

	c, srv := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(envelope)
	}))
	defer srv.Close()

	page, err := c.GetPageStorage("42")
	if err != nil {
		t.Fatal(err)
	}
	if page.Storage != macroStorage {
		t.Errorf("Storage = %q, want the raw XHTML %q", page.Storage, macroStorage)
	}
	if page.ID != "42" || page.Title != "T" || page.Space != "DEV" || page.Version != 3 {
		t.Errorf("metadata = %+v", page)
	}
}

// A rename carries no body of its own, so it has to re-send the one Confluence
// already holds; sending none would blank the page.
func TestRenamePagePreservesBody(t *testing.T) {
	var putBody string
	c, srv := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			io.WriteString(w, `{"title":"old","space":{"key":"DEV"},"version":{"number":4},`+
				`"body":{"storage":{"value":"<p>keep me</p>","representation":"storage"}}}`)
		case http.MethodPut:
			b, _ := io.ReadAll(r.Body)
			putBody = string(b)
			io.WriteString(w, `{"id":"1"}`)
		}
	}))
	defer srv.Close()

	if _, err := c.RenamePage("1", "new"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(putBody, `<p>keep me</p>`) {
		t.Errorf("rename dropped the body: %s", putBody)
	}
	if !strings.Contains(putBody, `"title":"new"`) {
		t.Errorf("rename did not set the new title: %s", putBody)
	}
	if !strings.Contains(putBody, `"number":5`) {
		t.Errorf("rename did not bump the version: %s", putBody)
	}
}

// expected_version has to be the version actually sent, otherwise Confluence's
// own conflict check is defeated by the fetch-and-bump.
func TestUpdatePageAtSendsTheCallersVersion(t *testing.T) {
	var putBody string
	c, srv := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			io.WriteString(w, `{"title":"T","space":{"key":"DEV"},"version":{"number":9}}`)
		case http.MethodPut:
			b, _ := io.ReadAll(r.Body)
			putBody = string(b)
			io.WriteString(w, `{"id":"1"}`)
		}
	}))
	defer srv.Close()

	if _, err := c.UpdatePageAt("1", "<p>x</p>", "", "storage", 4); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(putBody, `"number":5`) {
		t.Errorf("expected version 5 (4+1), got: %s", putBody)
	}

	// Without an expectation the server's version is bumped instead.
	if _, err := c.UpdatePageAt("1", "<p>x</p>", "", "storage", 0); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(putBody, `"number":10`) {
		t.Errorf("expected version 10 (9+1), got: %s", putBody)
	}
}

func TestUpdatePageAtExplainsAConflict(t *testing.T) {
	c, srv := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			io.WriteString(w, `{"title":"T","space":{"key":"DEV"},"version":{"number":9}}`)
			return
		}
		w.WriteHeader(http.StatusConflict)
		io.WriteString(w, `{"message":"version conflict"}`)
	}))
	defer srv.Close()

	_, err := c.UpdatePageAt("1", "<p>x</p>", "", "storage", 4)
	if err == nil {
		t.Fatal("expected a conflict error")
	}
	if !strings.Contains(err.Error(), "changed since version 4") {
		t.Errorf("conflict error is not actionable: %v", err)
	}

	// With no expectation the raw error is passed through unchanged.
	if _, err := c.UpdatePageAt("1", "<p>x</p>", "", "storage", 0); err == nil ||
		strings.Contains(err.Error(), "changed since version") {
		t.Errorf("unexpected rewrite without an expectation: %v", err)
	}
}

func TestResolveContentType(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		parentID string
		want     string
		wantErr  bool
	}{
		{name: "empty defaults to page", input: "", want: ContentTypePage},
		{name: "page", input: "page", want: ContentTypePage},
		{name: "page keeps its parent", input: "page", parentID: "42", want: ContentTypePage},
		{name: "blogpost", input: "blogpost", want: ContentTypeBlogpost},
		{name: "case and space are ignored", input: "  BlogPost ", want: ContentTypeBlogpost},
		{name: "blog is an alias", input: "blog", want: ContentTypeBlogpost},
		{name: "blogpost rejects a parent", input: "blogpost", parentID: "42", wantErr: true},
		{name: "unknown type", input: "comment", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveContentType(tt.input, tt.parentID)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveContentType(%q, %q) = %q, want an error", tt.input, tt.parentID, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveContentType(%q, %q) error: %v", tt.input, tt.parentID, err)
			}
			if got != tt.want {
				t.Errorf("resolveContentType(%q, %q) = %q, want %q", tt.input, tt.parentID, got, tt.want)
			}
		})
	}

	// The rejection has to say which argument to drop, since the caller cannot
	// tell from a Confluence 400 that blog posts simply have no ancestors.
	_, err := resolveContentType("blogpost", "42")
	if err == nil || !strings.Contains(err.Error(), "parent_id") {
		t.Errorf("error = %v, want it to name parent_id", err)
	}
}

func TestContentTypeOf(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "missing type falls back to page", input: "", want: ContentTypePage},
		{name: "page", input: "page", want: "page"},
		{name: "a blog post stays a blog post", input: "blogpost", want: "blogpost"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contentTypeOf(tt.input); got != tt.want {
				t.Errorf("contentTypeOf(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStartParam(t *testing.T) {
	tests := []struct {
		name  string
		input int
		want  string
	}{
		{name: "first page sends nothing", input: 0, want: ""},
		{name: "negative is treated as the first page", input: -5, want: ""},
		{name: "offset", input: 25, want: "&start=25"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := startParam(tt.input); got != tt.want {
				t.Errorf("startParam(%d) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestContentPath(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		version int
		want    string
	}{
		{
			name: "current content carries no status",
			id:   "42",
			want: "/rest/api/content/42?expand=space,version",
		},
		{
			// A version without status=historical is ignored by Confluence,
			// which would hand back the current body under an old number.
			name:    "historical revision",
			id:      "42",
			version: 3,
			want:    "/rest/api/content/42?expand=space,version&status=historical&version=3",
		},
		{
			name: "id is escaped",
			id:   "a b",
			want: "/rest/api/content/a%20b?expand=space,version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contentPath(tt.id, "space,version", tt.version); got != tt.want {
				t.Errorf("contentPath(%q, %d) = %q, want %q", tt.id, tt.version, got, tt.want)
			}
		})
	}
}
