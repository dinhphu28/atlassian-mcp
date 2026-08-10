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
		mcp.WithDescription("Create a new Jira issue. For a sub-task issue type, parent_key is required."),
		mcp.WithString("project_key", mcp.Required(), mcp.Description("Project key, e.g. DEV")),
		mcp.WithString("issue_type", mcp.Required(), mcp.Description("Issue type name, e.g. Task, Bug, Story, Sub-task")),
		mcp.WithString("summary", mcp.Required(), mcp.Description("Issue summary/title")),
		mcp.WithString("description", mcp.Description("Issue description (Jira wiki markup)")),
		mcp.WithString("parent_key", mcp.Description("Parent issue key (e.g. DEV-123); required when issue_type is a sub-task")),
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
			request.GetString("parent_key", ""),
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

	updateCommentTool := mcp.NewTool(
		"jira_update_comment",
		mcp.WithDescription("Edit an existing comment on a Jira issue (get the comment id from jira_get_comments)"),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
		mcp.WithString("comment_id", mcp.Required(), mcp.Description("Comment id from jira_get_comments")),
		mcp.WithString("body", mcp.Required(), mcp.Description("New comment body (Jira wiki markup)")),
	)

	s.AddTool(updateCommentTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		commentID, err := request.RequireString("comment_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		body, err := request.RequireString("body")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.UpdateComment(key, commentID, body))
	})

	deleteCommentTool := mcp.NewTool(
		"jira_delete_comment",
		mcp.WithDescription("Delete a comment from a Jira issue (get the comment id from jira_get_comments)"),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
		mcp.WithString("comment_id", mcp.Required(), mcp.Description("Comment id from jira_get_comments")),
	)

	s.AddTool(deleteCommentTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		commentID, err := request.RequireString("comment_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if err := client.DeleteComment(key, commentID); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Deleted comment %s on %s", commentID, key)), nil
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

	setTargetDatesTool := mcp.NewTool(
		"jira_set_target_dates",
		mcp.WithDescription("Set an issue's Advanced Roadmaps 'Target start' and/or 'Target end' dates. "+
			"The customfield ids are auto-discovered, so this works on any instance with Advanced Roadmaps enabled."),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
		mcp.WithString("target_start", mcp.Description("Target start date as yyyy-MM-dd (unchanged if omitted)")),
		mcp.WithString("target_end", mcp.Description("Target end date as yyyy-MM-dd (unchanged if omitted)")),
	)

	s.AddTool(setTargetDatesTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		msg, err := client.SetTargetDates(
			key,
			request.GetString("target_start", ""),
			request.GetString("target_end", ""),
		)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(msg), nil
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
