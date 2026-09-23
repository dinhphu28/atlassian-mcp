package jira

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestSplitIssueKeys(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{name: "single key", input: "DEV-1", want: []string{"DEV-1"}},
		{name: "spaces are trimmed", input: " DEV-1 , DEV-2 ", want: []string{"DEV-1", "DEV-2"}},
		{name: "blank entries are dropped", input: "DEV-1,,DEV-2,", want: []string{"DEV-1", "DEV-2"}},
		{name: "empty", input: "", wantErr: true},
		{name: "only separators", input: " , , ", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := splitIssueKeys(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("splitIssueKeys(%q) = %v, want an error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("splitIssueKeys(%q) error: %v", tt.input, err)
			}
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Errorf("splitIssueKeys(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestChunkIssues(t *testing.T) {
	keys := func(n int) []string {
		out := make([]string, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, fmt.Sprintf("DEV-%d", i+1))
		}
		return out
	}

	tests := []struct {
		name       string
		input      []string
		wantChunks []int
	}{
		{name: "empty", input: nil, wantChunks: nil},
		{name: "under the cap", input: keys(3), wantChunks: []int{3}},
		{name: "one short of the cap", input: keys(49), wantChunks: []int{49}},
		{name: "exactly the cap", input: keys(50), wantChunks: []int{50}},
		{name: "one over the cap", input: keys(51), wantChunks: []int{50, 1}},
		{name: "several chunks", input: keys(120), wantChunks: []int{50, 50, 20}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chunkIssues(tt.input, agileMoveLimit)
			if len(got) != len(tt.wantChunks) {
				t.Fatalf("chunkIssues(%d keys) = %d chunks, want %d", len(tt.input), len(got), len(tt.wantChunks))
			}

			total := 0
			for i, chunk := range got {
				if len(chunk) != tt.wantChunks[i] {
					t.Errorf("chunk %d has %d keys, want %d", i, len(chunk), tt.wantChunks[i])
				}
				total += len(chunk)
			}
			if total != len(tt.input) {
				t.Errorf("chunks cover %d keys, want all %d", total, len(tt.input))
			}

			// Chunking must preserve order, so a partial failure reports the
			// issues that really moved.
			var flat []string
			for _, chunk := range got {
				flat = append(flat, chunk...)
			}
			if strings.Join(flat, "|") != strings.Join(tt.input, "|") {
				t.Errorf("chunkIssues reordered or dropped keys: %v", flat)
			}
		})
	}

	if got := chunkIssues([]string{"DEV-1"}, 0); got != nil {
		t.Errorf("chunkIssues with size 0 = %v, want nil rather than an infinite loop", got)
	}
}

func TestNormalizeSprintState(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty means no filter", input: "", want: ""},
		{name: "single state", input: "active", want: "active"},
		{name: "case is normalised", input: "ACTIVE", want: "active"},
		{name: "several states", input: "future, active", want: "future,active"},
		{name: "unknown state", input: "open", wantErr: true},
		{name: "one bad state in a list", input: "active,open", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeSprintState(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalizeSprintState(%q) = %q, want an error", tt.input, got)
				}
				if !strings.Contains(err.Error(), "future") {
					t.Errorf("normalizeSprintState error = %v, want it to list the valid states", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeSprintState(%q) error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("normalizeSprintState(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestAgileID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "numeric", input: "42", want: "42"},
		{name: "space is trimmed", input: " 42 ", want: "42"},
		{name: "board name is not an id", input: "Team Board", wantErr: true},
		{name: "issue key is not an id", input: "DEV-1", wantErr: true},
		{name: "empty", input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := agileID("board", tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("agileID(%q) = %q, want an error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("agileID(%q) error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("agileID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeSprintDate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty stays empty", input: "", want: ""},
		{name: "date only", input: "2026-01-05", want: "2026-01-05T00:00:00.000"},
		{name: "date and time", input: "2026-01-05 09:30", want: "2026-01-05T09:30:00.000"},
		{name: "garbage", input: "next monday", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeSprintDate("start_date", tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalizeSprintDate(%q) = %q, want an error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeSprintDate(%q) error: %v", tt.input, err)
			}
			// The zone offset depends on the test machine, so only the
			// zone-independent prefix is asserted.
			if !strings.HasPrefix(got, tt.want) {
				t.Errorf("normalizeSprintDate(%q) = %q, want it to start with %q", tt.input, got, tt.want)
			}
		})
	}

	// An RFC 3339 input keeps its own zone rather than being shifted.
	got, err := normalizeSprintDate("end_date", "2026-01-05T09:30:00Z")
	if err != nil {
		t.Fatalf("normalizeSprintDate(RFC 3339) error: %v", err)
	}
	if got != "2026-01-05T09:30:00.000+00:00" {
		t.Errorf("normalizeSprintDate(RFC 3339) = %q, want the same instant in the Agile layout", got)
	}
}

func TestAgileQuery(t *testing.T) {
	tests := []struct {
		name    string
		jql     string
		fields  string
		limit   int
		startAt int
		want    string
	}{
		{name: "nothing set", want: ""},
		{name: "limit only", limit: 25, want: "?maxResults=25"},
		{name: "paging past the first page", limit: 50, startAt: 50, want: "?maxResults=50&startAt=50"},
		{name: "zero start is omitted", limit: 50, want: "?maxResults=50"},
		{name: "zero limit is omitted", jql: "status = Open", want: "?jql=status+%3D+Open"},
		{name: "fields are trimmed", fields: " summary , status ", want: "?fields=summary%2Cstatus"},
		{name: "everything", jql: "a = b", fields: "summary", limit: 10, want: "?fields=summary&jql=a+%3D+b&maxResults=10"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := agileQuery(tt.jql, tt.fields, tt.limit, tt.startAt); got != tt.want {
				t.Errorf("agileQuery(%q, %q, %d, %d) = %q, want %q", tt.jql, tt.fields, tt.limit, tt.startAt, got, tt.want)
			}
		})
	}
}

func TestExplainAgileError(t *testing.T) {
	if got := explainAgileError(nil); got != nil {
		t.Errorf("explainAgileError(nil) = %v, want nil", got)
	}

	other := errors.New("jira error 403: forbidden")
	if got := explainAgileError(other); got != other {
		t.Errorf("explainAgileError(403) = %v, want it passed through unchanged", got)
	}

	notFound := errors.New("jira error 404: null")
	got := explainAgileError(notFound)
	if !strings.Contains(got.Error(), "Jira Software") {
		t.Errorf("explainAgileError(404) = %v, want it to explain the missing Jira Software / board", got)
	}
	if !errors.Is(got, notFound) {
		t.Errorf("explainAgileError(404) = %v, want it to wrap the original error", got)
	}
}
