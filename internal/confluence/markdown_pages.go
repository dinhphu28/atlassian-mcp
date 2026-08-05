package confluence

import (
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
