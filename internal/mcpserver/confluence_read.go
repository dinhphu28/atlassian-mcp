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
			"(matched against page text) or a raw 'cql' expression for full control. "+
			"Results come back one page at a time: when the response carries _links.next there "+
			"are more, and 'start' is how you reach them."),
		mcp.WithString("query", mcp.Description("Search keyword (matched as CQL text ~ \"query\")")),
		mcp.WithString("cql", mcp.Description("Raw CQL expression, e.g. 'space = DEV AND label = api'. Overrides 'query' when set.")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of results (default 10)")),
		mcp.WithNumber("start", mcp.Description("Zero-based index of the first result to return (default 0). "+
			"Advance it by 'limit' to follow _links.next and read past the first page.")),
	)

	s.AddTool(searchTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query := request.GetString("query", "")
		cql := request.GetString("cql", "")
		limit := request.GetInt("limit", 10)
		start := request.GetInt("start", 0)

		if cql == "" {
			if query == "" {
				return mcp.NewToolResultError("either 'query' or 'cql' is required"), nil
			}
			cql = fmt.Sprintf(`text ~ "%s"`, query)
		}

		return jsonResult(client.Search(cql, limit, start))
	})

	getPageTool := mcp.NewTool(
		"confluence_get_page",
		mcp.WithDescription("Get a Confluence page by ID. Returns the body as Markdown by default, which is "+
			"lossy: macros, panels, layouts, status lozenges and task lists do not survive the conversion. "+
			"Read with representation='storage' whenever you intend to write the body back, or need a "+
			"Confluence-native construct Markdown cannot express. "+
			"Pass 'version' to read a historical revision instead of the current one: that is how you "+
			"diff an old revision, or restore it by reading it here and writing it back with "+
			"confluence_update_page. It works with both representations and with output_path. "+
			"The version number a historical read reports back is the OLD one: do not pass it as "+
			"expected_version to confluence_update_page (that would always conflict) - omit "+
			"expected_version, or re-read the current page to get one."),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("Confluence page ID")),
		mcp.WithString("representation",
			mcp.Enum(reprMarkdown, reprStorage),
			mcp.DefaultString(reprMarkdown),
			mcp.Description("Body format: 'markdown' (default, lossy) or 'storage' (Confluence storage XHTML, exact)")),
		mcp.WithString("output_path", mcp.Description("Optional path to write the body to (parent directories are created); "+
			"returns page metadata instead of the body. Honoured for both representations: writes Markdown, "+
			"or the raw storage XHTML when representation='storage'.")),
		mcp.WithBoolean("body_only", mcp.Description("storage only: return just the storage XHTML instead of the full REST JSON envelope (default false)")),
		mcp.WithNumber("version", mcp.Description("Read this historical version of the page instead of the current one "+
			"(version numbers come from confluence_get_page_history); omit or 0 for the current content")),
	)

	s.AddTool(getPageTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		repr, err := bodyRepresentation(request, reprMarkdown, reprStorage)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		out := request.GetString("output_path", "")
		version := request.GetInt("version", 0)

		if repr == reprStorage {
			// The JSON envelope is the cheapest answer, but it cannot be written
			// to a file or handed over as editable XHTML without being decoded.
			if out == "" && !request.GetBool("body_only", false) {
				return jsonResult(client.GetPageAt(pageID, version))
			}

			page, err := client.GetPageStorageAt(pageID, version)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if out == "" {
				return mcp.NewToolResultText(page.Storage), nil
			}
			return writeBodyResult(out, page.Storage, page.ID, page.Title, page.Space, page.Version, version > 0)
		}

		page, err := client.GetPageMarkdownAt(pageID, version)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		body := page.Markdown
		if notice := lossNotice(page.Dropped); notice != "" {
			body = notice + "\n" + body
		}

		if out != "" {
			return writeBodyResult(out, body, page.ID, page.Title, page.Space, page.Version, version > 0)
		}

		return mcp.NewToolResultText(body), nil
	})

	getChildrenTool := mcp.NewTool(
		"confluence_get_page_children",
		mcp.WithDescription("List the child pages directly under a Confluence page. "+
			"A response carrying _links.next has more children; pass 'start' to read them."),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("Parent Confluence page ID")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of children (default 25)")),
		mcp.WithNumber("start", mcp.Description("Zero-based index of the first child to return (default 0). "+
			"Advance it by 'limit' to follow _links.next.")),
	)

	s.AddTool(getChildrenTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.GetPageChildren(pageID, request.GetInt("limit", 25), request.GetInt("start", 0)))
	})

	getCommentsTool := mcp.NewTool(
		"confluence_get_comments",
		mcp.WithDescription("Get the comments on a Confluence page, including nested replies. "+
			"Returns Markdown by default, each comment prefixed with its author and date; "+
			"use representation 'storage' for raw JSON. A response carrying _links.next has more "+
			"comments; pass 'start' to read them."),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("Confluence page ID")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of comments (default 25)")),
		mcp.WithNumber("start", mcp.Description("Zero-based index of the first comment to return (default 0). "+
			"Advance it by 'limit' to follow _links.next.")),
		mcp.WithString("representation",
			mcp.Enum(reprMarkdown, reprStorage),
			mcp.DefaultString(reprMarkdown),
			mcp.Description("Body format: 'markdown' (default, lossy) or 'storage' (raw JSON with storage XHTML bodies)")),
	)

	s.AddTool(getCommentsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		limit := request.GetInt("limit", 25)
		start := request.GetInt("start", 0)

		repr, err := bodyRepresentation(request, reprMarkdown, reprStorage)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if repr == reprStorage {
			return jsonResult(client.GetComments(pageID, limit, start))
		}

		md, err := client.GetCommentsMarkdown(pageID, limit, start)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(md), nil
	})

	getAttachmentsTool := mcp.NewTool(
		"confluence_get_attachments",
		mcp.WithDescription("List the attachments (images, files) on a Confluence page. "+
			"A response carrying _links.next has more attachments; pass 'start' to read them."),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("Confluence page ID")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of attachments (default 25)")),
		mcp.WithNumber("start", mcp.Description("Zero-based index of the first attachment to return (default 0). "+
			"Advance it by 'limit' to follow _links.next.")),
	)

	s.AddTool(getAttachmentsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.GetAttachmentsFrom(pageID, request.GetInt("limit", 25), request.GetInt("start", 0)))
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

// writeBodyResult writes a page body to path, creating the parent directory so a
// not-yet-existing output folder is not an error, and reports the page metadata
// instead of the body.
func writeBodyResult(path, body, id, title, space string, version int, historical bool) (*mcp.CallToolResult, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// A historical read hands back the version that was asked for, not the one
	// the page is at, so it must not be mistaken for an expected_version.
	versionText := fmt.Sprintf("version %d", version)
	if historical {
		versionText = fmt.Sprintf("historical version %d - not an expected_version; omit expected_version "+
			"or re-read the current page for one", version)
	}

	return mcp.NewToolResultText(fmt.Sprintf(
		"Wrote page %s (title %q, space %s, %s) to %s",
		id, title, space, versionText, path)), nil
}

// lossNotice turns the constructs a Markdown conversion could not represent into
// one inert Markdown comment, so an agent reading a page can tell that editing
// this body as Markdown would drop them.
func lossNotice(dropped []string) string {
	if len(dropped) == 0 {
		return ""
	}

	return fmt.Sprintf("<!-- confluence-mcp: %d Confluence construct(s) were flattened or dropped by the "+
		"Markdown conversion (%s). Re-read with representation=\"storage\" to edit this page without losing them. -->\n",
		len(dropped), strings.Join(dropped, ", "))
}
