package mcpserver

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func requestWith(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}
}

func TestBodyRepresentationDefaultsAndNormalizes(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{"omitted defaults to markdown", map[string]any{}, reprMarkdown},
		{"empty defaults to markdown", map[string]any{"representation": ""}, reprMarkdown},
		{"storage", map[string]any{"representation": "storage"}, reprStorage},
		{"mixed case", map[string]any{"representation": "Storage"}, reprStorage},
		{"padded", map[string]any{"representation": " storage "}, reprStorage},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := bodyRepresentation(requestWith(tt.args), reprMarkdown, reprStorage)
			if err != nil {
				t.Fatalf("bodyRepresentation(%v) error: %v", tt.args, err)
			}
			if got != tt.want {
				t.Errorf("bodyRepresentation(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

// A representation a tool cannot honour must be an error: silently converting
// Markdown for a caller who asked for verbatim handling damages the body.
func TestBodyRepresentationRejectsUnsupported(t *testing.T) {
	for _, value := range []string{"wiki", "xhtml", "stroage"} {
		if _, err := bodyRepresentation(requestWith(map[string]any{"representation": value}), reprMarkdown, reprStorage); err == nil {
			t.Errorf("bodyRepresentation(%q) = nil error, want a rejection", value)
		}
	}

	// wiki is valid where the tool allows it.
	got, err := bodyRepresentation(requestWith(map[string]any{"representation": "wiki"}), reprMarkdown, reprStorage, reprWiki)
	if err != nil || got != reprWiki {
		t.Errorf("bodyRepresentation(wiki) = %q, %v; want wiki, nil", got, err)
	}
}

func TestBodyArgumentDistinguishesAbsentFromEmpty(t *testing.T) {
	if _, has, _ := bodyArgument(requestWith(map[string]any{"title": "T"})); has {
		t.Error("a call with no content and no file_path reported a body")
	}
	if _, has, _ := bodyArgument(requestWith(map[string]any{"content": ""})); has {
		t.Error("an empty content reported a body; an update would blank the page")
	}
	body, has, err := bodyArgument(requestWith(map[string]any{"content": "# hi"}))
	if err != nil || !has || body != "# hi" {
		t.Errorf("bodyArgument(content) = %q, %v, %v; want \"# hi\", true, nil", body, has, err)
	}
}
