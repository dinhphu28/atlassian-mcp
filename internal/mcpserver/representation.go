package mcpserver

import (
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// Body formats a Confluence tool can be asked for. Markdown is converted by
// this server and is lossy; storage is Confluence's own XHTML and round-trips
// exactly; wiki is the legacy wiki markup, accepted on writes only.
const (
	reprMarkdown = "markdown"
	reprStorage  = "storage"
	reprWiki     = "wiki"
)

// representation reads the representation argument and validates it against the
// formats the calling tool supports. Values are lower-cased and trimmed, so
// "Storage" works; anything unsupported is an error rather than a silent
// fall-back to Markdown, which would quietly convert (and so damage) a body the
// caller asked to be handled verbatim.
func bodyRepresentation(request mcp.CallToolRequest, allowed ...string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(request.GetString("representation", reprMarkdown)))
	if value == "" {
		value = reprMarkdown
	}

	for _, a := range allowed {
		if value == a {
			return value, nil
		}
	}

	return "", fmt.Errorf("unsupported representation %q; use one of: %s", value, strings.Join(allowed, ", "))
}
