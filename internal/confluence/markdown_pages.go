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

// CreatePageMarkdown creates a page from Markdown. Mermaid diagrams are rendered
// and uploaded as attachments; if that is not possible they degrade to code
// macros. Because attachments require an existing page, a page with diagrams is
// created first (with code-macro fallbacks) and then patched to reference the
// uploaded images. The returned warnings list any diagram that did not become an
// image (and why); the page write still succeeds.
func (c *Client) CreatePageMarkdown(spaceKey, title, md, parentID string, render MermaidRenderer) (string, []string, error) {
	storage, diagrams, err := markdown.ToStorage(md)
	if err != nil {
		return "", nil, err
	}

	initial := applyDiagrams(storage, diagrams, nil)
	raw, err := c.CreatePage(spaceKey, title, initial, parentID, "storage")
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
	updated, err := c.UpdatePage(pageID, final, title, "storage")
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
// existing is the set of attachment filenames already on the page: when a render
// or upload fails but a matching attachment already exists, it is reused instead
// of falling back to a code macro. The second return value lists a warning for
// each diagram that failed, whether or not an existing image rescued it.
func renderAndUpload(c *Client, pageID string, diagrams []markdown.Diagram, render MermaidRenderer, existing map[string]bool) (map[string]string, []string) {
	filenames := map[string]string{}
	var warnings []string
	for i, d := range diagrams {
		name := diagramFilename(d.Source)

		png, err := render(d.Source)
		if err != nil {
			warnings = append(warnings, reuseOrWarn(filenames, existing, d.Placeholder, name, i, "render", err))
			continue
		}
		if _, err := c.UploadAttachment(pageID, name, png); err != nil {
			warnings = append(warnings, reuseOrWarn(filenames, existing, d.Placeholder, name, i, "upload", err))
			continue
		}
		filenames[d.Placeholder] = name
	}
	return filenames, warnings
}

// reuseOrWarn records a fallback for a failed diagram: if the attachment already
// exists on the page it is reused (keeping the page an image) and the warning
// notes that; otherwise the diagram degrades to a code macro. It returns the
// warning message.
func reuseOrWarn(filenames map[string]string, existing map[string]bool, placeholder, name string, i int, stage string, cause error) string {
	if existing[name] {
		filenames[placeholder] = name
		return fmt.Sprintf("diagram %d: %s failed (%v); reused existing attachment %s", i, stage, cause, name)
	}
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
	storage, err := markdownComment(md)
	if err != nil {
		return "", err
	}
	return c.UpdateComment(commentID, storage, "storage")
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
