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
		mcp.WithString("assignee", mcp.Description("Optional Jira username to assign the new issue to")),
		mcp.WithString("priority", mcp.Description("Optional priority name (e.g. High) or id; see jira_get_priorities")),
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
			request.GetString("assignee", ""),
			request.GetString("priority", ""),
		))
	})

	addCommentTool := mcp.NewTool(
		"jira_add_comment",
		mcp.WithDescription("Add a comment to a Jira issue. By default everyone who can see the issue can read it; "+
			"pass visibility_type and visibility_value together to restrict it to a project role or a group."),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
		mcp.WithString("body", mcp.Required(), mcp.Description("Comment body (Jira wiki markup)")),
		mcp.WithString("visibility_type", mcp.Description("Restrict the comment to a 'role' or a 'group'; requires visibility_value")),
		mcp.WithString("visibility_value", mcp.Description("Project role name (e.g. Administrators) or group name (e.g. jira-developers); requires visibility_type")),
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

		return jsonResult(client.AddComment(
			key, body,
			request.GetString("visibility_type", ""),
			request.GetString("visibility_value", ""),
		))
	})

	updateCommentTool := mcp.NewTool(
		"jira_update_comment",
		mcp.WithDescription("Edit an existing comment on a Jira issue (get the comment id from jira_get_comments). "+
			"Omitting visibility_type and visibility_value keeps the comment's current restriction: the edit "+
			"would otherwise republish a restricted comment to everyone who can see the issue."),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
		mcp.WithString("comment_id", mcp.Required(), mcp.Description("Comment id from jira_get_comments")),
		mcp.WithString("body", mcp.Required(), mcp.Description("New comment body (Jira wiki markup)")),
		mcp.WithString("visibility_type", mcp.Description("Restrict the comment to a 'role' or a 'group' (requires visibility_value), "+
			"or 'none' to remove an existing restriction. Omit to keep the comment's current restriction.")),
		mcp.WithString("visibility_value", mcp.Description("Project role name (e.g. Administrators) or group name (e.g. jira-developers); requires visibility_type")),
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

		return jsonResult(client.UpdateComment(
			key, commentID, body,
			request.GetString("visibility_type", ""),
			request.GetString("visibility_value", ""),
		))
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
		mcp.WithDescription("Move a Jira issue through a status transition (get the id from jira_get_transitions). "+
			"A transition whose screen requires input fails unless those fields are supplied: the usual one is "+
			"'resolution' on a Done transition. Call jira_get_transitions with expand_fields=true to see exactly "+
			"which fields a transition screen requires and which values it allows."),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
		mcp.WithString("transition_id", mcp.Required(), mcp.Description("Transition id from jira_get_transitions")),
		mcp.WithString("resolution", mcp.Description("Resolution name (e.g. Done, case-insensitive) or id, for a transition screen that requires one; "+
			"the accepted values are the transition screen's, listed by jira_get_transitions with expand_fields=true")),
		mcp.WithString("comment", mcp.Description("Comment to post as part of the transition (Jira wiki markup)")),
		mcp.WithString("fields", mcp.Description("Raw JSON object of any other fields the transition screen requires, "+
			"e.g. {\"customfield_10001\": {\"value\": \"Yes\"}}")),
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

		if err := client.TransitionIssue(
			key, transitionID,
			request.GetString("resolution", ""),
			request.GetString("comment", ""),
			request.GetString("fields", ""),
		); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Transitioned issue %s (transition %s)", key, transitionID)), nil
	})

	assignIssueTool := mcp.NewTool(
		"jira_assign_issue",
		mcp.WithDescription("Assign a Jira issue to a user. Leave assignee empty (or 'null') to unassign; use '-1' for the project's default assignee."),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
		mcp.WithString("assignee", mcp.Description("Jira username to assign to; empty or 'null' unassigns, '-1' sets the default assignee")),
	)

	s.AddTool(assignIssueTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		assignee := request.GetString("assignee", "")

		if err := client.AssignIssue(key, assignee); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if assignee == "" || assignee == "null" {
			return mcp.NewToolResultText(fmt.Sprintf("Unassigned issue %s", key)), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Assigned issue %s to %s", key, assignee)), nil
	})

	addWorklogTool := mcp.NewTool(
		"jira_add_worklog",
		mcp.WithDescription("Log work (time spent) on a Jira issue"),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
		mcp.WithString("time_spent", mcp.Required(), mcp.Description("Time spent as a Jira duration, e.g. '3h 30m' or '1d'")),
		mcp.WithString("started", mcp.Description("When the work started; 'yyyy-MM-dd', 'yyyy-MM-dd HH:mm' or a full timestamp (defaults to now, local time zone)")),
		mcp.WithString("comment", mcp.Description("Worklog comment (Jira wiki markup)")),
		mcp.WithString("adjust_estimate", mcp.Description("How to adjust the remaining estimate: auto (default), leave, new or manual")),
		mcp.WithString("estimate_value", mcp.Description("Jira duration for adjust_estimate: the new estimate for 'new', the amount to reduce by for 'manual'")),
	)

	s.AddTool(addWorklogTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		timeSpent, err := request.RequireString("time_spent")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.AddWorklog(
			key, timeSpent,
			request.GetString("started", ""),
			request.GetString("comment", ""),
			request.GetString("adjust_estimate", ""),
			request.GetString("estimate_value", ""),
		))
	})

	updateWorklogTool := mcp.NewTool(
		"jira_update_worklog",
		mcp.WithDescription("Edit a worklog on a Jira issue (get the worklog id from jira_get_worklogs). "+
			"Omitted fields keep their current values."),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
		mcp.WithString("worklog_id", mcp.Required(), mcp.Description("Worklog id from jira_get_worklogs")),
		mcp.WithString("time_spent", mcp.Description("New time spent, e.g. '3h 30m' (unchanged if omitted)")),
		mcp.WithString("started", mcp.Description("New start time; 'yyyy-MM-dd', 'yyyy-MM-dd HH:mm' or a full timestamp (unchanged if omitted)")),
		mcp.WithString("comment", mcp.Description("New worklog comment in Jira wiki markup (unchanged if omitted)")),
		mcp.WithString("adjust_estimate", mcp.Description("How to adjust the remaining estimate: auto (default), leave or new")),
		mcp.WithString("estimate_value", mcp.Description("Jira duration for the new estimate when adjust_estimate is 'new'")),
	)

	s.AddTool(updateWorklogTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		worklogID, err := request.RequireString("worklog_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.UpdateWorklog(
			key, worklogID,
			request.GetString("time_spent", ""),
			request.GetString("started", ""),
			request.GetString("comment", ""),
			request.GetString("adjust_estimate", ""),
			request.GetString("estimate_value", ""),
		))
	})

	deleteWorklogTool := mcp.NewTool(
		"jira_delete_worklog",
		mcp.WithDescription("Delete a worklog from a Jira issue (get the worklog id from jira_get_worklogs)"),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
		mcp.WithString("worklog_id", mcp.Required(), mcp.Description("Worklog id from jira_get_worklogs")),
		mcp.WithString("adjust_estimate", mcp.Description("How to adjust the remaining estimate: auto (default), leave, new or manual")),
		mcp.WithString("estimate_value", mcp.Description("Jira duration for adjust_estimate: the new estimate for 'new', the amount to increase by for 'manual'")),
	)

	s.AddTool(deleteWorklogTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		worklogID, err := request.RequireString("worklog_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if err := client.DeleteWorklog(
			key, worklogID,
			request.GetString("adjust_estimate", ""),
			request.GetString("estimate_value", ""),
		); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Deleted worklog %s on %s", worklogID, key)), nil
	})

	setPriorityTool := mcp.NewTool(
		"jira_set_priority",
		mcp.WithDescription("Set a Jira issue's priority (list the instance's priorities with jira_get_priorities)"),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
		mcp.WithString("priority", mcp.Required(), mcp.Description("Priority name, e.g. High (case-insensitive), or a numeric priority id")),
	)

	s.AddTool(setPriorityTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		priority, err := request.RequireString("priority")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		msg, err := client.SetPriority(key, priority)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(msg), nil
	})

	linkIssuesTool := mcp.NewTool(
		"jira_link_issues",
		mcp.WithDescription("Link two Jira issues. The arguments read as a sentence: "+
			"issue_key link_type target_issue_key, e.g. DEV-1 'blocks' DEV-2. "+
			"List the available types with jira_get_issue_link_types."),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key the link starts from (e.g. DEV-123)")),
		mcp.WithString("link_type", mcp.Required(), mcp.Description(
			"Link type name (e.g. Blocks), its id, or a direction description (e.g. 'blocks', 'is blocked by', 'relates to')")),
		mcp.WithString("target_issue_key", mcp.Required(), mcp.Description("Issue key the link points at (e.g. DEV-456)")),
		mcp.WithString("comment", mcp.Description("Optional comment (Jira wiki markup) to post with the link")),
	)

	s.AddTool(linkIssuesTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		linkType, err := request.RequireString("link_type")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		targetKey, err := request.RequireString("target_issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		msg, err := client.LinkIssues(key, linkType, targetKey, request.GetString("comment", ""))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(msg), nil
	})

	deleteIssueLinkTool := mcp.NewTool(
		"jira_delete_issue_link",
		mcp.WithDescription("Delete an issue link by id (the ids are in an issue's 'issuelinks' field from jira_get_issue)"),
		mcp.WithString("link_id", mcp.Required(), mcp.Description("Issue link id from an issue's issuelinks field")),
	)

	s.AddTool(deleteIssueLinkTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		linkID, err := request.RequireString("link_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if err := client.DeleteIssueLink(linkID); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Deleted issue link %s", linkID)), nil
	})
}
