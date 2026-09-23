package jira

import (
	"strings"
	"testing"
)

func TestXrayMembershipPayload(t *testing.T) {
	tests := []struct {
		name    string
		add     []string
		remove  []string
		want    string
		wantErr bool
	}{
		{name: "add only", add: []string{"DEV-1", "DEV-2"}, want: `{"add":["DEV-1","DEV-2"]}`},
		{name: "remove only", remove: []string{"DEV-3"}, want: `{"remove":["DEV-3"]}`},
		{
			name:   "both",
			add:    []string{"DEV-1"},
			remove: []string{"DEV-2"},
			want:   `{"add":["DEV-1"],"remove":["DEV-2"]}`,
		},
		{name: "keys are trimmed", add: []string{"  DEV-1 "}, want: `{"add":["DEV-1"]}`},
		{name: "blank entries are dropped", add: []string{"DEV-1", "", "  "}, want: `{"add":["DEV-1"]}`},
		{name: "nothing to do", wantErr: true},
		{name: "only blanks", add: []string{" "}, remove: []string{""}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := xrayMembershipPayload(tt.add, tt.remove)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("xrayMembershipPayload(%v, %v) = %s, want an error", tt.add, tt.remove, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("xrayMembershipPayload(%v, %v) error: %v", tt.add, tt.remove, err)
			}
			if got != tt.want {
				t.Errorf("xrayMembershipPayload(%v, %v) = %s, want %s", tt.add, tt.remove, got, tt.want)
			}
		})
	}
}

func TestXrayPageQuery(t *testing.T) {
	tests := []struct {
		name  string
		limit int
		page  int
		want  string
	}{
		{name: "unset", want: ""},
		{name: "limit only", limit: 50, want: "?limit=50"},
		{name: "page only", page: 2, want: "?page=2"},
		{name: "both", limit: 50, page: 2, want: "?limit=50&page=2"},
		{name: "zero and negative are unset", limit: 0, page: -1, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := xrayPageQuery(tt.limit, tt.page); got != tt.want {
				t.Errorf("xrayPageQuery(%d, %d) = %q, want %q", tt.limit, tt.page, got, tt.want)
			}
		})
	}
}

func TestXrayTestStepPayload(t *testing.T) {
	tests := []struct {
		name     string
		action   string
		data     string
		expected string
		create   bool
		want     string
		wantErr  bool
	}{
		{
			name:   "create sends all three fields",
			action: "Open login", data: "user/pass", expected: "Dashboard shown", create: true,
			want: `{"data":"user/pass","result":"Dashboard shown","step":"Open login"}`,
		},
		{
			name:   "create keeps empty data and result",
			action: "Open login", create: true,
			want: `{"data":"","result":"","step":"Open login"}`,
		},
		{name: "create needs an action", data: "x", create: true, wantErr: true},
		{name: "create rejects a blank action", action: "   ", create: true, wantErr: true},
		{
			name:   "update sends only what was supplied",
			action: "Open login",
			want:   `{"step":"Open login"}`,
		},
		{
			name:     "update can change the expected result alone",
			expected: "Dashboard shown",
			want:     `{"result":"Dashboard shown"}`,
		},
		{name: "update needs at least one field", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := xrayTestStepPayload(tt.action, tt.data, tt.expected, tt.create)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("xrayTestStepPayload(...) = %s, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("xrayTestStepPayload(...) error: %v", err)
			}
			if got != tt.want {
				t.Errorf("xrayTestStepPayload(...) = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestXrayValidateExecutionImport(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{
			name: "new execution from info",
			raw:  `{"info":{"summary":"nightly"},"tests":[{"testKey":"DEV-42","status":"PASS"}]}`,
		},
		{
			name: "existing execution by key",
			raw:  `{"testExecutionKey":"DEV-100","tests":[{"testKey":"DEV-42","status":"FAIL"}]}`,
		},
		{name: "empty", raw: "   ", wantErr: true},
		{name: "not JSON", raw: "PASS", wantErr: true},
		{name: "a Cucumber report is a bare array", raw: `[{"keyword":"Feature"}]`, wantErr: true},
		{name: "no tests", raw: `{"info":{"summary":"nightly"}}`, wantErr: true},
		{name: "empty tests", raw: `{"info":{"summary":"nightly"},"tests":[]}`, wantErr: true},
		{name: "tests but no target", raw: `{"tests":[{"testKey":"DEV-42","status":"PASS"}]}`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := xrayValidateExecutionImport(tt.raw)
			if tt.wantErr && err == nil {
				t.Fatalf("xrayValidateExecutionImport(%q) = nil, want an error", tt.raw)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("xrayValidateExecutionImport(%q) error: %v", tt.raw, err)
			}
		})
	}

	// A payload that names no Test Execution should say how to name one, since
	// the import endpoint itself fails with a bare 500.
	err := xrayValidateExecutionImport(`{"tests":[{"testKey":"DEV-42","status":"PASS"}]}`)
	if err == nil || !strings.Contains(err.Error(), "testExecutionKey") {
		t.Errorf("xrayValidateExecutionImport error = %v, want it to mention testExecutionKey", err)
	}
}
