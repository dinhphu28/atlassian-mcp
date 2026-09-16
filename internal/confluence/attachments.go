package confluence

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
)

// GetAttachments lists the attachments on a page, including each one's media
// type and version.
func (c *Client) GetAttachments(pageID string, limit int) (string, error) {
	path := fmt.Sprintf("/rest/api/content/%s/child/attachment?limit=%d&expand=version,metadata",
		url.PathEscape(pageID), limit)
	return c.get(path)
}

// Attachment is a downloaded attachment's bytes plus the metadata needed to
// present it.
type Attachment struct {
	Filename  string
	MediaType string
	Data      []byte
}

// DownloadAttachment fetches an attachment's bytes by its content ID.
func (c *Client) DownloadAttachment(attachmentID string) (*Attachment, error) {
	raw, err := c.get("/rest/api/content/" + url.PathEscape(attachmentID) + "?expand=metadata")
	if err != nil {
		return nil, err
	}

	var meta struct {
		Title    string `json:"title"`
		Metadata struct {
			MediaType string `json:"mediaType"`
		} `json:"metadata"`
		Links struct {
			Download string `json:"download"`
		} `json:"_links"`
	}
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		return nil, fmt.Errorf("cannot parse attachment %s: %w", attachmentID, err)
	}
	if meta.Links.Download == "" {
		return nil, fmt.Errorf("attachment %s has no download link", attachmentID)
	}

	// The download link is relative to the instance base, not /rest/api.
	data, err := c.getBytes(meta.Links.Download)
	if err != nil {
		return nil, err
	}

	return &Attachment{
		Filename:  meta.Title,
		MediaType: meta.Metadata.MediaType,
		Data:      data,
	}, nil
}

// AttachmentID returns the content ID of the attachment named filename on a
// page, or an empty string when the page has no attachment by that name.
func (c *Client) AttachmentID(pageID, filename string) (string, error) {
	raw, err := c.get(fmt.Sprintf("/rest/api/content/%s/child/attachment?limit=200&filename=%s",
		url.PathEscape(pageID), url.QueryEscape(filename)))
	if err != nil {
		return "", err
	}

	var resp struct {
		Results []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return "", fmt.Errorf("cannot parse attachments of %s: %w", pageID, err)
	}

	// Confluence applies the filename filter server-side; the title is checked
	// again so an ignored filter cannot hand back the wrong attachment.
	for _, r := range resp.Results {
		if r.Title == filename {
			return r.ID, nil
		}
	}

	return "", nil
}

// UploadAttachment uploads bytes as an attachment named filename on a page.
// Confluence rejects a second attachment with the same name, so an existing one
// is updated instead: the bytes become a new version of it and the name stays
// the same, which keeps any page markup that references it working.
func (c *Client) UploadAttachment(pageID, filename string, data []byte) (string, error) {
	base := "/rest/api/content/" + url.PathEscape(pageID) + "/child/attachment"

	existingID, err := c.AttachmentID(pageID, filename)
	if err != nil {
		return "", err
	}
	if existingID != "" {
		return c.uploadMultipart(base+"/"+url.PathEscape(existingID)+"/data", filename, data)
	}

	return c.uploadMultipart(base, filename, data)
}

// DeleteAttachment removes an attachment by its content ID.
func (c *Client) DeleteAttachment(attachmentID string) error {
	_, err := c.do(http.MethodDelete, "/rest/api/content/"+url.PathEscape(attachmentID), "")
	return err
}

// uploadMultipart sends data as a multipart file upload to path and returns the
// raw response body.
func (c *Client) uploadMultipart(path, filename string, data []byte) (string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	fw, err := w.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}
	if _, err := fw.Write(data); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, &buf)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", w.FormDataContentType())
	// Required by Confluence for attachment uploads (XSRF check bypass).
	req.Header.Set("X-Atlassian-Token", "nocheck")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("confluence error %d: %s", resp.StatusCode, string(respBody))
	}

	return string(respBody), nil
}
