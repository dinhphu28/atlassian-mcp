package jira

import (
	"strings"
	"testing"
)

func TestMatchPriority(t *testing.T) {
	catalog := []priority{
		{ID: "1", Name: "Highest"},
		{ID: "2", Name: "High"},
		{ID: "3", Name: "Medium"},
	}

	tests := []struct {
		name    string
		input   string
		wantID  string
		wantErr bool
	}{
		{name: "exact name", input: "High", wantID: "2"},
		{name: "name is case insensitive", input: "mEdIuM", wantID: "3"},
		{name: "surrounding space is ignored", input: "  Highest  ", wantID: "1"},
		{name: "numeric id", input: "3", wantID: "3"},
		{name: "unknown name", input: "Urgent", wantErr: true},
		{name: "unknown id", input: "99", wantErr: true},
		{name: "empty", input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := matchPriority(catalog, tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("matchPriority(%q) = %+v, want an error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("matchPriority(%q) error: %v", tt.input, err)
			}
			if got.ID != tt.wantID {
				t.Errorf("matchPriority(%q) = id %s, want id %s", tt.input, got.ID, tt.wantID)
			}
		})
	}

	// An unknown priority should name the valid ones, since the set is
	// per-instance and the caller cannot guess it.
	_, err := matchPriority(catalog, "Urgent")
	if err == nil || !strings.Contains(err.Error(), "Highest (id 1)") {
		t.Errorf("matchPriority error = %v, want it to list the configured priorities", err)
	}

	if _, err := matchPriority(nil, "High"); err == nil {
		t.Error("matchPriority(nil, \"High\") = nil error, want an error")
	}
}
