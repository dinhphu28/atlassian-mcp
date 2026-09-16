package confluence

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// bodyField builds the Confluence REST `body` payload for the given content
// representation (e.g. "storage" or "wiki").
func bodyField(representation, content string) map[string]any {
	return map[string]any{
		representation: map[string]any{
			"value":          content,
			"representation": representation,
		},
	}
}

// marshalPayload JSON-encodes v without HTML-escaping so that storage-format
// XHTML (e.g. <h2>, <ac:image>) is transmitted verbatim rather than as <
// escapes.
func marshalPayload(v any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return bytes.TrimRight(buf.Bytes(), "\n")
}

// Search runs a CQL query and returns up to limit results.
func (c *Client) Search(cql string, limit int) (string, error) {
	path := fmt.Sprintf("/rest/api/content/search?limit=%d&expand=space,version&cql=%s",
		limit, url.QueryEscape(cql))
	return c.get(path)
}

// GetPage returns a single page by ID, including its storage-format body.
func (c *Client) GetPage(pageID string) (string, error) {
	path := "/rest/api/content/" + url.PathEscape(pageID) +
		"?expand=space,version,body.storage"
	return c.get(path)
}

// PageStorage is a page's storage-format body together with the metadata a
// caller needs to report it or re-publish it.
type PageStorage struct {
	ID      string
	Title   string
	Space   string
	Version int
	Storage string
}

// GetPageStorage returns a page's storage-format body decoded out of the REST
// envelope, so callers receive the XHTML itself rather than a JSON-escaped
// string they would have to unescape before editing.
func (c *Client) GetPageStorage(pageID string) (*PageStorage, error) {
	raw, err := c.GetPage(pageID)
	if err != nil {
		return nil, err
	}

	var page struct {
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
	if err := json.Unmarshal([]byte(raw), &page); err != nil {
		return nil, fmt.Errorf("cannot parse page %s: %w", pageID, err)
	}

	return &PageStorage{
		ID:      page.ID,
		Title:   page.Title,
		Space:   page.Space.Key,
		Version: page.Version.Number,
		Storage: page.Body.Storage.Value,
	}, nil
}

// GetPageChildren lists the child pages directly under a page.
func (c *Client) GetPageChildren(pageID string, limit int) (string, error) {
	path := fmt.Sprintf("/rest/api/content/%s/child/page?limit=%d&expand=space,version",
		url.PathEscape(pageID), limit)
	return c.get(path)
}

// GetComments returns the comments on a page. depth=all includes nested replies
// (children of comments), not just top-level comments; history carries each
// comment's author and created date, and ancestors gives its reply chain.
func (c *Client) GetComments(pageID string, limit int) (string, error) {
	path := fmt.Sprintf("/rest/api/content/%s/child/comment?limit=%d&depth=all&expand=body.storage,version,history,ancestors",
		url.PathEscape(pageID), limit)
	return c.get(path)
}

// CreatePage creates a new page. parentID is optional (empty for a top-level
// page). representation is the body format, e.g. "storage" or "wiki".
func (c *Client) CreatePage(spaceKey, title, content, parentID, representation string) (string, error) {
	payload := map[string]any{
		"type":  "page",
		"title": title,
		"space": map[string]any{"key": spaceKey},
		"body":  bodyField(representation, content),
	}
	if parentID != "" {
		payload["ancestors"] = []map[string]any{{"id": parentID}}
	}

	body := marshalPayload(payload)
	return c.do(http.MethodPost, "/rest/api/content", string(body))
}

// UpdatePage updates an existing page, bumping whatever version the server
// currently holds. An empty title keeps the existing one.
func (c *Client) UpdatePage(pageID, content, title, representation string) (string, error) {
	return c.UpdatePageAt(pageID, content, title, representation, 0)
}

// UpdatePageAt updates an existing page. The current space (and title, when
// none is given) are fetched automatically.
//
// expectedVersion is the version the edit was derived from: when it is greater
// than zero the write is sent as that version plus one, so Confluence rejects it
// if the page has moved on in the meantime instead of silently overwriting
// someone else's edit. Zero keeps the fetch-and-bump behaviour, which always
// wins the race.
func (c *Client) UpdatePageAt(pageID, content, title, representation string, expectedVersion int) (string, error) {
	raw, err := c.get("/rest/api/content/" + url.PathEscape(pageID) + "?expand=version,space")
	if err != nil {
		return "", err
	}

	var current struct {
		Title string `json:"title"`
		Space struct {
			Key string `json:"key"`
		} `json:"space"`
		Version struct {
			Number int `json:"number"`
		} `json:"version"`
	}
	if err := json.Unmarshal([]byte(raw), &current); err != nil {
		return "", fmt.Errorf("cannot parse current page: %w", err)
	}

	if title == "" {
		title = current.Title
	}

	payload := map[string]any{
		"id":      pageID,
		"type":    "page",
		"title":   title,
		"space":   map[string]any{"key": current.Space.Key},
		"version": map[string]any{"number": nextVersion(current.Version.Number, expectedVersion)},
		"body":    bodyField(representation, content),
	}

	body := marshalPayload(payload)
	res, err := c.do(http.MethodPut, "/rest/api/content/"+url.PathEscape(pageID), string(body))
	return res, versionConflict(err, expectedVersion)
}

// RenamePage changes only a page's title, re-sending the body Confluence already
// holds so the page is not blanked by an update that carries no content.
func (c *Client) RenamePage(pageID, title string) (string, error) {
	raw, err := c.get("/rest/api/content/" + url.PathEscape(pageID) + "?expand=version,space,body.storage")
	if err != nil {
		return "", err
	}

	var current struct {
		Space struct {
			Key string `json:"key"`
		} `json:"space"`
		Version struct {
			Number int `json:"number"`
		} `json:"version"`
		Body struct {
			Storage struct {
				Value          string `json:"value"`
				Representation string `json:"representation"`
			} `json:"storage"`
		} `json:"body"`
	}
	if err := json.Unmarshal([]byte(raw), &current); err != nil {
		return "", fmt.Errorf("cannot parse current page: %w", err)
	}

	payload := map[string]any{
		"id":      pageID,
		"type":    "page",
		"title":   title,
		"space":   map[string]any{"key": current.Space.Key},
		"version": map[string]any{"number": current.Version.Number + 1},
		"body":    bodyField(current.Body.Storage.Representation, current.Body.Storage.Value),
	}

	body := marshalPayload(payload)
	return c.do(http.MethodPut, "/rest/api/content/"+url.PathEscape(pageID), string(body))
}

// nextVersion is the version number to send for an update: one past the version
// the caller based the edit on, or one past the server's current version when
// the caller did not say.
func nextVersion(serverVersion, expectedVersion int) int {
	if expectedVersion > 0 {
		return expectedVersion + 1
	}
	return serverVersion + 1
}

// versionConflict rewrites Confluence's 409 into an actionable message: the
// content changed under the caller, so the edit has to be rebased on a fresh
// read rather than retried as-is.
func versionConflict(err error, expectedVersion int) error {
	if err == nil || expectedVersion <= 0 || !strings.Contains(err.Error(), "confluence error 409") {
		return err
	}
	return fmt.Errorf("content changed since version %d; re-read it and redo the edit: %w", expectedVersion, err)
}

// AddComment posts a comment on a page.
func (c *Client) AddComment(pageID, content, representation string) (string, error) {
	payload := map[string]any{
		"type":      "comment",
		"container": map[string]any{"id": pageID, "type": "page"},
		"body":      bodyField(representation, content),
	}

	body := marshalPayload(payload)
	return c.do(http.MethodPost, "/rest/api/content", string(body))
}

// DeletePage deletes a page by ID (moves it to the trash).
func (c *Client) DeletePage(pageID string) error {
	_, err := c.do(http.MethodDelete, "/rest/api/content/"+url.PathEscape(pageID), "")
	return err
}

// UpdateComment edits an existing comment, bumping whatever version the server
// currently holds. representation is the body format, e.g. "storage".
func (c *Client) UpdateComment(commentID, content, representation string) (string, error) {
	return c.UpdateCommentAt(commentID, content, representation, 0)
}

// UpdateCommentAt edits an existing comment. expectedVersion has the same
// meaning as in UpdatePageAt: greater than zero makes the write fail rather
// than overwrite a comment that changed since it was read.
func (c *Client) UpdateCommentAt(commentID, content, representation string, expectedVersion int) (string, error) {
	raw, err := c.get("/rest/api/content/" + url.PathEscape(commentID) + "?expand=version")
	if err != nil {
		return "", err
	}

	var current struct {
		Version struct {
			Number int `json:"number"`
		} `json:"version"`
	}
	if err := json.Unmarshal([]byte(raw), &current); err != nil {
		return "", fmt.Errorf("cannot parse current comment %s: %w", commentID, err)
	}

	payload := map[string]any{
		"id":      commentID,
		"type":    "comment",
		"version": map[string]any{"number": nextVersion(current.Version.Number, expectedVersion)},
		"body":    bodyField(representation, content),
	}

	body := marshalPayload(payload)
	res, err := c.do(http.MethodPut, "/rest/api/content/"+url.PathEscape(commentID), string(body))
	return res, versionConflict(err, expectedVersion)
}

// DeleteComment deletes a comment by ID.
func (c *Client) DeleteComment(commentID string) error {
	_, err := c.do(http.MethodDelete, "/rest/api/content/"+url.PathEscape(commentID), "")
	return err
}

// GetLabels lists the labels on a page.
func (c *Client) GetLabels(pageID string) (string, error) {
	return c.get("/rest/api/content/" + url.PathEscape(pageID) + "/label")
}

// AddLabel adds a global label to a page.
func (c *Client) AddLabel(pageID, name string) (string, error) {
	body, _ := json.Marshal([]map[string]any{{"prefix": "global", "name": name}})
	return c.do(http.MethodPost, "/rest/api/content/"+url.PathEscape(pageID)+"/label", string(body))
}

// GetPageHistory returns the version history of a page.
func (c *Client) GetPageHistory(pageID string, limit int) (string, error) {
	path := fmt.Sprintf("/rest/api/content/%s/version?limit=%d", url.PathEscape(pageID), limit)
	return c.get(path)
}

// MovePage re-parents a page under targetParentID, preserving its title and
// content, bumping whatever version the server currently holds.
func (c *Client) MovePage(pageID, targetParentID string) (string, error) {
	return c.MovePageAt(pageID, targetParentID, 0)
}

// MovePageAt re-parents a page. expectedVersion has the same meaning as in
// UpdatePageAt: greater than zero makes the move fail rather than re-publish a
// body that changed since it was read.
func (c *Client) MovePageAt(pageID, targetParentID string, expectedVersion int) (string, error) {
	raw, err := c.get("/rest/api/content/" + url.PathEscape(pageID) + "?expand=version,space,body.storage")
	if err != nil {
		return "", err
	}

	var current struct {
		Title string `json:"title"`
		Space struct {
			Key string `json:"key"`
		} `json:"space"`
		Version struct {
			Number int `json:"number"`
		} `json:"version"`
		Body struct {
			Storage struct {
				Value          string `json:"value"`
				Representation string `json:"representation"`
			} `json:"storage"`
		} `json:"body"`
	}
	if err := json.Unmarshal([]byte(raw), &current); err != nil {
		return "", fmt.Errorf("cannot parse current page: %w", err)
	}

	payload := map[string]any{
		"id":        pageID,
		"type":      "page",
		"title":     current.Title,
		"space":     map[string]any{"key": current.Space.Key},
		"version":   map[string]any{"number": nextVersion(current.Version.Number, expectedVersion)},
		"ancestors": []map[string]any{{"id": targetParentID}},
		"body":      bodyField(current.Body.Storage.Representation, current.Body.Storage.Value),
	}

	body := marshalPayload(payload)
	res, err := c.do(http.MethodPut, "/rest/api/content/"+url.PathEscape(pageID), string(body))
	return res, versionConflict(err, expectedVersion)
}

// ReplyToComment posts a reply to an existing comment. The parent comment's
// container (the page) is resolved automatically.
func (c *Client) ReplyToComment(parentCommentID, content, representation string) (string, error) {
	raw, err := c.get("/rest/api/content/" + url.PathEscape(parentCommentID) + "?expand=container")
	if err != nil {
		return "", err
	}

	var parent struct {
		Container struct {
			ID string `json:"id"`
		} `json:"container"`
	}
	if err := json.Unmarshal([]byte(raw), &parent); err != nil {
		return "", fmt.Errorf("cannot parse parent comment %s: %w", parentCommentID, err)
	}
	if parent.Container.ID == "" {
		return "", fmt.Errorf("cannot determine the page for comment %s", parentCommentID)
	}

	payload := map[string]any{
		"type":      "comment",
		"container": map[string]any{"id": parent.Container.ID, "type": "page"},
		"ancestors": []map[string]any{{"id": parentCommentID}},
		"body":      bodyField(representation, content),
	}

	body := marshalPayload(payload)
	return c.do(http.MethodPost, "/rest/api/content", string(body))
}
