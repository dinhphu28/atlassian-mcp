package mcpserver

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"dinhphu28/atlassian-mcp/internal/jira"
)

func registerJiraReadTools(s *server.MCPServer, client *jira.Client) {
	searchTool := mcp.NewTool(
		"jira_search",
		mcp.WithDescription("Search Jira issues with a JQL query. Page with start_at, and prefer naming the "+
			"fields you need: by default Jira returns every field of every issue, so a fields selector such as "+
			"'summary,status,assignee' cuts the response size dramatically."),
		mcp.WithString("jql", mcp.Required(), mcp.Description("JQL expression, e.g. 'project = DEV AND status = Open ORDER BY created DESC'")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of issues (default 25)")),
		mcp.WithNumber("start_at", mcp.Description("0-based index of the first issue to return (default 0); use it with limit to page")),
		mcp.WithString("fields", mcp.Description("Comma-separated fields to return, e.g. 'summary,status,assignee'. "+
			"Omit for every field (much larger); '*all' and '-field' exclusions also work.")),
	)

	s.AddTool(searchTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		jql, err := request.RequireString("jql")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.Search(
			jql,
			request.GetInt("limit", 25),
			request.GetInt("start_at", 0),
			request.GetString("fields", ""),
		))
	})

	getIssueTool := mcp.NewTool(
		"jira_get_issue",
		mcp.WithDescription("Get a Jira issue by key or ID"),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
	)

	s.AddTool(getIssueTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.GetIssue(key))
	})

	getCommentsTool := mcp.NewTool(
		"jira_get_comments",
		mcp.WithDescription("Get the comments on a Jira issue"),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
	)

	s.AddTool(getCommentsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.GetComments(key))
	})

	getTransitionsTool := mcp.NewTool(
		"jira_get_transitions",
		mcp.WithDescription("List the status transitions available for a Jira issue (ids for jira_transition_issue). "+
			"Set expand_fields to see each transition's screen fields, i.e. what jira_transition_issue must supply "+
			"(a required 'resolution' is the usual one)."),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
		mcp.WithBoolean("expand_fields", mcp.Description("Include each transition's screen fields, with their allowed values and whether they are required (default false)")),
	)

	s.AddTool(getTransitionsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.GetTransitions(key, request.GetBool("expand_fields", false)))
	})

	getWorklogsTool := mcp.NewTool(
		"jira_get_worklogs",
		mcp.WithDescription("Get the work logged on a Jira issue (worklog ids for jira_update_worklog / jira_delete_worklog)"),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
	)

	s.AddTool(getWorklogsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.GetWorklogs(key))
	})

	downloadAttachmentsTool := mcp.NewTool(
		"jira_download_attachments",
		mcp.WithDescription("Download all attachments on a Jira issue. Images are returned as viewable images; other files as base64."),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
	)

	s.AddTool(downloadAttachmentsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		metas, err := client.IssueAttachments(key)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jiraAttachmentsResult(client, metas, false)
	})

	getIssueImagesTool := mcp.NewTool(
		"jira_get_issue_images",
		mcp.WithDescription("Get the image attachments on a Jira issue as viewable images."),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
	)

	s.AddTool(getIssueImagesTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		metas, err := client.IssueAttachments(key)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jiraAttachmentsResult(client, metas, true)
	})

	getPrioritiesTool := mcp.NewTool(
		"jira_get_priorities",
		mcp.WithDescription("List the priorities configured on this Jira instance (names and ids for jira_set_priority)"),
	)

	s.AddTool(getPrioritiesTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return jsonResult(client.GetPriorities())
	})

	getIssueLinkTypesTool := mcp.NewTool(
		"jira_get_issue_link_types",
		mcp.WithDescription("List the issue link types configured on this Jira instance "+
			"(names and direction descriptions, e.g. Blocks / 'blocks' / 'is blocked by', for jira_link_issues)"),
	)

	s.AddTool(getIssueLinkTypesTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return jsonResult(client.GetIssueLinkTypes())
	})
}

// jiraAttachmentsResult downloads the given attachments and builds a multi-block
// result: a caption per file plus a viewable image block for images (or base64
// text for other files). When imagesOnly is true, non-image files are skipped.
func jiraAttachmentsResult(client *jira.Client, metas []jira.AttachmentMeta, imagesOnly bool) (*mcp.CallToolResult, error) {
	var contents []mcp.Content
	count := 0

	for _, m := range metas {
		isImage := strings.HasPrefix(m.MimeType, "image/")
		if imagesOnly && !isImage {
			continue
		}

		att, err := client.DownloadAttachment(m)
		if err != nil {
			contents = append(contents, mcp.NewTextContent(fmt.Sprintf("%s: download failed: %v", m.Filename, err)))
			continue
		}

		caption := fmt.Sprintf("%s (%s, %d bytes)", att.Filename, att.MediaType, len(att.Data))
		encoded := base64.StdEncoding.EncodeToString(att.Data)
		if isImage {
			contents = append(contents, mcp.NewTextContent(caption), mcp.NewImageContent(encoded, att.MediaType))
		} else {
			contents = append(contents, mcp.NewTextContent(caption+"\nbase64:\n"+encoded))
		}
		count++
	}

	if count == 0 {
		if imagesOnly {
			return mcp.NewToolResultText("No image attachments found."), nil
		}
		return mcp.NewToolResultText("No attachments found."), nil
	}

	return &mcp.CallToolResult{Content: contents}, nil
}
