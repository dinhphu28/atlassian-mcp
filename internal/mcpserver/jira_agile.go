package mcpserver

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"dinhphu28/atlassian-mcp/internal/jira"
)

func registerJiraAgileReadTools(s *server.MCPServer, client *jira.Client) {
	getBoardsTool := mcp.NewTool(
		"jira_get_boards",
		mcp.WithDescription("List the Jira Software (Agile) boards you can see. Board ids from here are what "+
			"jira_get_sprints, jira_get_backlog and jira_create_sprint need. Requires Jira Software."),
		mcp.WithString("project_key", mcp.Description("Only boards of this project, e.g. DEV (project key or numeric id)")),
		mcp.WithString("name", mcp.Description("Only boards whose name contains this text")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of boards (default 50, the API's own maximum)")),
		mcp.WithNumber("start_at", mcp.Description("0-based index of the first result to return (default 0); use it with limit to page past the API's 50-item cap")),
	)

	s.AddTool(getBoardsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return jsonResult(client.GetBoards(
			request.GetString("project_key", ""),
			request.GetString("name", ""),
			request.GetInt("limit", 50),
			request.GetInt("start_at", 0),
		))
	})

	getSprintsTool := mcp.NewTool(
		"jira_get_sprints",
		mcp.WithDescription("List a board's sprints (get the board id from jira_get_boards). The sprint ids returned "+
			"here are what jira_get_sprint_issues and jira_move_issues_to_sprint need."),
		mcp.WithString("board_id", mcp.Required(), mcp.Description("Numeric board id from jira_get_boards")),
		mcp.WithString("state", mcp.Description("Comma-separated sprint states to include: future, active, closed (default: all)")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of sprints (default 50, the API's own maximum)")),
		mcp.WithNumber("start_at", mcp.Description("0-based index of the first result to return (default 0); use it with limit to page past the API's 50-item cap")),
	)

	s.AddTool(getSprintsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		boardID, err := request.RequireString("board_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.GetSprints(
			boardID,
			request.GetString("state", ""),
			request.GetInt("limit", 50),
			request.GetInt("start_at", 0),
		))
	})

	getSprintIssuesTool := mcp.NewTool(
		"jira_get_sprint_issues",
		mcp.WithDescription("Get the issues in a sprint (get the sprint id from jira_get_sprints). "+
			"Pass 'fields' to keep the response small on a large sprint."),
		mcp.WithString("sprint_id", mcp.Required(), mcp.Description("Numeric sprint id from jira_get_sprints")),
		mcp.WithString("jql", mcp.Description("Extra JQL to narrow the sprint's issues, e.g. 'status != Done'")),
		mcp.WithString("fields", mcp.Description("Comma-separated fields to return, e.g. 'summary,status,assignee' (default: all)")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of issues (default 50, the API's own maximum)")),
		mcp.WithNumber("start_at", mcp.Description("0-based index of the first result to return (default 0); use it with limit to page past the API's 50-item cap")),
	)

	s.AddTool(getSprintIssuesTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sprintID, err := request.RequireString("sprint_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.GetSprintIssues(
			sprintID,
			request.GetString("jql", ""),
			request.GetString("fields", ""),
			request.GetInt("limit", 50),
			request.GetInt("start_at", 0),
		))
	})

	getBacklogTool := mcp.NewTool(
		"jira_get_backlog",
		mcp.WithDescription("Get a board's backlog: the issues on the board that are in no sprint "+
			"(get the board id from jira_get_boards)."),
		mcp.WithString("board_id", mcp.Required(), mcp.Description("Numeric board id from jira_get_boards")),
		mcp.WithString("jql", mcp.Description("Extra JQL to narrow the backlog, e.g. 'priority = High'")),
		mcp.WithString("fields", mcp.Description("Comma-separated fields to return, e.g. 'summary,status' (default: all)")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of issues (default 50, the API's own maximum)")),
		mcp.WithNumber("start_at", mcp.Description("0-based index of the first result to return (default 0); use it with limit to page past the API's 50-item cap")),
	)

	s.AddTool(getBacklogTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		boardID, err := request.RequireString("board_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.GetBacklog(
			boardID,
			request.GetString("jql", ""),
			request.GetString("fields", ""),
			request.GetInt("limit", 50),
			request.GetInt("start_at", 0),
		))
	})

	getEpicIssuesTool := mcp.NewTool(
		"jira_get_epic_issues",
		mcp.WithDescription("Get the issues belonging to an epic. The epic is addressed by its own issue key, "+
			"e.g. DEV-42 (find it with jira_search, 'issuetype = Epic')."),
		mcp.WithString("epic_key", mcp.Required(), mcp.Description("Epic issue key, e.g. DEV-42")),
		mcp.WithString("fields", mcp.Description("Comma-separated fields to return, e.g. 'summary,status' (default: all)")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of issues (default 50, the API's own maximum)")),
		mcp.WithNumber("start_at", mcp.Description("0-based index of the first result to return (default 0); use it with limit to page past the API's 50-item cap")),
	)

	s.AddTool(getEpicIssuesTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		epicKey, err := request.RequireString("epic_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.GetEpicIssues(
			epicKey,
			request.GetString("fields", ""),
			request.GetInt("limit", 50),
			request.GetInt("start_at", 0),
		))
	})
}

func registerJiraAgileWriteTools(s *server.MCPServer, client *jira.Client) {
	createSprintTool := mcp.NewTool(
		"jira_create_sprint",
		mcp.WithDescription("Create a future sprint on a board (get the board id from jira_get_boards). "+
			"The sprint is created but not started; start it from the board."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Sprint name, e.g. 'Sprint 12'")),
		mcp.WithString("board_id", mcp.Required(), mcp.Description("Numeric board id from jira_get_boards")),
		mcp.WithString("start_date", mcp.Description("Planned start, ISO 8601 or 'yyyy-MM-dd HH:mm' (optional)")),
		mcp.WithString("end_date", mcp.Description("Planned end, ISO 8601 or 'yyyy-MM-dd HH:mm' (optional)")),
		mcp.WithString("goal", mcp.Description("Sprint goal (optional)")),
	)

	s.AddTool(createSprintTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := request.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		boardID, err := request.RequireString("board_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.CreateSprint(
			name, boardID,
			request.GetString("start_date", ""),
			request.GetString("end_date", ""),
			request.GetString("goal", ""),
		))
	})

	moveToSprintTool := mcp.NewTool(
		"jira_move_issues_to_sprint",
		mcp.WithDescription("Move issues into a sprint (get the sprint id from jira_get_sprints). Only future and "+
			"active sprints accept issues. Jira takes at most 50 issues per request, so longer lists are sent in batches."),
		mcp.WithString("sprint_id", mcp.Required(), mcp.Description("Numeric sprint id from jira_get_sprints")),
		mcp.WithString("issue_keys", mcp.Required(), mcp.Description("Comma-separated issue keys, e.g. 'DEV-1, DEV-2'")),
	)

	s.AddTool(moveToSprintTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sprintID, err := request.RequireString("sprint_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		issueKeys, err := request.RequireString("issue_keys")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		message, err := client.MoveIssuesToSprint(sprintID, issueKeys)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(message), nil
	})

	moveToBacklogTool := mcp.NewTool(
		"jira_move_issues_to_backlog",
		mcp.WithDescription("Take issues out of their sprint and put them back on their board's backlog. "+
			"Jira takes at most 50 issues per request, so longer lists are sent in batches."),
		mcp.WithString("issue_keys", mcp.Required(), mcp.Description("Comma-separated issue keys, e.g. 'DEV-1, DEV-2'")),
	)

	s.AddTool(moveToBacklogTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		issueKeys, err := request.RequireString("issue_keys")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		message, err := client.MoveIssuesToBacklog(issueKeys)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(message), nil
	})

	moveToEpicTool := mcp.NewTool(
		"jira_move_issues_to_epic",
		mcp.WithDescription("Set the epic of a list of issues (the epic is addressed by its issue key, e.g. DEV-42). "+
			"The special epic key 'none' removes the issues from their current epic. Jira takes at most 50 issues "+
			"per request, so longer lists are sent in batches."),
		mcp.WithString("epic_key", mcp.Required(), mcp.Description("Epic issue key, e.g. DEV-42, or 'none' to remove the issues from their epic")),
		mcp.WithString("issue_keys", mcp.Required(), mcp.Description("Comma-separated issue keys, e.g. 'DEV-1, DEV-2'")),
	)

	s.AddTool(moveToEpicTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		epicKey, err := request.RequireString("epic_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		issueKeys, err := request.RequireString("issue_keys")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		message, err := client.MoveIssuesToEpic(epicKey, issueKeys)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(message), nil
	})
}
