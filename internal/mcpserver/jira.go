package mcpserver

import (
	"github.com/mark3labs/mcp-go/server"

	"dinhphu28/atlassian-mcp/internal/jira"
)

// RegisterJira registers the Jira tools on s. Read tools are always registered;
// write tools are skipped when readOnly is true.
func RegisterJira(s *server.MCPServer, client *jira.Client, readOnly bool) {
	registerJiraReadTools(s, client)
	registerJiraFieldReadTools(s, client)
	registerJiraAdminReadTools(s, client)
	registerJiraMetaReadTools(s, client)
	registerJiraSocialReadTools(s, client)
	registerJiraAgileReadTools(s, client)
	registerXrayReadTools(s, client)
	registerXrayTestReadTools(s, client)
	if !readOnly {
		registerJiraWriteTools(s, client)
		registerJiraFieldWriteTools(s, client)
		registerJiraAdminWriteTools(s, client)
		registerJiraMetaWriteTools(s, client)
		registerJiraSocialWriteTools(s, client)
		registerJiraAgileWriteTools(s, client)
		registerXrayWriteTools(s, client)
		registerXrayTestWriteTools(s, client)
	}
}
