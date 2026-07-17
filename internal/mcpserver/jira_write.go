package mcpserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"dinhphu28/atlassian-mcp/internal/jira"
)

func registerJiraWriteTools(s *server.MCPServer, client *jira.Client) {
	createIssueTool := mcp.NewTool(
		"jira_create_issue",
		mcp.WithDescription("Create a new Jira issue"),
		mcp.WithString("project_key", mcp.Required(), mcp.Description("Project key, e.g. DEV")),
		mcp.WithString("issue_type", mcp.Required(), mcp.Description("Issue type name, e.g. Task, Bug, Story")),
		mcp.WithString("summary", mcp.Required(), mcp.Description("Issue summary/title")),
		mcp.WithString("description", mcp.Description("Issue description (Jira wiki markup)")),
	)

	s.AddTool(createIssueTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectKey, err := request.RequireString("project_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		issueType, err := request.RequireString("issue_type")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		summary, err := request.RequireString("summary")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.CreateIssue(
			projectKey, issueType, summary,
			request.GetString("description", ""),
		))
	})

	addCommentTool := mcp.NewTool(
		"jira_add_comment",
		mcp.WithDescription("Add a comment to a Jira issue"),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
		mcp.WithString("body", mcp.Required(), mcp.Description("Comment body (Jira wiki markup)")),
	)

	s.AddTool(addCommentTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		body, err := request.RequireString("body")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.AddComment(key, body))
	})

	updateIssueTool := mcp.NewTool(
		"jira_update_issue",
		mcp.WithDescription("Update a Jira issue's summary and/or description"),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
		mcp.WithString("summary", mcp.Description("New summary (unchanged if omitted)")),
		mcp.WithString("description", mcp.Description("New description in Jira wiki markup (unchanged if omitted)")),
	)

	s.AddTool(updateIssueTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		summary := request.GetString("summary", "")
		description := request.GetString("description", "")
		if summary == "" && description == "" {
			return mcp.NewToolResultError("provide 'summary' and/or 'description' to update"), nil
		}

		if err := client.UpdateIssue(key, summary, description); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Updated issue %s", key)), nil
	})

	uploadAttachmentTool := mcp.NewTool(
		"jira_upload_attachment",
		mcp.WithDescription("Upload a local file as an attachment on a Jira issue"),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
		mcp.WithString("file_path", mcp.Required(), mcp.Description("Absolute path to the local file to upload")),
	)

	s.AddTool(uploadAttachmentTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		filePath, err := request.RequireString("file_path")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		data, err := os.ReadFile(filePath)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.UploadAttachment(key, filepath.Base(filePath), data))
	})

	transitionIssueTool := mcp.NewTool(
		"jira_transition_issue",
		mcp.WithDescription("Move a Jira issue through a status transition (get the id from jira_get_transitions)"),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
		mcp.WithString("transition_id", mcp.Required(), mcp.Description("Transition id from jira_get_transitions")),
	)

	s.AddTool(transitionIssueTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		transitionID, err := request.RequireString("transition_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if err := client.TransitionIssue(key, transitionID); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Transitioned issue %s (transition %s)", key, transitionID)), nil
	})
}
