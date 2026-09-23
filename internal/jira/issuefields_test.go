package jira

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestSplitFieldList(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "empty", input: "", want: nil},
		{name: "only separators", input: " , , ", want: nil},
		{name: "single entry", input: "backend", want: []string{"backend"}},
		{name: "spaces are trimmed", input: " backend , api ", want: []string{"backend", "api"}},
		{name: "empty entries are dropped", input: "a,,b", want: []string{"a", "b"}},
		{name: "inner spaces are kept", input: "Fix Version 1, 2.0", want: []string{"Fix Version 1", "2.0"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := splitFieldList(tt.input); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitFieldList(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestNameRefs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []map[string]any
	}{
		{name: "empty stays nil so the field is omitted", input: "", want: nil},
		{name: "only separators stays nil", input: " , ", want: nil},
		{name: "single entry", input: "Backend", want: []map[string]any{{"name": "Backend"}}},
		{
			name:  "spaces around entries are trimmed",
			input: " Backend , Frontend ",
			want:  []map[string]any{{"name": "Backend"}, {"name": "Frontend"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nameRefs(tt.input); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("nameRefs(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestLabelOps(t *testing.T) {
	tests := []struct {
		name   string
		add    string
		remove string
		want   []map[string]any
	}{
		{name: "nothing yields no ops", want: nil},
		{name: "single add", add: "alpha", want: []map[string]any{{"add": "alpha"}}},
		{name: "single remove", remove: "beta", want: []map[string]any{{"remove": "beta"}}},
		{
			name:   "adds precede removes",
			add:    " alpha , gamma ",
			remove: "beta",
			want: []map[string]any{
				{"add": "alpha"}, {"add": "gamma"}, {"remove": "beta"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := labelOps(tt.add, tt.remove); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("labelOps(%q, %q) = %v, want %v", tt.add, tt.remove, got, tt.want)
			}
		})
	}
}

func TestBuildFieldPayload(t *testing.T) {
	tests := []struct {
		name        string
		edits       FieldEdits
		wantJSON    string
		wantTouched []string
		wantErr     string
	}{
		{
			name:    "nothing supplied",
			edits:   FieldEdits{},
			wantErr: "nothing to set",
		},
		{
			name:        "labels replace the whole set via fields",
			edits:       FieldEdits{Labels: " alpha , beta "},
			wantJSON:    `{"fields":{"labels":["alpha","beta"]}}`,
			wantTouched: []string{"labels"},
		},
		{
			name:        "label add and remove go in update",
			edits:       FieldEdits{LabelsAdd: "alpha", LabelsRemove: "beta"},
			wantJSON:    `{"update":{"labels":[{"add":"alpha"},{"remove":"beta"}]}}`,
			wantTouched: []string{"labels"},
		},
		{
			name:        "due date",
			edits:       FieldEdits{DueDate: "2026-01-31"},
			wantJSON:    `{"fields":{"duedate":"2026-01-31"}}`,
			wantTouched: []string{"duedate"},
		},
		{
			name:    "malformed due date",
			edits:   FieldEdits{DueDate: "31/01/2026"},
			wantErr: "yyyy-MM-dd",
		},
		{
			name:  "components and versions become name references",
			edits: FieldEdits{Components: "Backend", FixVersions: "2.0, 2.1", AffectsVersions: "1.9"},
			wantJSON: `{"fields":{"components":[{"name":"Backend"}],` +
				`"fixVersions":[{"name":"2.0"},{"name":"2.1"}],"versions":[{"name":"1.9"}]}}`,
			wantTouched: []string{"components", "fixVersions", "versions"},
		},
		{
			// An empty list parameter must vanish from the payload: sending []
			// would clear the field instead of leaving it alone.
			name:        "empty list parameters are omitted, not sent empty",
			edits:       FieldEdits{Components: "  ", FixVersions: ",", Reporter: "jdoe"},
			wantJSON:    `{"fields":{"reporter":{"name":"jdoe"}}}`,
			wantTouched: []string{"reporter"},
		},
		{
			// Jira rewrites the estimate a write omits, so the issue's current
			// original estimate has to ride along with the new remaining one.
			name: "the estimate that is not being changed is resent as it stands",
			edits: FieldEdits{
				RemainingEstimate:        "1d",
				currentOriginalEstimate:  "5d",
				currentRemainingEstimate: "2d",
			},
			wantJSON: `{"fields":{"timetracking":` +
				`{"originalEstimate":"5d","remainingEstimate":"1d"}}}`,
			wantTouched: []string{"timetracking"},
		},
		{
			name:        "an unset sibling estimate is simply absent",
			edits:       FieldEdits{RemainingEstimate: "2d 4h"},
			wantJSON:    `{"fields":{"timetracking":{"remainingEstimate":"2d 4h"}}}`,
			wantTouched: []string{"timetracking"},
		},
		{
			name:  "both estimates share one timetracking field",
			edits: FieldEdits{OriginalEstimate: "3d", RemainingEstimate: "1d"},
			wantJSON: `{"fields":{"timetracking":` +
				`{"originalEstimate":"3d","remainingEstimate":"1d"}}}`,
			wantTouched: []string{"timetracking"},
		},
		{
			name:        "raw json merges into both sections",
			edits:       FieldEdits{FieldsJSON: `{"customfield_10001":5}`, UpdateJSON: `{"customfield_10002":[{"add":"x"}]}`},
			wantJSON:    `{"fields":{"customfield_10001":5},"update":{"customfield_10002":[{"add":"x"}]}}`,
			wantTouched: []string{"customfield_10001", "customfield_10002"},
		},
		{
			name:    "fields_json must be an object",
			edits:   FieldEdits{FieldsJSON: `[1,2]`},
			wantErr: "fields_json must be a JSON object",
		},
		{
			name:    "update_json must be an object",
			edits:   FieldEdits{UpdateJSON: `"labels"`},
			wantErr: "update_json must be a JSON object",
		},
		{
			name:    "raw json cannot restate a named parameter",
			edits:   FieldEdits{Components: "Backend", FieldsJSON: `{"components":[]}`},
			wantErr: `fields_json sets "components"`,
		},
		{
			name:    "one field cannot be in both sections",
			edits:   FieldEdits{Labels: "alpha", LabelsAdd: "beta"},
			wantErr: "cannot be set in both",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, touched, err := buildFieldPayload(tt.edits)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("buildFieldPayload(%+v) = %v, want error containing %q", tt.edits, payload, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("buildFieldPayload error = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("buildFieldPayload(%+v) error: %v", tt.edits, err)
			}

			got, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("cannot marshal payload: %v", err)
			}
			if string(got) != tt.wantJSON {
				t.Errorf("buildFieldPayload payload = %s, want %s", got, tt.wantJSON)
			}
			if !reflect.DeepEqual(touched, tt.wantTouched) {
				t.Errorf("buildFieldPayload touched = %v, want %v", touched, tt.wantTouched)
			}
		})
	}
}

func TestEditFieldType(t *testing.T) {
	tests := []struct {
		name       string
		schemaType string
		items      string
		want       string
	}{
		{name: "scalar", schemaType: "string", want: "string"},
		{name: "array names its element type", schemaType: "array", items: "version", want: "array of version"},
		{name: "array without items", schemaType: "array", want: "array"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := editFieldType(tt.schemaType, tt.items); got != tt.want {
				t.Errorf("editFieldType(%q, %q) = %q, want %q", tt.schemaType, tt.items, got, tt.want)
			}
		})
	}
}

func TestAllowedValueLabel(t *testing.T) {
	tests := []struct {
		name  string
		value map[string]any
		want  string
	}{
		{name: "version uses name", value: map[string]any{"id": "101", "name": "2.0"}, want: "2.0"},
		{name: "select option uses value", value: map[string]any{"id": "102", "value": "Red"}, want: "Red"},
		{name: "falls back to id", value: map[string]any{"id": "103"}, want: "103"},
		{name: "nothing usable", value: map[string]any{"self": 1}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := allowedValueLabel(tt.value); got != tt.want {
				t.Errorf("allowedValueLabel(%v) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestCompactEditFields(t *testing.T) {
	raw := `{"fields":{
		"summary":{"required":true,"name":"Summary","operations":["set"],
			"schema":{"type":"string"}},
		"fixVersions":{"required":false,"name":"Fix Version/s","operations":["set","add","remove"],
			"schema":{"type":"array","items":"version"},
			"allowedValues":[{"id":"101","name":"2.0"},{"id":"102","name":"2.1"}]},
		"customfield_10001":{"required":false,"name":"Story Points","operations":["set"],
			"schema":{"type":"number","custom":"com.atlassian.jira.plugin.system.customfieldtypes:float"}}
	}}`

	got, err := compactEditFields(raw)
	if err != nil {
		t.Fatalf("compactEditFields error: %v", err)
	}

	want := []EditField{
		{ID: "customfield_10001", Name: "Story Points", Type: "number", Operations: []string{"set"}},
		{
			ID: "fixVersions", Name: "Fix Version/s", Type: "array of version",
			Operations: []string{"set", "add", "remove"}, AllowedValues: []string{"2.0", "2.1"},
		},
		{ID: "summary", Name: "Summary", Required: true, Type: "string", Operations: []string{"set"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("compactEditFields = %+v, want %+v", got, want)
	}

	if _, err := compactEditFields("not json"); err == nil {
		t.Error("compactEditFields(\"not json\") = nil error, want an error")
	}
}

func TestCompactEditFieldsCapsAllowedValues(t *testing.T) {
	var values []string
	for i := 0; i < maxAllowedValues+7; i++ {
		values = append(values, `{"value":"opt`+string(rune('a'+i%26))+`"}`)
	}
	raw := `{"fields":{"customfield_10010":{"name":"Team","schema":{"type":"option"},` +
		`"allowedValues":[` + strings.Join(values, ",") + `]}}}`

	got, err := compactEditFields(raw)
	if err != nil {
		t.Fatalf("compactEditFields error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("compactEditFields returned %d fields, want 1", len(got))
	}
	if n := len(got[0].AllowedValues); n != maxAllowedValues {
		t.Errorf("allowed values = %d, want the cap of %d", n, maxAllowedValues)
	}
	if !strings.Contains(got[0].AllowedValuesNote, "of 27") {
		t.Errorf("allowed_values_note = %q, want it to report the full count", got[0].AllowedValuesNote)
	}
}
