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
