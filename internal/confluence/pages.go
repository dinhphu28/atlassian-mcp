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

// Confluence content types this client writes. A blog post is a separate type,
// not a page with a flag, and it has no ancestors.
const (
	ContentTypePage     = "page"
	ContentTypeBlogpost = "blogpost"
)

// resolveContentType normalises the content type a caller asked for. An empty
// value means a page, so existing callers keep their behaviour.
//
// A blog post lives at the root of its space and cannot be nested, so pairing
// one with a parent is rejected here: Confluence would otherwise answer with an
// opaque error that does not say which of the two arguments was wrong.
func resolveContentType(contentType, parentID string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "", ContentTypePage:
		return ContentTypePage, nil
	case ContentTypeBlogpost, "blog", "blog post":
		if parentID != "" {
			return "", fmt.Errorf("a %s has no parent page; omit parent_id, or use content_type %q",
				ContentTypeBlogpost, ContentTypePage)
		}
		return ContentTypeBlogpost, nil
	default:
		return "", fmt.Errorf("unknown content_type %q; use %q or %q",
			contentType, ContentTypePage, ContentTypeBlogpost)
	}
}

// contentTypeOf is the type an update has to echo back: whatever Confluence
// reports the content to be, defaulting to a page when the response carries no
// type. Hardcoding "page" would corrupt or fail an update to a blog post.
func contentTypeOf(current string) string {
	if current == "" {
		return ContentTypePage
	}
	return current
}

// startParam is the offset parameter for a paged list endpoint. The first page
// adds nothing, so requests that do not page are byte-for-byte what they were.
func startParam(start int) string {
	if start <= 0 {
		return ""
	}
	return fmt.Sprintf("&start=%d", start)
}

// contentPath builds the read path for one content item, selecting an older
// revision when version is greater than zero. Confluence serves a historical
// revision only when status=historical accompanies the version number; sending
// the version alone silently returns the current content instead.
func contentPath(contentID, expand string, version int) string {
	path := "/rest/api/content/" + url.PathEscape(contentID) + "?expand=" + expand
	if version > 0 {
		path += fmt.Sprintf("&status=historical&version=%d", version)
	}
	return path
}

// Search runs a CQL query and returns up to limit results, starting at the
// zero-based offset start (advance it by limit to follow _links.next).
func (c *Client) Search(cql string, limit, start int) (string, error) {
	path := fmt.Sprintf("/rest/api/content/search?limit=%d&expand=space,version&cql=%s%s",
		limit, url.QueryEscape(cql), startParam(start))
	return c.get(path)
}

// GetPage returns a single page by ID, including its storage-format body.
func (c *Client) GetPage(pageID string) (string, error) {
	return c.GetPageAt(pageID, 0)
}

// GetPageAt returns a page by ID including its storage-format body. A version
// greater than zero reads that historical revision (the version numbers come
// from GetPageHistory) rather than the current content.
func (c *Client) GetPageAt(pageID string, version int) (string, error) {
	return c.get(contentPath(pageID, "space,version,body.storage", version))
}

// PageStorage is a page's storage-format body together with the metadata a
// caller needs to report it or re-publish it.
type PageStorage struct {
	ID    string
	Title string
	Space string
	// Type is the content type Confluence reports, "page" or "blogpost", so a
	// caller re-publishing the body does not turn a blog post into a page.
	Type    string
	Version int
	Storage string
}

// GetPageStorage returns a page's storage-format body decoded out of the REST
// envelope, so callers receive the XHTML itself rather than a JSON-escaped
// string they would have to unescape before editing.
func (c *Client) GetPageStorage(pageID string) (*PageStorage, error) {
	return c.GetPageStorageAt(pageID, 0)
}

// GetPageStorageAt is GetPageStorage for one revision: a version greater than
// zero returns that historical body, which is what a restore does before
// writing it back with UpdatePage.
func (c *Client) GetPageStorageAt(pageID string, version int) (*PageStorage, error) {
	raw, err := c.GetPageAt(pageID, version)
	if err != nil {
		return nil, err
	}

	var page struct {
		ID    string `json:"id"`
		Type  string `json:"type"`
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
		Type:    page.Type,
		Title:   page.Title,
		Space:   page.Space.Key,
		Version: page.Version.Number,
		Storage: page.Body.Storage.Value,
	}, nil
}

// GetPageChildren lists the child pages directly under a page, starting at the
// zero-based offset start.
func (c *Client) GetPageChildren(pageID string, limit, start int) (string, error) {
	path := fmt.Sprintf("/rest/api/content/%s/child/page?limit=%d&expand=space,version%s",
		url.PathEscape(pageID), limit, startParam(start))
	return c.get(path)
}

// GetComments returns the comments on a page, starting at the zero-based offset
// start. depth=all includes nested replies (children of comments), not just
// top-level comments; history carries each comment's author and created date,
// and ancestors gives its reply chain.
func (c *Client) GetComments(pageID string, limit, start int) (string, error) {
	path := fmt.Sprintf("/rest/api/content/%s/child/comment?limit=%d&depth=all&expand=body.storage,version,history,ancestors%s",
		url.PathEscape(pageID), limit, startParam(start))
	return c.get(path)
}

// GetAttachmentsFrom is GetAttachments with a paging offset, so a caller can
// walk past the first page of a heavily attached page by advancing start.
func (c *Client) GetAttachmentsFrom(pageID string, limit, start int) (string, error) {
	path := fmt.Sprintf("/rest/api/content/%s/child/attachment?limit=%d&expand=version,metadata%s",
		url.PathEscape(pageID), limit, startParam(start))
	return c.get(path)
}

// CreatePage creates a new page or blog post. parentID is optional (empty for a
// top-level page) and is rejected for a blog post, which has no ancestors.
// contentType is "page" (the default when empty) or "blogpost"; representation
// is the body format, e.g. "storage" or "wiki".
func (c *Client) CreatePage(spaceKey, title, content, parentID, representation, contentType string) (string, error) {
	resolvedType, err := resolveContentType(contentType, parentID)
	if err != nil {
		return "", err
	}

	payload := map[string]any{
		"type":  resolvedType,
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
		Type  string `json:"type"`
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
		"type":    contentTypeOf(current.Type),
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
		Type  string `json:"type"`
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
		"type":    contentTypeOf(current.Type),
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

// DeletePage deletes a page by ID (moves it to the trash, where a space admin
// can still restore it).
func (c *Client) DeletePage(pageID string) error {
	_, err := c.do(http.MethodDelete, "/rest/api/content/"+url.PathEscape(pageID), "")
	return err
}

// PurgePage deletes a page and then removes it from the trash, destroying it
// for good. Confluence only purges content that is already trashed, so this is
// necessarily two requests: the ordinary delete, then a delete of the trashed
// copy.
func (c *Client) PurgePage(pageID string) error {
	if err := c.DeletePage(pageID); err != nil {
		return err
	}
	// The purge needs space-admin rights the first delete does not, so this half
	// commonly fails on its own. Saying so keeps a caller from concluding the
	// page is untouched and retrying against a page that is already trashed.
	if _, err := c.do(http.MethodDelete, "/rest/api/content/"+url.PathEscape(pageID)+"?status=trashed", ""); err != nil {
		return fmt.Errorf("page %s was moved to the trash but could not be purged (purging requires "+
			"space-admin rights); it is recoverable from the trash: %w", pageID, err)
	}
	return nil
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
		Type  string `json:"type"`
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
		"type":      contentTypeOf(current.Type),
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
