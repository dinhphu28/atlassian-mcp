package jira

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeFields(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: ""},
		{name: "single field", input: "summary", want: "summary"},
		{name: "spaces are trimmed", input: "summary, status , assignee", want: "summary,status,assignee"},
		{name: "trailing comma is dropped", input: "summary,status,", want: "summary,status"},
		{name: "only separators", input: " , , ", want: ""},
		{name: "negation is passed through", input: "*navigable,-comment", want: "*navigable,-comment"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeFields(tt.input); got != tt.want {
				t.Errorf("normalizeFields(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCommentVisibility(t *testing.T) {
	tests := []struct {
		name      string
		vType     string
		vValue    string
		wantNil   bool
		wantType  string
		wantValue string
		wantErr   bool
	}{
		{name: "both empty is public", wantNil: true},
		{name: "role", vType: "role", vValue: "Administrators", wantType: "role", wantValue: "Administrators"},
		{name: "group", vType: "group", vValue: "jira-developers", wantType: "group", wantValue: "jira-developers"},
		{name: "type is case insensitive", vType: "Role", vValue: "Developers", wantType: "role", wantValue: "Developers"},
		{name: "surrounding space is ignored", vType: " group ", vValue: " devs ", wantType: "group", wantValue: "devs"},
		{name: "unknown type", vType: "user", vValue: "alice", wantErr: true},
		{name: "type without value", vType: "role", wantErr: true},
		{name: "value without type", vValue: "Administrators", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := commentVisibility(tt.vType, tt.vValue)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("commentVisibility(%q, %q) = %v, want an error", tt.vType, tt.vValue, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("commentVisibility(%q, %q) error: %v", tt.vType, tt.vValue, err)
			}
			if tt.wantNil {
				if got != nil {
					t.Fatalf("commentVisibility(%q, %q) = %v, want nil", tt.vType, tt.vValue, got)
				}
				return
			}
			if got["type"] != tt.wantType || got["value"] != tt.wantValue {
				t.Errorf("commentVisibility(%q, %q) = %v, want type %q value %q",
					tt.vType, tt.vValue, got, tt.wantType, tt.wantValue)
			}
		})
	}
}

func TestTransitionPayload(t *testing.T) {
	tests := []struct {
		name       string
		resolution string
		comment    string
		fields     string
		want       string
		wantErr    bool
	}{
		{
			name: "id only keeps the legacy payload",
			want: `{"transition":{"id":"31"}}`,
		},
		{
			name:       "resolution",
			resolution: "Done",
			want:       `{"fields":{"resolution":{"name":"Done"}},"transition":{"id":"31"}}`,
		},
		{
			name:    "comment travels in update",
			comment: "Shipped in *1.4*",
			want:    `{"transition":{"id":"31"},"update":{"comment":[{"add":{"body":"Shipped in *1.4*"}}]}}`,
		},
		{
			name:   "extra fields",
			fields: `{"customfield_10001":"value"}`,
			want:   `{"fields":{"customfield_10001":"value"},"transition":{"id":"31"}}`,
		},
		{
			name:       "resolution merges into extra fields",
			resolution: "Won't Do",
			fields:     `{"customfield_10001":"value"}`,
			want:       `{"fields":{"customfield_10001":"value","resolution":{"name":"Won't Do"}},"transition":{"id":"31"}}`,
		},
		{
			name:       "everything at once",
			resolution: "Done",
			comment:    "done",
			fields:     `{"assignee":{"name":"alice"}}`,
			want: `{"fields":{"assignee":{"name":"alice"},"resolution":{"name":"Done"}},` +
				`"transition":{"id":"31"},"update":{"comment":[{"add":{"body":"done"}}]}}`,
		},
		{
			name:   "blank fields are ignored",
			fields: "   ",
			want:   `{"transition":{"id":"31"}}`,
		},
		{name: "fields must be valid JSON", fields: `{not json}`, wantErr: true},
		{name: "fields must be an object", fields: `["resolution"]`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := transitionPayload("31", tt.resolution, tt.comment, tt.fields)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("transitionPayload(fields=%q) = %v, want an error", tt.fields, payload)
				}
				return
			}
			if err != nil {
				t.Fatalf("transitionPayload error: %v", err)
			}

			// Compare the encoded body, since that is what Jira sees and
			// json.Marshal orders map keys deterministically.
			got, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("marshal payload: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("transitionPayload body = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestMatchResolution(t *testing.T) {
	catalog := []resolution{
		{ID: "1", Name: "Fixed"},
		{ID: "10000", Name: "Done"},
		{ID: "10001", Name: "Won't Do"},
	}

	tests := []struct {
		name    string
		input   string
		wantID  string
		wantErr bool
	}{
		{name: "exact name", input: "Done", wantID: "10000"},
		{name: "name is case insensitive", input: "fIxEd", wantID: "1"},
		{name: "surrounding space is ignored", input: "  Won't Do  ", wantID: "10001"},
		{name: "numeric id", input: "10001", wantID: "10001"},
		{name: "unknown name", input: "Resolved", wantErr: true},
		{name: "unknown id", input: "99", wantErr: true},
		{name: "empty", input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := matchResolution(catalog, tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("matchResolution(%q) = %+v, want an error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("matchResolution(%q) error: %v", tt.input, err)
			}
			if got.ID != tt.wantID {
				t.Errorf("matchResolution(%q) = id %s, want id %s", tt.input, got.ID, tt.wantID)
			}
		})
	}

	// The valid resolutions are per-instance, so an unknown one must name them.
	_, err := matchResolution(catalog, "Resolved")
	if err == nil || !strings.Contains(err.Error(), "Done (id 10000)") {
		t.Errorf("matchResolution error = %v, want it to list the configured resolutions", err)
	}

	if _, err := matchResolution(nil, "Done"); err == nil {
		t.Error("matchResolution(nil, \"Done\") = nil error, want an error")
	}
}
