package jira

import (
	"encoding/json"
	"testing"
)

// entry builds a changelogEntry from the compact wire shape, so the tests read
// like the JSON Jira actually returns.
func historyEntry(t *testing.T, raw string) changelogEntry {
	t.Helper()
	var e changelogEntry
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		t.Fatalf("cannot parse test changelog entry: %v", err)
	}
	return e
}

func TestCompactChangelog(t *testing.T) {
	entries := []changelogEntry{
		historyEntry(t, `{"created":"2024-01-01T10:00:00.000+0000","author":{"displayName":"Ada Lovelace","name":"ada"},
			"items":[{"field":"status","from":"1","fromString":"Open","to":"3","toString":"In Progress"}]}`),
		historyEntry(t, `{"created":"2024-01-02T10:00:00.000+0000","author":{"displayName":"","name":"grace"},
			"items":[{"field":"assignee","from":null,"fromString":null,"to":"grace","toString":"Grace Hopper"},
			         {"field":"description","from":null,"fromString":null,"to":null,"toString":"now set"}]}`),
		historyEntry(t, `{"created":"2024-01-03T10:00:00.000+0000","author":{"displayName":"Ada Lovelace","name":"ada"},
			"items":[{"field":"Sprint","from":"5","fromString":null,"to":null,"toString":null}]}`),
	}

	tests := []struct {
		name        string
		limit       int
		wantCount   int
		wantFirstAt string
	}{
		{name: "no limit keeps every entry", limit: 0, wantCount: 3, wantFirstAt: "2024-01-01T10:00:00.000+0000"},
		{name: "negative limit keeps every entry", limit: -1, wantCount: 3, wantFirstAt: "2024-01-01T10:00:00.000+0000"},
		{name: "limit keeps the most recent", limit: 2, wantCount: 2, wantFirstAt: "2024-01-02T10:00:00.000+0000"},
		{name: "limit above the count is harmless", limit: 50, wantCount: 3, wantFirstAt: "2024-01-01T10:00:00.000+0000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compactChangelog(entries, tt.limit)
			if len(got) != tt.wantCount {
				t.Fatalf("compactChangelog(limit %d) returned %d entries, want %d", tt.limit, len(got), tt.wantCount)
			}
			if got[0]["created"] != tt.wantFirstAt {
				t.Errorf("compactChangelog(limit %d) starts at %v, want %s", tt.limit, got[0]["created"], tt.wantFirstAt)
			}
		})
	}

	// Oldest first, so the newest change reads last.
	got := compactChangelog(entries, 0)
	if got[len(got)-1]["created"] != "2024-01-03T10:00:00.000+0000" {
		t.Errorf("last entry = %v, want the newest change", got[len(got)-1]["created"])
	}

	if got[0]["author"] != "Ada Lovelace" {
		t.Errorf("author = %v, want the display name", got[0]["author"])
	}
	// A user with no display name still has to be identifiable.
	if got[1]["author"] != "grace" {
		t.Errorf("author without a display name = %v, want the username", got[1]["author"])
	}

	items := got[0]["items"].([]map[string]any)
	if len(items) != 1 || items[0]["field"] != "status" {
		t.Fatalf("items = %+v, want one status change", items)
	}
	if items[0]["from"] != "Open" || items[0]["to"] != "In Progress" {
		t.Errorf("status change = %v -> %v, want Open -> In Progress", items[0]["from"], items[0]["to"])
	}

	// A null side stays null rather than becoming an empty string, so "was
	// never set" is distinguishable from "was set to nothing".
	items = got[1]["items"].([]map[string]any)
	if items[0]["from"] != nil {
		t.Errorf("from of a first-time assignment = %v, want nil", items[0]["from"])
	}
	if items[0]["to"] != "Grace Hopper" {
		t.Errorf("to = %v, want the display string", items[0]["to"])
	}
	if items[1]["from"] != nil || items[1]["to"] != "now set" {
		t.Errorf("description change = %v -> %v, want nil -> \"now set\"", items[1]["from"], items[1]["to"])
	}

	// Without a display string the raw id is better than nothing.
	items = got[2]["items"].([]map[string]any)
	if items[0]["from"] != "5" || items[0]["to"] != nil {
		t.Errorf("sprint change = %v -> %v, want \"5\" -> nil", items[0]["from"], items[0]["to"])
	}

	if empty := compactChangelog(nil, 10); len(empty) != 0 {
		t.Errorf("compactChangelog(nil, 10) = %+v, want no entries", empty)
	}
}

func TestBulkIssueUpdates(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantLen int
		wantErr bool
	}{
		{
			name:    "bare fields objects are wrapped",
			input:   `[{"project":{"key":"DEV"},"summary":"one"},{"project":{"key":"DEV"},"summary":"two"}]`,
			wantLen: 2,
		},
		{
			name:    "an element that already has fields is left alone",
			input:   `[{"fields":{"project":{"key":"DEV"},"summary":"one"}}]`,
			wantLen: 1,
		},
		{name: "whitespace around the array is ignored", input: "  [{\"summary\":\"one\"}]\n", wantLen: 1},
		{name: "empty array", input: `[]`, wantErr: true},
		{name: "empty string", input: ``, wantErr: true},
		{name: "not an array", input: `{"summary":"one"}`, wantErr: true},
		{name: "not JSON", input: `summary: one`, wantErr: true},
		{name: "element is not an object", input: `["DEV-1"]`, wantErr: true},
		{name: "element is null", input: `[null]`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := bulkIssueUpdates(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("bulkIssueUpdates(%q) = %+v, want an error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("bulkIssueUpdates(%q) error: %v", tt.input, err)
			}
			if len(got) != tt.wantLen {
				t.Fatalf("bulkIssueUpdates(%q) returned %d updates, want %d", tt.input, len(got), tt.wantLen)
			}
			for i, u := range got {
				fields, ok := u["fields"].(map[string]any)
				if !ok {
					t.Fatalf("update %d = %+v, want a {\"fields\": …} wrapper", i, u)
				}
				if len(u) != 1 {
					t.Errorf("update %d = %+v, want only the fields key", i, u)
				}
				if fields["summary"] == nil {
					t.Errorf("update %d lost its summary: %+v", i, fields)
				}
			}
		})
	}

	// Wrapping must not nest an already-wrapped element one level deeper.
	got, err := bulkIssueUpdates(`[{"fields":{"summary":"one"}}]`)
	if err != nil {
		t.Fatalf("bulkIssueUpdates error: %v", err)
	}
	fields := got[0]["fields"].(map[string]any)
	if _, nested := fields["fields"]; nested {
		t.Errorf("an already-wrapped element was wrapped again: %+v", got[0])
	}
}
