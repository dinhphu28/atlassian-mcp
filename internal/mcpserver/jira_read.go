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
		mcp.WithDescription("Search Jira issues with a JQL query"),
		mcp.WithString("jql", mcp.Required(), mcp.Description("JQL expression, e.g. 'project = DEV AND status = Open ORDER BY created DESC'")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of issues (default 25)")),
	)

	s.AddTool(searchTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		jql, err := request.RequireString("jql")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.Search(jql, request.GetInt("limit", 25)))
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
		mcp.WithDescription("List the status transitions available for a Jira issue (ids for jira_transition_issue)"),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
	)

	s.AddTool(getTransitionsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.GetTransitions(key))
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
