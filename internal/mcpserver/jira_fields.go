package mcpserver

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"dinhphu28/atlassian-mcp/internal/jira"
)

func registerJiraFieldReadTools(s *server.MCPServer, client *jira.Client) {
	getEditFieldsTool := mcp.NewTool(
		"jira_get_edit_fields",
		mcp.WithDescription("List the fields an issue's edit screen accepts, with their ids, types and allowed "+
			"values. Use this to find the id of a custom field (e.g. Story Points, Epic Link) before setting it "+
			"with jira_set_fields, and to check which values a select or version field allows."),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
		mcp.WithBoolean("compact", mcp.Description("Summarise each field as id, name, required, type, operations "+
			"and allowed values, capped at 20 per field (default true). Set false for the full editmeta payload, "+
			"which is very large.")),
	)

	s.AddTool(getEditFieldsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.GetEditFields(key, request.GetBool("compact", true)))
	})
}

func registerJiraFieldWriteTools(s *server.MCPServer, client *jira.Client) {
	setFieldsTool := mcp.NewTool(
		"jira_set_fields",
		mcp.WithDescription("Set arbitrary fields on a Jira issue in one request: labels, due date, components, "+
			"versions, reporter, time estimates, and any custom field through fields_json/update_json. Omitted "+
			"parameters are left unchanged. Call jira_get_edit_fields first to find custom field ids and the "+
			"values a field allows."),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
		mcp.WithString("labels", mcp.Description("Comma-separated labels REPLACING the issue's whole label set")),
		mcp.WithString("labels_add", mcp.Description("Comma-separated labels to add, keeping the existing ones")),
		mcp.WithString("labels_remove", mcp.Description("Comma-separated labels to remove")),
		mcp.WithString("due_date", mcp.Description("Due date in yyyy-MM-dd format")),
		mcp.WithString("components", mcp.Description("Comma-separated component names, replacing the current ones")),
		mcp.WithString("fix_versions", mcp.Description("Comma-separated fix version names, replacing the current ones")),
		mcp.WithString("affects_versions", mcp.Description("Comma-separated affects version names, replacing the current ones")),
		mcp.WithString("reporter", mcp.Description("Jira username of the reporter (Server/DC username, not an email)")),
		mcp.WithString("original_estimate", mcp.Description("Original estimate as a Jira duration, e.g. '3d 4h'. "+
			"Jira stores both estimates in one field and rewrites the one a write omits, so passing only this "+
			"one makes the tool read the current remaining estimate and resend it unchanged.")),
		mcp.WithString("remaining_estimate", mcp.Description("Remaining estimate as a Jira duration, e.g. '2d'. "+
			"As with original_estimate, the other half is read and resent so it is not overwritten.")),
		mcp.WithString("fields_json", mcp.Description("JSON object merged into the request's \"fields\" section, "+
			"for anything without a named parameter, e.g. {\"customfield_10001\": 5}. Values replace the field; "+
			"this is also how to clear one, with null or [].")),
		mcp.WithString("update_json", mcp.Description("JSON object merged into the request's \"update\" section, "+
			"for add/remove semantics on multi-value fields, e.g. "+
			"{\"customfield_10002\": [{\"add\": {\"value\": \"x\"}}]}")),
	)

	s.AddTool(setFieldsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		edits := jira.FieldEdits{
			Labels:            request.GetString("labels", ""),
			LabelsAdd:         request.GetString("labels_add", ""),
			LabelsRemove:      request.GetString("labels_remove", ""),
			DueDate:           request.GetString("due_date", ""),
			Components:        request.GetString("components", ""),
			FixVersions:       request.GetString("fix_versions", ""),
			AffectsVersions:   request.GetString("affects_versions", ""),
			Reporter:          request.GetString("reporter", ""),
			OriginalEstimate:  request.GetString("original_estimate", ""),
			RemainingEstimate: request.GetString("remaining_estimate", ""),
			FieldsJSON:        request.GetString("fields_json", ""),
			UpdateJSON:        request.GetString("update_json", ""),
		}

		confirmation, err := client.SetFields(key, edits)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(confirmation), nil
	})
}
