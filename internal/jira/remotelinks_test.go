package jira

import (
	"reflect"
	"strings"
	"testing"
)

func TestRemoteLinkPayload(t *testing.T) {
	const pageURL = "https://confluence.example.com/display/DOC/Release+Notes"

	tests := []struct {
		name         string
		linkURL      string
		title        string
		summary      string
		relationship string
		globalID     string
		iconURL      string
		iconTitle    string
		existing     map[string]any
		want         map[string]any
		wantErr      bool
	}{
		{
			name:    "no global_id defaults to the url",
			linkURL: pageURL,
			title:   "Release Notes",
			want: map[string]any{
				"globalId": pageURL,
				"object":   map[string]any{"url": pageURL, "title": "Release Notes"},
			},
		},
		{
			name:     "explicit global_id is kept",
			linkURL:  pageURL,
			title:    "Release Notes",
			globalID: "confluence-page-12345",
			want: map[string]any{
				"globalId": "confluence-page-12345",
				"object":   map[string]any{"url": pageURL, "title": "Release Notes"},
			},
		},
		{
			name:         "optional fields are included when given",
			linkURL:      pageURL,
			title:        "Release Notes",
			summary:      "What shipped in 4.2",
			relationship: "documentation",
			iconURL:      "https://confluence.example.com/favicon.ico",
			iconTitle:    "Confluence",
			want: map[string]any{
				"globalId":     pageURL,
				"relationship": "documentation",
				"object": map[string]any{
					"url":     pageURL,
					"title":   "Release Notes",
					"summary": "What shipped in 4.2",
					"icon": map[string]any{
						"url16x16": "https://confluence.example.com/favicon.ico",
						"title":    "Confluence",
					},
				},
			},
		},
		{
			name:      "blank optional fields are omitted when there is no link to preserve",
			linkURL:   "  " + pageURL + "  ",
			title:     "  Release Notes  ",
			summary:   "   ",
			iconTitle: "Confluence",
			want: map[string]any{
				"globalId": pageURL,
				"object":   map[string]any{"url": pageURL, "title": "Release Notes"},
			},
		},
		{
			// The POST is a full replace, so a re-link that names only the url
			// and title must carry the existing link's other fields back.
			name:    "an existing link's fields survive a re-link",
			linkURL: pageURL,
			title:   "Release Notes",
			existing: map[string]any{
				"globalId":     pageURL,
				"relationship": "documentation",
				"application":  map[string]any{"type": "com.atlassian.confluence", "name": "Confluence"},
				"object": map[string]any{
					"url":     pageURL,
					"title":   "Old title",
					"summary": "What shipped in 4.2",
					"icon":    map[string]any{"url16x16": "https://confluence.example.com/favicon.ico"},
					"status":  map[string]any{"resolved": false},
				},
			},
			want: map[string]any{
				"globalId":     pageURL,
				"relationship": "documentation",
				"application":  map[string]any{"type": "com.atlassian.confluence", "name": "Confluence"},
				"object": map[string]any{
					"url":     pageURL,
					"title":   "Release Notes",
					"summary": "What shipped in 4.2",
					"icon":    map[string]any{"url16x16": "https://confluence.example.com/favicon.ico"},
					"status":  map[string]any{"resolved": false},
				},
			},
		},
		{
			name:         "supplied fields win over the existing link",
			linkURL:      pageURL,
			title:        "Release Notes",
			summary:      "What shipped in 4.3",
			relationship: "mentioned in",
			existing: map[string]any{
				"relationship": "documentation",
				"object":       map[string]any{"summary": "What shipped in 4.2"},
			},
			want: map[string]any{
				"globalId":     pageURL,
				"relationship": "mentioned in",
				"object": map[string]any{
					"url":     pageURL,
					"title":   "Release Notes",
					"summary": "What shipped in 4.3",
				},
			},
		},
		{name: "url is required", title: "Release Notes", wantErr: true},
		{name: "title is required", linkURL: pageURL, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := remoteLinkPayload(tt.linkURL, tt.title, tt.summary, tt.relationship, tt.globalID, tt.iconURL, tt.iconTitle, tt.existing)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("remoteLinkPayload() = %+v, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("remoteLinkPayload() error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("remoteLinkPayload() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRemoteLinkDeleteQuery(t *testing.T) {
	tests := []struct {
		name     string
		linkID   string
		globalID string
		want     string
		wantErr  bool
	}{
		{name: "by link id", linkID: "10100", want: "/10100"},
		{
			name:     "by global id, escaped",
			globalID: "system=http://www.mycompany.com/support&id=1",
			want:     "?globalId=system%3Dhttp%3A%2F%2Fwww.mycompany.com%2Fsupport%26id%3D1",
		},
		{name: "link id wins over global id", linkID: "10100", globalID: "anything", want: "/10100"},
		{name: "neither is an error", wantErr: true},
		{name: "blank is an error", linkID: "  ", globalID: " ", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := remoteLinkDeleteQuery(tt.linkID, tt.globalID)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("remoteLinkDeleteQuery(%q, %q) = %q, want an error", tt.linkID, tt.globalID, got)
				}
				// The error has to tell the caller where a link id comes from.
				if !strings.Contains(err.Error(), "jira_get_remote_links") {
					t.Errorf("remoteLinkDeleteQuery error = %v, want it to name jira_get_remote_links", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("remoteLinkDeleteQuery(%q, %q) error: %v", tt.linkID, tt.globalID, err)
			}
			if got != tt.want {
				t.Errorf("remoteLinkDeleteQuery(%q, %q) = %q, want %q", tt.linkID, tt.globalID, got, tt.want)
			}
		})
	}
}
