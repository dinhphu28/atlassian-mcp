package mcpserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// prettyEncode indents v as JSON with HTML escaping turned off, so
// storage-format XHTML in a response body reads as <ac:structured-macro …>
// rather than <ac:structured-macro …>. The request side disables the
// same escaping (see confluence.marshalPayload).
func prettyEncode(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return ""
	}
	return strings.TrimRight(buf.String(), "\n")
}

// jsonResult turns a raw JSON response (or error) into a pretty-printed MCP
// tool result. Shared by all product tool sets. A body that is not JSON is
// passed through unchanged rather than collapsing to "null", which is how an
// HTML error or login page served with status 200 used to surface.
func jsonResult(raw string, err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if strings.TrimSpace(raw) == "" {
		return mcp.NewToolResultText("(empty response)"), nil
	}

	var pretty any
	if err := json.Unmarshal([]byte(raw), &pretty); err != nil {
		return mcp.NewToolResultText(raw), nil
	}

	return mcp.NewToolResultText(prettyEncode(pretty)), nil
}

// markdownPageResult wraps a Markdown page write (create/update) that may have
// produced diagram warnings. On success it returns the page JSON, plus a
// diagram_warnings array whenever any Mermaid diagram did not become an image.
// Warnings are also echoed to stderr so they are visible in server logs.
func markdownPageResult(raw string, warnings []string, err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var page any
	_ = json.Unmarshal([]byte(raw), &page)

	out := map[string]any{"page": page}
	if len(warnings) > 0 {
		out["diagram_warnings"] = warnings
		for _, w := range warnings {
			fmt.Fprintln(os.Stderr, "mermaid warning:", w)
		}
	}

	return mcp.NewToolResultText(prettyEncode(out)), nil
}
