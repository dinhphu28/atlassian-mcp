package confluence

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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

// GetPageChildren lists the child pages directly under a page.
func (c *Client) GetPageChildren(pageID string, limit int) (string, error) {
	path := fmt.Sprintf("/rest/api/content/%s/child/page?limit=%d&expand=space,version",
		url.PathEscape(pageID), limit)
	return c.get(path)
}

// GetComments returns the comments on a page.
func (c *Client) GetComments(pageID string, limit int) (string, error) {
	path := fmt.Sprintf("/rest/api/content/%s/child/comment?limit=%d&expand=body.storage,version",
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

// UpdatePage updates an existing page. The current version and space are fetched
// automatically and the version is bumped. An empty title keeps the existing one.
func (c *Client) UpdatePage(pageID, content, title, representation string) (string, error) {
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
		"version": map[string]any{"number": current.Version.Number + 1},
		"body":    bodyField(representation, content),
	}

	body := marshalPayload(payload)
	return c.do(http.MethodPut, "/rest/api/content/"+url.PathEscape(pageID), string(body))
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
// content. The current version is fetched and bumped automatically.
func (c *Client) MovePage(pageID, targetParentID string) (string, error) {
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
		"version":   map[string]any{"number": current.Version.Number + 1},
		"ancestors": []map[string]any{{"id": targetParentID}},
		"body":      bodyField(current.Body.Storage.Representation, current.Body.Storage.Value),
	}

	body := marshalPayload(payload)
	return c.do(http.MethodPut, "/rest/api/content/"+url.PathEscape(pageID), string(body))
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
