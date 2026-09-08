package mcpserver

import (
	"github.com/mark3labs/mcp-go/server"

	"dinhphu28/atlassian-mcp/internal/jira"
)

// RegisterJira registers the Jira tools on s. Read tools are always registered;
// write tools are skipped when readOnly is true.
func RegisterJira(s *server.MCPServer, client *jira.Client, readOnly bool) {
	registerJiraReadTools(s, client)
	registerXrayReadTools(s, client)
	if !readOnly {
		registerJiraWriteTools(s, client)
		registerXrayWriteTools(s, client)
	}
}
