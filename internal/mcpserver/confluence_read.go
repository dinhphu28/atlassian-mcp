package mcpserver

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"dinhphu28/atlassian-mcp/internal/confluence"
)

func registerConfluenceReadTools(s *server.MCPServer, client *confluence.Client) {
	searchTool := mcp.NewTool(
		"confluence_search",
		mcp.WithDescription("Search Confluence content. Provide either a free-text 'query' "+
			"(matched against page text) or a raw 'cql' expression for full control."),
		mcp.WithString("query", mcp.Description("Search keyword (matched as CQL text ~ \"query\")")),
		mcp.WithString("cql", mcp.Description("Raw CQL expression, e.g. 'space = DEV AND label = api'. Overrides 'query' when set.")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of results (default 10)")),
	)

	s.AddTool(searchTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query := request.GetString("query", "")
		cql := request.GetString("cql", "")
		limit := request.GetInt("limit", 10)

		if cql == "" {
			if query == "" {
				return mcp.NewToolResultError("either 'query' or 'cql' is required"), nil
			}
			cql = fmt.Sprintf(`text ~ "%s"`, query)
		}

		return jsonResult(client.Search(cql, limit))
	})

	getPageTool := mcp.NewTool(
		"confluence_get_page",
		mcp.WithDescription("Get a Confluence page by ID. Returns the body as Markdown by default."),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("Confluence page ID")),
		mcp.WithString("representation", mcp.Description("Body format: 'markdown' (default) or 'storage' (raw JSON)")),
		mcp.WithString("output_path", mcp.Description("Optional path to write the Markdown to; returns metadata instead of the body")),
	)

	s.AddTool(getPageTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if request.GetString("representation", "markdown") == "storage" {
			return jsonResult(client.GetPage(pageID))
		}

		page, err := client.GetPageMarkdown(pageID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if out := request.GetString("output_path", ""); out != "" {
			// Create the parent directory so a not-yet-existing output folder is
			// not an error.
			if dir := filepath.Dir(out); dir != "" {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
			}
			if err := os.WriteFile(out, []byte(page.Markdown), 0o644); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf(
				"Wrote page %s (title %q, space %s, version %d) to %s",
				page.ID, page.Title, page.Space, page.Version, out)), nil
		}

		return mcp.NewToolResultText(page.Markdown), nil
	})

	getChildrenTool := mcp.NewTool(
		"confluence_get_page_children",
		mcp.WithDescription("List the child pages directly under a Confluence page"),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("Parent Confluence page ID")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of children (default 25)")),
	)

	s.AddTool(getChildrenTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.GetPageChildren(pageID, request.GetInt("limit", 25)))
	})

	getCommentsTool := mcp.NewTool(
		"confluence_get_comments",
		mcp.WithDescription("Get the comments on a Confluence page, including nested replies. "+
			"Returns Markdown by default, each comment prefixed with its author and date; "+
			"use representation 'storage' for raw JSON."),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("Confluence page ID")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of comments (default 25)")),
		mcp.WithString("representation", mcp.Description("Body format: 'markdown' (default) or 'storage' (raw JSON)")),
	)

	s.AddTool(getCommentsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		limit := request.GetInt("limit", 25)

		if request.GetString("representation", "markdown") == "storage" {
			return jsonResult(client.GetComments(pageID, limit))
		}

		md, err := client.GetCommentsMarkdown(pageID, limit)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(md), nil
	})

	getAttachmentsTool := mcp.NewTool(
		"confluence_get_attachments",
		mcp.WithDescription("List the attachments (images, files) on a Confluence page"),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("Confluence page ID")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of attachments (default 25)")),
	)

	s.AddTool(getAttachmentsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.GetAttachments(pageID, request.GetInt("limit", 25)))
	})

	downloadAttachmentTool := mcp.NewTool(
		"confluence_download_attachment",
		mcp.WithDescription("Download an attachment by its ID. Images are returned as viewable "+
			"images; other files as base64. Get the ID from confluence_get_attachments."),
		mcp.WithString("attachment_id", mcp.Required(), mcp.Description("Attachment content ID (e.g. att12345)")),
	)

	s.AddTool(downloadAttachmentTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		attachmentID, err := request.RequireString("attachment_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		att, err := client.DownloadAttachment(attachmentID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		encoded := base64.StdEncoding.EncodeToString(att.Data)
		caption := fmt.Sprintf("%s (%s, %d bytes)", att.Filename, att.MediaType, len(att.Data))

		if strings.HasPrefix(att.MediaType, "image/") {
			return mcp.NewToolResultImage(caption, encoded, att.MediaType), nil
		}

		return mcp.NewToolResultText(caption + "\nbase64:\n" + encoded), nil
	})

	getLabelsTool := mcp.NewTool(
		"confluence_get_labels",
		mcp.WithDescription("Get the labels on a Confluence page"),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("Confluence page ID")),
	)

	s.AddTool(getLabelsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.GetLabels(pageID))
	})

	getPageHistoryTool := mcp.NewTool(
		"confluence_get_page_history",
		mcp.WithDescription("Get the version history of a Confluence page"),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("Confluence page ID")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of versions (default 25)")),
	)

	s.AddTool(getPageHistoryTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.GetPageHistory(pageID, request.GetInt("limit", 25)))
	})
}
