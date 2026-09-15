package jira

import (
	"testing"
	"time"
)

func TestNormalizeStarted(t *testing.T) {
	local := func(y int, mo time.Month, d, h, mi, s int) string {
		return time.Date(y, mo, d, h, mi, s, 0, time.Local).Format(jiraTimeFormat)
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty stays empty", "", ""},
		{"date only", "2026-09-15", local(2026, 9, 15, 0, 0, 0)},
		{"date and time", "2026-09-15 09:30", local(2026, 9, 15, 9, 30, 0)},
		{"date and time with seconds", "2026-09-15T09:30:15", local(2026, 9, 15, 9, 30, 15)},
		{"rfc3339 keeps its offset", "2026-09-15T09:30:00+07:00", "2026-09-15T09:30:00.000+0700"},
		{"already jira format", "2026-09-15T09:30:00.000+0700", "2026-09-15T09:30:00.000+0700"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeStarted(tt.input)
			if err != nil {
				t.Fatalf("normalizeStarted(%q) error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("normalizeStarted(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}

	if _, err := normalizeStarted("yesterday"); err == nil {
		t.Error("normalizeStarted(\"yesterday\") = nil error, want a parse error")
	}
}

func TestWorklogQuery(t *testing.T) {
	tests := []struct {
		name        string
		adjust      string
		value       string
		manualParam string
		want        string
		wantErr     bool
	}{
		{name: "empty means jira default", want: ""},
		{name: "leave", adjust: "leave", want: "?adjustEstimate=leave"},
		{name: "case insensitive", adjust: "AUTO", want: "?adjustEstimate=auto"},
		{name: "new carries the estimate", adjust: "new", value: "2d", want: "?adjustEstimate=new&newEstimate=2d"},
		{name: "new without a value", adjust: "new", wantErr: true},
		{
			name: "manual reduces", adjust: "manual", value: "1h", manualParam: "reduceBy",
			want: "?adjustEstimate=manual&reduceBy=1h",
		},
		{name: "manual without a value", adjust: "manual", manualParam: "reduceBy", wantErr: true},
		{name: "manual unsupported", adjust: "manual", value: "1h", wantErr: true},
		{name: "unknown value", adjust: "sometimes", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := worklogQuery(tt.adjust, tt.value, tt.manualParam)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("worklogQuery(%q, %q, %q) = %q, want an error", tt.adjust, tt.value, tt.manualParam, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("worklogQuery(%q, %q, %q) error: %v", tt.adjust, tt.value, tt.manualParam, err)
			}
			if got != tt.want {
				t.Errorf("worklogQuery(%q, %q, %q) = %q, want %q", tt.adjust, tt.value, tt.manualParam, got, tt.want)
			}
		})
	}
}
