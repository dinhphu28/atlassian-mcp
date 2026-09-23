package mcpserver

import (
	"github.com/mark3labs/mcp-go/server"

	"dinhphu28/atlassian-mcp/internal/confluence"
)

// RegisterConfluence registers the Confluence tools on s. Read tools are always
// registered; write tools are skipped when readOnly is true.
func RegisterConfluence(s *server.MCPServer, client *confluence.Client, readOnly bool) {
	registerConfluenceReadTools(s, client)
	registerConfluenceSpaceReadTools(s, client)
	registerConfluenceContentReadTools(s, client)
	if !readOnly {
		registerConfluenceWriteTools(s, client)
		registerConfluenceSpaceWriteTools(s, client)
		registerConfluenceContentWriteTools(s, client)
	}
}
