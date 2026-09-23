package confluence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"dinhphu28/atlassian-mcp/internal/markdown"
)

// MermaidRenderer renders Mermaid source to image bytes (PNG). A nil renderer,
// or one that returns an error, causes the diagram to fall back to a code macro.
type MermaidRenderer func(source string) ([]byte, error)

// CreatePageMarkdown creates a page (or, with contentType "blogpost", a blog
// post) from Markdown. Mermaid diagrams are rendered and uploaded as
// attachments; if that is not possible they degrade to code macros. Because
// attachments require an existing page, content with diagrams is created first
// (with code-macro fallbacks) and then patched to reference the uploaded images.
// The returned warnings list any diagram that did not become an image (and why);
// the write still succeeds.
func (c *Client) CreatePageMarkdown(spaceKey, title, md, parentID, contentType string, render MermaidRenderer) (string, []string, error) {
	storage, diagrams, err := markdown.ToStorage(md)
	if err != nil {
		return "", nil, err
	}

	initial := applyDiagrams(storage, diagrams, nil)
	raw, err := c.CreatePage(spaceKey, title, initial, parentID, "storage", contentType)
	if err != nil {
		return "", nil, err
	}
	if len(diagrams) == 0 {
		return raw, nil, nil
	}
	if render == nil {
		return raw, rendererUnavailableWarnings(diagrams), nil
	}

	pageID := parseID(raw)
	if pageID == "" {
		return raw, nil, nil
	}
	// A freshly created page has no prior attachments to reuse.
	filenames, warnings := renderAndUpload(c, pageID, diagrams, render, nil)
	if len(filenames) == 0 {
		return raw, warnings, nil
	}
	final := applyDiagrams(storage, diagrams, filenames)
	updated, err := c.UpdatePage(pageID, final, title, "storage")
	if err != nil {
		return "", warnings, err
	}
	return updated, warnings, nil
}

// UpdatePageMarkdown updates a page from Markdown, rendering and uploading any
// Mermaid diagrams to the existing page before the version bump. When a diagram
// fails to render or upload but an attachment for it already exists on the page,
// the existing image is reused rather than downgrading the page to a code macro.
// The returned warnings list any diagram that did not become an image.
func (c *Client) UpdatePageMarkdown(pageID, md, title string, render MermaidRenderer) (string, []string, error) {
	return c.UpdatePageMarkdownAt(pageID, md, title, render, 0)
}

// UpdatePageMarkdownAt is UpdatePageMarkdown with the optimistic-locking
// behaviour of UpdatePageAt: expectedVersion greater than zero makes the write
// fail if the page changed since it was read.
func (c *Client) UpdatePageMarkdownAt(pageID, md, title string, render MermaidRenderer, expectedVersion int) (string, []string, error) {
	storage, diagrams, err := markdown.ToStorage(md)
	if err != nil {
		return "", nil, err
	}
	filenames := map[string]string{}
	var warnings []string
	if len(diagrams) > 0 {
		if render == nil {
			warnings = rendererUnavailableWarnings(diagrams)
		} else {
			existing := c.attachmentFilenames(pageID)
			filenames, warnings = renderAndUpload(c, pageID, diagrams, render, existing)
		}
	}
	final := applyDiagrams(storage, diagrams, filenames)
	updated, err := c.UpdatePageAt(pageID, final, title, "storage", expectedVersion)
	if err != nil {
		return "", warnings, err
	}
	return updated, warnings, nil
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

// diagramFilename derives a stable attachment name from the diagram source, so
// the same diagram always maps to the same attachment (idempotent re-upload) and
// reordering/inserting diagrams never silently clobbers an unrelated image.
func diagramFilename(source string) string {
	sum := sha256.Sum256([]byte(source))
	return "mermaid-" + hex.EncodeToString(sum[:])[:8] + ".png"
}

// renderAndUpload renders each diagram and uploads successful renders as
// attachments, returning placeholder -> filename for the images now available.
// existing is the set of attachment filenames already on the page; a diagram
// whose image is already there is reused as-is. The second return value lists a
// warning for each diagram that had to degrade to a code macro.
func renderAndUpload(c *Client, pageID string, diagrams []markdown.Diagram, render MermaidRenderer, existing map[string]bool) (map[string]string, []string) {
	filenames := map[string]string{}
	var warnings []string
	for i, d := range diagrams {
		name := diagramFilename(d.Source)

		// The name is a hash of the diagram source, so an attachment already
		// carrying it is this exact image: reuse it instead of re-rendering and
		// storing another identical version.
		if existing[name] {
			filenames[d.Placeholder] = name
			continue
		}

		png, err := render(d.Source)
		if err != nil {
			warnings = append(warnings, codeBlockWarning(i, "render", err))
			continue
		}
		if _, err := c.UploadAttachment(pageID, name, png); err != nil {
			warnings = append(warnings, codeBlockWarning(i, "upload", err))
			continue
		}
		filenames[d.Placeholder] = name
	}
	return filenames, warnings
}

// codeBlockWarning reports that a diagram degraded to a code macro because the
// named stage failed and the page carries no image for it to fall back on.
func codeBlockWarning(i int, stage string, cause error) string {
	return fmt.Sprintf("diagram %d: %s failed, rendered as code block: %v", i, stage, cause)
}

// rendererUnavailableWarnings reports that mmdc was not available, so every
// diagram fell back to a code macro.
func rendererUnavailableWarnings(diagrams []markdown.Diagram) []string {
	warnings := make([]string, 0, len(diagrams))
	for i := range diagrams {
		warnings = append(warnings, fmt.Sprintf("diagram %d: mmdc not available, rendered as code block (set MERMAID_CLI_PATH or install mermaid-cli)", i))
	}
	return warnings
}

// attachmentFilenames returns the set of attachment titles currently on a page.
// On any error it returns nil, so callers simply proceed with no reuse.
func (c *Client) attachmentFilenames(pageID string) map[string]bool {
	raw, err := c.GetAttachments(pageID, 200)
	if err != nil {
		return nil
	}
	var resp struct {
		Results []struct {
			Title string `json:"title"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil
	}
	set := make(map[string]bool, len(resp.Results))
	for _, r := range resp.Results {
		set[r.Title] = true
	}
	return set
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

// UpdateCommentMarkdown edits a comment using Markdown.
func (c *Client) UpdateCommentMarkdown(commentID, md string) (string, error) {
	return c.UpdateCommentMarkdownAt(commentID, md, 0)
}

// UpdateCommentMarkdownAt is UpdateCommentMarkdown with the optimistic-locking
// behaviour of UpdateCommentAt.
func (c *Client) UpdateCommentMarkdownAt(commentID, md string, expectedVersion int) (string, error) {
	storage, err := markdownComment(md)
	if err != nil {
		return "", err
	}
	return c.UpdateCommentAt(commentID, storage, "storage", expectedVersion)
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

// PageMarkdown is a page rendered as Markdown plus the metadata needed to
// update it without a second fetch. Dropped names the Confluence constructs the
// conversion could not represent, so a caller can decide to re-read the page as
// storage instead of editing a body that has already lost them.
type PageMarkdown struct {
	ID       string
	Title    string
	Space    string
	Version  int
	Markdown string
	Dropped  []string
}

// GetPageMarkdown fetches a page and returns its body converted to Markdown.
func (c *Client) GetPageMarkdown(pageID string) (*PageMarkdown, error) {
	return c.GetPageMarkdownAt(pageID, 0)
}

// GetPageMarkdownAt is GetPageMarkdown for one revision: a version greater than
// zero converts that historical body instead of the current one. Version is the
// revision actually read, so a caller restoring an old page can tell which one
// it has in hand.
func (c *Client) GetPageMarkdownAt(pageID string, version int) (*PageMarkdown, error) {
	raw, err := c.GetPageAt(pageID, version)
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
		Dropped:  markdown.UntranslatedMacros(p.Body.Storage.Value),
	}, nil
}

// GetCommentsMarkdown fetches a page's comments and returns them as a Markdown
// list, each comment's body converted from storage format. start is the paging
// offset, as on GetComments.
func (c *Client) GetCommentsMarkdown(pageID string, limit, start int) (string, error) {
	raw, err := c.GetComments(pageID, limit, start)
	if err != nil {
		return "", err
	}

	var resp struct {
		Results []struct {
			History struct {
				CreatedBy struct {
					DisplayName string `json:"displayName"`
				} `json:"createdBy"`
				CreatedDate string `json:"createdDate"`
			} `json:"history"`
			Ancestors []struct {
				ID string `json:"id"`
			} `json:"ancestors"`
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

	if len(resp.Results) == 0 {
		return "No comments.", nil
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

		// Header carries author + created date (dropped by the old body-only
		// conversion) and marks nested replies by their depth in the thread.
		author := cm.History.CreatedBy.DisplayName
		if author == "" {
			author = "unknown"
		}
		b.WriteString("**")
		b.WriteString(author)
		b.WriteString("**")
		if cm.History.CreatedDate != "" {
			b.WriteString(" · ")
			b.WriteString(cm.History.CreatedDate)
		}
		if depth := len(cm.Ancestors); depth > 0 {
			b.WriteString(fmt.Sprintf(" · ↳ reply (depth %d)", depth))
		}
		b.WriteString("\n\n")
		b.WriteString(md)
	}
	return b.String(), nil
}
