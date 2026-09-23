package mcpserver

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"dinhphu28/atlassian-mcp/internal/jira"
)

func registerJiraAdminReadTools(s *server.MCPServer, client *jira.Client) {
	getIssueHistoryTool := mcp.NewTool(
		"jira_get_issue_history",
		mcp.WithDescription("Get a Jira issue's change history: who changed which field, from what to what, and when. "+
			"Returns a compact projection (one entry per change, newest last); set raw=true for Jira's full changelog payload."),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
		mcp.WithNumber("limit", mcp.Description("Keep only the most recent N entries (default 50; 0 or less keeps all)")),
		mcp.WithBoolean("raw", mcp.Description("Return Jira's untouched changelog payload instead of the compact projection (default false)")),
	)

	s.AddTool(getIssueHistoryTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.GetIssueHistory(
			key,
			request.GetInt("limit", 50),
			request.GetBool("raw", false),
		))
	})
}

func registerJiraAdminWriteTools(s *server.MCPServer, client *jira.Client) {
	deleteIssueTool := mcp.NewTool(
		"jira_delete_issue",
		mcp.WithDescription("Permanently delete a Jira issue. This is irreversible. "+
			"Jira rejects the delete when the issue has sub-tasks unless delete_subtasks is true, "+
			"which deletes those sub-tasks along with it."),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
		mcp.WithBoolean("delete_subtasks", mcp.Description("Also delete the issue's sub-tasks (default false; without it Jira refuses an issue that has any)")),
	)

	s.AddTool(deleteIssueTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		deleteSubtasks := request.GetBool("delete_subtasks", false)

		if err := client.DeleteIssue(key, deleteSubtasks); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if deleteSubtasks {
			return mcp.NewToolResultText(fmt.Sprintf("Deleted issue %s and its sub-tasks", key)), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Deleted issue %s", key)), nil
	})

	deleteAttachmentTool := mcp.NewTool(
		"jira_delete_attachment",
		mcp.WithDescription("Permanently delete an attachment from a Jira issue. This is irreversible. "+
			"Get the attachment id from jira_get_issue (the 'attachment' field)."),
		mcp.WithString("attachment_id", mcp.Required(), mcp.Description("Attachment id from an issue's attachment field (jira_get_issue)")),
	)

	s.AddTool(deleteAttachmentTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		attachmentID, err := request.RequireString("attachment_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if err := client.DeleteAttachment(attachmentID); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Deleted attachment %s", attachmentID)), nil
	})

	bulkCreateIssuesTool := mcp.NewTool(
		"jira_bulk_create_issues",
		mcp.WithDescription("Create several Jira issues in one request. issues_json is a JSON array of field objects, "+
			"each one what jira_create_issue would build. Partial success is possible: the response lists the created "+
			"issues and a per-issue error for the ones Jira rejected, so check both."),
		mcp.WithString("issues_json", mcp.Required(), mcp.Description(
			`JSON array of issue field objects, e.g. [{"project":{"key":"DEV"},"issuetype":{"name":"Task"},"summary":"First"},`+
				`{"project":{"key":"DEV"},"issuetype":{"name":"Bug"},"summary":"Second","description":"Jira wiki markup"}]. `+
				`An element may also be pre-wrapped as {"fields": {…}}.`)),
	)

	s.AddTool(bulkCreateIssuesTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		issuesJSON, err := request.RequireString("issues_json")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.BulkCreateIssues(issuesJSON))
	})
}
