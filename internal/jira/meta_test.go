package jira

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestCompactProjects(t *testing.T) {
	raw := `[
		{"id":"10000","key":"DEV","name":"Development","projectTypeKey":"software",
		 "avatarUrls":{"48x48":"https://jira.example/secure/projectavatar?size=large&pid=10000"},
		 "expand":"description,lead,url,projectKeys"},
		{"id":"10001","key":"OPS","name":"Operations","projectTypeKey":"service_desk"}
	]`

	got, err := compactProjects(raw)
	if err != nil {
		t.Fatalf("compactProjects error: %v", err)
	}

	want := []projectSummary{
		{ID: "10000", Key: "DEV", Name: "Development", ProjectTypeKey: "software"},
		{ID: "10001", Key: "OPS", Name: "Operations", ProjectTypeKey: "service_desk"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("compactProjects() = %+v, want %+v", got, want)
	}

	if _, err := compactProjects("not json"); err == nil {
		t.Error("compactProjects(\"not json\") = nil error, want an error")
	}
}

func TestCappedAllowedValues(t *testing.T) {
	many := make([]map[string]any, 0, maxAllowedValues+5)
	for i := 0; i < maxAllowedValues+5; i++ {
		many = append(many, map[string]any{"name": fmt.Sprintf("v%d", i)})
	}

	tests := []struct {
		name       string
		values     []map[string]any
		wantLabels []string
		wantNote   bool
	}{
		{name: "none", values: nil},
		{
			name:       "labels come from the key Jira used",
			values:     []map[string]any{{"name": "Backend"}, {"value": "Urgent"}, {"id": "10100"}},
			wantLabels: []string{"Backend", "Urgent", "10100"},
		},
		{
			name:       "unlabelled values are dropped",
			values:     []map[string]any{{"name": "Backend"}, {"self": "https://jira.example/x"}},
			wantLabels: []string{"Backend"},
		},
		{
			name:       "long lists are capped with a note",
			values:     many,
			wantLabels: nil,
			wantNote:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			labels, note := cappedAllowedValues(tt.values)

			if tt.wantNote {
				if len(labels) != maxAllowedValues {
					t.Errorf("cappedAllowedValues() kept %d labels, want %d", len(labels), maxAllowedValues)
				}
				if !strings.Contains(note, fmt.Sprintf("%d of %d", maxAllowedValues, len(tt.values))) {
					t.Errorf("cappedAllowedValues() note = %q, want it to count the values", note)
				}
				return
			}

			if note != "" {
				t.Errorf("cappedAllowedValues() note = %q, want none", note)
			}
			if !reflect.DeepEqual(labels, tt.wantLabels) {
				t.Errorf("cappedAllowedValues() = %v, want %v", labels, tt.wantLabels)
			}
		})
	}
}

func TestCompactCreateFields(t *testing.T) {
	fields := map[string]createMetaField{
		"summary":     {Name: "Summary", Required: true},
		"description": {Name: "Description"},
		"customfield_10010": {
			Name:          "Team",
			Required:      true,
			AllowedValues: []map[string]any{{"value": "Payments"}, {"value": "Search"}},
		},
		"assignee": {Name: "Assignee"},
	}
	fields["customfield_10010"] = withSchema(fields["customfield_10010"], "array", "option")

	required, optional := compactCreateFields(fields)

	wantRequired := []CreateMetaField{
		{ID: "customfield_10010", Name: "Team", Type: "array of option", AllowedValues: []string{"Payments", "Search"}},
		{ID: "summary", Name: "Summary"},
	}
	if !reflect.DeepEqual(required, wantRequired) {
		t.Errorf("compactCreateFields() required = %+v, want %+v", required, wantRequired)
	}

	// Both lists are sorted by id, so the projection is stable across calls
	// even though Jira's fields arrive in a map.
	wantOptional := []string{"assignee", "description"}
	if !reflect.DeepEqual(optional, wantOptional) {
		t.Errorf("compactCreateFields() optional = %v, want %v", optional, wantOptional)
	}
}

// withSchema sets a field's schema, which is an anonymous struct and cannot be
// written in a composite literal.
func withSchema(f createMetaField, schemaType, items string) createMetaField {
	f.Schema.Type = schemaType
	f.Schema.Items = items
	return f
}

func TestCreateFieldsByID(t *testing.T) {
	got := createFieldsByID([]createMetaField{
		{FieldID: "summary", Name: "Summary", Required: true},
		{Name: "a field with no id"},
		{FieldID: "labels", Name: "Labels"},
	})

	if len(got) != 2 {
		t.Fatalf("createFieldsByID() = %d fields, want 2 (the entry without a fieldId is dropped)", len(got))
	}
	if got["summary"].Name != "Summary" || !got["summary"].Required {
		t.Errorf("createFieldsByID()[\"summary\"] = %+v, want the required Summary field", got["summary"])
	}
	if _, ok := got["labels"]; !ok {
		t.Error("createFieldsByID() lost the labels field")
	}
}

func TestCompactCreateMeta(t *testing.T) {
	raw := `{"expand":"projects","projects":[{
		"id":"10000","key":"DEV","name":"Development",
		"issuetypes":[
			{"id":"10001","name":"Task","subtask":false,"fields":{
				"summary":{"name":"Summary","required":true,"schema":{"type":"string"}},
				"description":{"name":"Description","required":false,"schema":{"type":"string"}},
				"issuetype":{"name":"Issue Type","required":true,"schema":{"type":"issuetype"},
					"allowedValues":[{"id":"10001","name":"Task"}]}
			}},
			{"id":"10003","name":"Sub-task","subtask":true,"fields":{
				"parent":{"name":"Parent","required":true,"schema":{"type":"any"}}
			}}
		]
	}]}`

	meta, err := compactCreateMeta(raw)
	if err != nil {
		t.Fatalf("compactCreateMeta error: %v", err)
	}

	if meta.ProjectKey != "DEV" || meta.ProjectName != "Development" {
		t.Errorf("compactCreateMeta() project = %s/%s, want DEV/Development", meta.ProjectKey, meta.ProjectName)
	}
	if len(meta.IssueTypes) != 2 {
		t.Fatalf("compactCreateMeta() = %d issue types, want 2", len(meta.IssueTypes))
	}

	task := meta.IssueTypes[0]
	if task.Name != "Task" || task.Subtask {
		t.Errorf("compactCreateMeta() first issue type = %+v, want the non-subtask Task", task)
	}
	wantRequired := []CreateMetaField{
		{ID: "issuetype", Name: "Issue Type", Type: "issuetype", AllowedValues: []string{"Task"}},
		{ID: "summary", Name: "Summary", Type: "string"},
	}
	if !reflect.DeepEqual(task.RequiredFields, wantRequired) {
		t.Errorf("compactCreateMeta() required fields = %+v, want %+v", task.RequiredFields, wantRequired)
	}
	if !reflect.DeepEqual(task.OptionalFieldIDs, []string{"description"}) {
		t.Errorf("compactCreateMeta() optional fields = %v, want [description]", task.OptionalFieldIDs)
	}
	if !meta.IssueTypes[1].Subtask {
		t.Error("compactCreateMeta() lost the subtask flag of Sub-task")
	}

	if _, err := compactCreateMeta(`{"projects":[]}`); err == nil {
		t.Error("compactCreateMeta with no project = nil error, want an error")
	}
	if _, err := compactCreateMeta("<html>login</html>"); err == nil {
		t.Error("compactCreateMeta of a non-JSON body = nil error, want an error")
	}
}

func TestParseCreateMetaIssueTypes(t *testing.T) {
	raw := `{"maxResults":50,"startAt":0,"total":2,"isLast":true,"values":[
		{"id":"10001","name":"Task","subtask":false,"description":"A task."},
		{"id":"10003","name":"Sub-task","subtask":true}
	]}`

	got, err := parseCreateMetaIssueTypes(raw)
	if err != nil {
		t.Fatalf("parseCreateMetaIssueTypes error: %v", err)
	}

	want := []CreateMetaIssueType{
		{ID: "10001", Name: "Task"},
		{ID: "10003", Name: "Sub-task", Subtask: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseCreateMetaIssueTypes() = %+v, want %+v", got, want)
	}

	if _, err := parseCreateMetaIssueTypes("nope"); err == nil {
		t.Error("parseCreateMetaIssueTypes(\"nope\") = nil error, want an error")
	}
}

func TestParseCreateMetaFields(t *testing.T) {
	// The per-issue-type endpoint carries the field id in "fieldId" instead of
	// keying the fields by it.
	raw := `{"maxResults":50,"startAt":0,"total":3,"isLast":true,"values":[
		{"fieldId":"summary","name":"Summary","required":true,"schema":{"type":"string"}},
		{"fieldId":"components","name":"Components","required":true,
		 "schema":{"type":"array","items":"component"},
		 "allowedValues":[{"id":"1","name":"Backend"}]},
		{"fieldId":"labels","name":"Labels","required":false,"schema":{"type":"array","items":"string"}}
	]}`

	required, optional, err := parseCreateMetaFields(raw)
	if err != nil {
		t.Fatalf("parseCreateMetaFields error: %v", err)
	}

	wantRequired := []CreateMetaField{
		{ID: "components", Name: "Components", Type: "array of component", AllowedValues: []string{"Backend"}},
		{ID: "summary", Name: "Summary", Type: "string"},
	}
	if !reflect.DeepEqual(required, wantRequired) {
		t.Errorf("parseCreateMetaFields() required = %+v, want %+v", required, wantRequired)
	}
	if !reflect.DeepEqual(optional, []string{"labels"}) {
		t.Errorf("parseCreateMetaFields() optional = %v, want [labels]", optional)
	}

	if _, _, err := parseCreateMetaFields("{"); err == nil {
		t.Error("parseCreateMetaFields(\"{\") = nil error, want an error")
	}
}

func TestMatchCreateMetaIssueType(t *testing.T) {
	types := []CreateMetaIssueType{
		{ID: "10001", Name: "Task"},
		{ID: "10002", Name: "Bug"},
		{ID: "10003", Name: "Sub-task", Subtask: true},
	}

	tests := []struct {
		name    string
		input   string
		wantID  string
		wantErr bool
	}{
		{name: "exact name", input: "Bug", wantID: "10002"},
		{name: "name is case insensitive", input: "sub-TASK", wantID: "10003"},
		{name: "surrounding space is ignored", input: "  Task  ", wantID: "10001"},
		{name: "numeric id", input: "10002", wantID: "10002"},
		{name: "unknown name", input: "Epic", wantErr: true},
		{name: "unknown id", input: "99999", wantErr: true},
		{name: "empty", input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := matchCreateMetaIssueType(types, tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("matchCreateMetaIssueType(%q) = %+v, want an error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("matchCreateMetaIssueType(%q) error: %v", tt.input, err)
			}
			if got.ID != tt.wantID {
				t.Errorf("matchCreateMetaIssueType(%q) = id %s, want id %s", tt.input, got.ID, tt.wantID)
			}
		})
	}

	// An unknown type should name the types the project does accept, since the
	// set is per-project and the caller cannot guess it.
	_, err := matchCreateMetaIssueType(types, "Epic")
	if err == nil || !strings.Contains(err.Error(), "Task (id 10001)") {
		t.Errorf("matchCreateMetaIssueType error = %v, want it to list the project's issue types", err)
	}

	if _, err := matchCreateMetaIssueType(nil, "Task"); err == nil {
		t.Error("matchCreateMetaIssueType(nil, \"Task\") = nil error, want an error")
	}
}

func TestVersionPayload(t *testing.T) {
	tests := []struct {
		name        string
		projectKey  string
		version     string
		description string
		startDate   string
		releaseDate string
		released    bool
		want        map[string]any
		wantErr     bool
	}{
		{
			name:       "name and project only",
			projectKey: "DEV",
			version:    "1.4.0",
			want:       map[string]any{"project": "DEV", "name": "1.4.0", "released": false},
		},
		{
			name:        "every field",
			projectKey:  " DEV ",
			version:     " 1.4.0 ",
			description: "Q3 release",
			startDate:   "2026-01-02",
			releaseDate: "2026-03-31",
			released:    true,
			want: map[string]any{
				"project":     "DEV",
				"name":        "1.4.0",
				"released":    true,
				"description": "Q3 release",
				"startDate":   "2026-01-02",
				"releaseDate": "2026-03-31",
			},
		},
		{name: "no project", version: "1.4.0", wantErr: true},
		{name: "no name", projectKey: "DEV", wantErr: true},
		{name: "blank name", projectKey: "DEV", version: "   ", wantErr: true},
		{name: "release date is not a date", projectKey: "DEV", version: "1.4.0", releaseDate: "31/03/2026", wantErr: true},
		{name: "start date carries a time", projectKey: "DEV", version: "1.4.0", startDate: "2026-01-02T00:00:00", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := versionPayload(tt.projectKey, tt.version, tt.description, tt.startDate, tt.releaseDate, tt.released)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("versionPayload() = %v, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("versionPayload() error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("versionPayload() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCreateMetaQuery(t *testing.T) {
	tests := []struct {
		name         string
		issueType    string
		expandFields bool
		want         string
	}{
		{name: "no filter", want: "projectKeys=DEV"},
		{name: "expanded", expandFields: true, want: "expand=projects.issuetypes.fields&projectKeys=DEV"},
		{name: "a name filters by name", issueType: "Task", want: "issuetypeNames=Task&projectKeys=DEV"},
		// A numeric id sent as issuetypeNames matches nothing, which surfaces as
		// "no create screen" for a perfectly valid issue type.
		{name: "an id filters by id", issueType: "10001", want: "issuetypeIds=10001&projectKeys=DEV"},
		{name: "surrounding space is ignored", issueType: "  10001  ", want: "issuetypeIds=10001&projectKeys=DEV"},
		{name: "a name that starts with digits is still a name", issueType: "10001 Bug",
			want: "issuetypeNames=10001+Bug&projectKeys=DEV"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := createMetaQuery("DEV", tt.issueType, tt.expandFields).Encode()
			if got != tt.want {
				t.Errorf("createMetaQuery(DEV, %q, %v) = %q, want %q", tt.issueType, tt.expandFields, got, tt.want)
			}
		})
	}
}
