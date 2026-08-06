package mcpserver

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
)

// jsonResult turns a raw JSON response (or error) into a pretty-printed MCP
// tool result. Shared by all product tool sets.
func jsonResult(raw string, err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var pretty any
	_ = json.Unmarshal([]byte(raw), &pretty)
	b, _ := json.MarshalIndent(pretty, "", "  ")

	return mcp.NewToolResultText(string(b)), nil
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

	b, _ := json.MarshalIndent(out, "", "  ")
	return mcp.NewToolResultText(string(b)), nil
}
