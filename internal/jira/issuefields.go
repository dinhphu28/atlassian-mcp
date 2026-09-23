package jira

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// maxAllowedValues caps how many allowed values the compact editmeta view
// reports per field. Fields like "Fix Version/s" or a big select list can carry
// hundreds, which is what makes the raw response unusable in a tool result.
const maxAllowedValues = 20

// FieldEdits is everything SetFields can change on an issue. Every member is
// the raw string the caller supplied; empty members are left untouched, which
// is deliberate: sending an empty array or null would clear the field instead.
type FieldEdits struct {
	LabelsAdd       string
	LabelsRemove    string
	Labels          string
	DueDate         string
	Components      string
	FixVersions     string
	AffectsVersions string
	Reporter        string

	OriginalEstimate  string
	RemainingEstimate string

	FieldsJSON string
	UpdateJSON string

	// currentOriginalEstimate and currentRemainingEstimate are the issue's
	// stored time tracking, read by SetFields rather than supplied by a caller:
	// Jira rewrites the half of the timetracking field that a write leaves out
	// (JRASERVER-30459), so both halves have to be sent whenever either changes.
	currentOriginalEstimate  string
	currentRemainingEstimate string
}

// splitFieldList splits a comma-separated list, trimming each entry and dropping
// empty ones, so "a, b ,,c" yields [a b c] and a blank string yields nothing.
func splitFieldList(list string) []string {
	var out []string
	for _, part := range strings.Split(list, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// nameRefs turns a comma-separated list of names into the [{"name": …}] form
// Jira wants for components and versions. A list with no usable entries yields
// nil, so the caller can omit the field rather than send [] and clear it.
func nameRefs(list string) []map[string]any {
	names := splitFieldList(list)
	if len(names) == 0 {
		return nil
	}
	refs := make([]map[string]any, 0, len(names))
	for _, name := range names {
		refs = append(refs, map[string]any{"name": name})
	}
	return refs
}

// labelOps builds the "labels" entry of the update section, which Jira models
// as a list of single-key operations: [{"add":"x"},{"remove":"y"}]. Adds come
// first so a value in both lists ends up removed.
func labelOps(add, remove string) []map[string]any {
	var ops []map[string]any
	for _, label := range splitFieldList(add) {
		ops = append(ops, map[string]any{"add": label})
	}
	for _, label := range splitFieldList(remove) {
		ops = append(ops, map[string]any{"remove": label})
	}
	return ops
}

// parseJSONObjectArg decodes one of the raw JSON escape-hatch parameters. param
// names it in the error, since the two look alike and a caller who mixes them
// up otherwise gets an opaque 400 from Jira.
func parseJSONObjectArg(raw, param string) (map[string]any, error) {
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return nil, fmt.Errorf("%s must be a JSON object like {\"customfield_10001\": 5}: %w", param, err)
	}
	if obj == nil {
		return nil, fmt.Errorf("%s must be a JSON object like {\"customfield_10001\": 5}, not null", param)
	}
	return obj, nil
}

// mergeFieldSection copies src into dst, refusing to overwrite a key a named
// parameter already set: silently letting the raw JSON win would make
// components="A" plus fields_json {"components": …} do something the caller
// did not ask for.
func mergeFieldSection(dst, src map[string]any, param string) error {
	keys := make([]string, 0, len(src))
	for k := range src {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		if _, taken := dst[k]; taken {
			return fmt.Errorf("%s sets %q, which another parameter already sets; use one or the other", param, k)
		}
		dst[k] = src[k]
	}
	return nil
}

// buildFieldPayload assembles the body of a single PUT /rest/api/2/issue/{key}
// from the requested edits, and reports which fields it touched so the caller
// can confirm them. Empty sections are omitted entirely.
func buildFieldPayload(e FieldEdits) (map[string]any, []string, error) {
	fields := map[string]any{}
	update := map[string]any{}
	var touched []string

	if labels := splitFieldList(e.Labels); len(labels) > 0 {
		fields["labels"] = labels
		touched = append(touched, "labels")
	}
	if ops := labelOps(e.LabelsAdd, e.LabelsRemove); len(ops) > 0 {
		update["labels"] = ops
		touched = append(touched, "labels")
	}

	if e.DueDate != "" {
		if !dateOnly.MatchString(e.DueDate) {
			return nil, nil, fmt.Errorf("due_date must be a date in yyyy-MM-dd format, got %q", e.DueDate)
		}
		fields["duedate"] = e.DueDate
		touched = append(touched, "duedate")
	}

	for _, named := range []struct {
		id   string
		list string
	}{
		{"components", e.Components},
		{"fixVersions", e.FixVersions},
		{"versions", e.AffectsVersions},
	} {
		if refs := nameRefs(named.list); len(refs) > 0 {
			fields[named.id] = refs
			touched = append(touched, named.id)
		}
	}

	if reporter := strings.TrimSpace(e.Reporter); reporter != "" {
		fields["reporter"] = map[string]any{"name": reporter}
		touched = append(touched, "reporter")
	}

	// Jira takes both estimates through the single "timetracking" field, and
	// rewrites the half a write leaves out (JRASERVER-30459: setting only the
	// remaining estimate also overwrites the original one). So whenever either
	// estimate changes, the other is resent as it stands — SetFields reads it
	// off the issue first and puts it in currentOriginalEstimate /
	// currentRemainingEstimate.
	if e.OriginalEstimate != "" || e.RemainingEstimate != "" {
		original := firstNonEmpty(e.OriginalEstimate, e.currentOriginalEstimate)
		remaining := firstNonEmpty(e.RemainingEstimate, e.currentRemainingEstimate)

		timetracking := map[string]any{}
		if original != "" {
			timetracking["originalEstimate"] = original
		}
		if remaining != "" {
			timetracking["remainingEstimate"] = remaining
		}
		fields["timetracking"] = timetracking
		touched = append(touched, "timetracking")
	}

	if e.FieldsJSON != "" {
		raw, err := parseJSONObjectArg(e.FieldsJSON, "fields_json")
		if err != nil {
			return nil, nil, err
		}
		if err := mergeFieldSection(fields, raw, "fields_json"); err != nil {
			return nil, nil, err
		}
		touched = append(touched, sortedObjectKeys(raw)...)
	}
	if e.UpdateJSON != "" {
		raw, err := parseJSONObjectArg(e.UpdateJSON, "update_json")
		if err != nil {
			return nil, nil, err
		}
		if err := mergeFieldSection(update, raw, "update_json"); err != nil {
			return nil, nil, err
		}
		touched = append(touched, sortedObjectKeys(raw)...)
	}

	// Jira rejects the whole request when a field appears in both sections, so
	// say which field it is rather than forwarding that 400.
	for _, k := range sortedObjectKeys(fields) {
		if _, both := update[k]; both {
			return nil, nil, fmt.Errorf("field %q cannot be set in both the fields and the update section "+
				"(e.g. 'labels' replaces the set, 'labels_add'/'labels_remove' amend it)", k)
		}
	}

	if len(fields) == 0 && len(update) == 0 {
		return nil, nil, fmt.Errorf("nothing to set: supply at least one of labels, labels_add, labels_remove, " +
			"due_date, components, fix_versions, affects_versions, reporter, original_estimate, " +
			"remaining_estimate, fields_json or update_json")
	}

	payload := map[string]any{}
	if len(fields) > 0 {
		payload["fields"] = fields
	}
	if len(update) > 0 {
		payload["update"] = update
	}
	return payload, dedupeStrings(touched), nil
}

// firstNonEmpty returns the first of its arguments that is not empty, which is
// how a supplied estimate takes precedence over the one already on the issue.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// sortedObjectKeys returns m's keys in a stable order, so payload assembly and
// the errors it produces do not depend on Go's map iteration order.
func sortedObjectKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// dedupeStrings drops repeats while keeping first-seen order; "labels" can be
// touched by both the replace and the add/remove parameters.
func dedupeStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// SetFields applies arbitrary field edits to an issue in one
// PUT /rest/api/2/issue/{key}, covering both the "fields" (replace) and the
// "update" (add/remove) semantics. To clear a field, pass an explicit null or
// [] through FieldsJSON: an empty parameter means "leave alone".
func (c *Client) SetFields(key string, e FieldEdits) (string, error) {
	if key == "" {
		return "", fmt.Errorf("issue_key is required")
	}

	// Only one estimate given: read the other off the issue so the write cannot
	// clobber it. Failing here is better than silently destroying an estimate.
	if (e.OriginalEstimate == "") != (e.RemainingEstimate == "") {
		original, remaining, err := c.currentTimeTracking(key)
		if err != nil {
			return "", err
		}
		e.currentOriginalEstimate, e.currentRemainingEstimate = original, remaining
	}

	payload, touched, err := buildFieldPayload(e)
	if err != nil {
		return "", err
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	if _, err := c.do(http.MethodPut, "/rest/api/2/issue/"+url.PathEscape(key), string(body)); err != nil {
		return "", err
	}

	return fmt.Sprintf("Updated %s: %s", key, strings.Join(touched, ", ")), nil
}

// currentTimeTracking reads an issue's stored original and remaining estimates.
// They are needed whenever only one of the two is being set, because Jira
// synchronises the half that a write omits.
func (c *Client) currentTimeTracking(key string) (original, remaining string, err error) {
	raw, err := c.get("/rest/api/2/issue/" + url.PathEscape(key) + "?fields=timetracking")
	if err != nil {
		return "", "", fmt.Errorf("cannot read the current time tracking of %s, which is needed because "+
			"Jira overwrites the estimate a write leaves out; pass both original_estimate and "+
			"remaining_estimate to skip this read: %w", key, err)
	}

	var issue struct {
		Fields struct {
			TimeTracking struct {
				OriginalEstimate  string `json:"originalEstimate"`
				RemainingEstimate string `json:"remainingEstimate"`
			} `json:"timetracking"`
		} `json:"fields"`
	}
	if err := json.Unmarshal([]byte(raw), &issue); err != nil {
		return "", "", fmt.Errorf("cannot parse the time tracking of %s: %w", key, err)
	}
	return issue.Fields.TimeTracking.OriginalEstimate, issue.Fields.TimeTracking.RemainingEstimate, nil
}

// editMetaField is one entry of the "fields" map in an editmeta response. The
// map key is the field id, which is why the id is not a member here.
type editMetaField struct {
	Name       string   `json:"name"`
	Required   bool     `json:"required"`
	Operations []string `json:"operations"`
	Schema     struct {
		Type   string `json:"type"`
		Items  string `json:"items"`
		Custom string `json:"custom"`
	} `json:"schema"`
	AllowedValues []map[string]any `json:"allowedValues"`
}

// EditField is the compact description of one editable field: enough to write a
// jira_set_fields call against it, without the bulk of the raw response.
type EditField struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Required          bool     `json:"required"`
	Type              string   `json:"type"`
	Operations        []string `json:"operations,omitempty"`
	AllowedValues     []string `json:"allowed_values,omitempty"`
	AllowedValuesNote string   `json:"allowed_values_note,omitempty"`
}

// allowedValueLabel picks the human-facing label of an allowed value. Jira uses
// a different key per field kind — "name" for versions and components, "value"
// for select options, "key" for some references — and falls back to the id.
func allowedValueLabel(value map[string]any) string {
	for _, key := range []string{"name", "value", "key", "id"} {
		if s, ok := value[key].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// editFieldType renders a field's schema as one readable type, keeping the
// element type of an array ("array of version") since that is what decides the
// shape a value must have.
func editFieldType(schemaType, items string) string {
	if schemaType == "array" && items != "" {
		return "array of " + items
	}
	return schemaType
}

// compactEditFields reduces an editmeta response to one entry per field, sorted
// by field id. Allowed values are capped, with a note saying how many there are,
// because a single field can list hundreds of them.
func compactEditFields(raw string) ([]EditField, error) {
	var meta struct {
		Fields map[string]editMetaField `json:"fields"`
	}
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		return nil, fmt.Errorf("cannot parse editmeta response: %w", err)
	}

	ids := make([]string, 0, len(meta.Fields))
	for id := range meta.Fields {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := make([]EditField, 0, len(ids))
	for _, id := range ids {
		f := meta.Fields[id]
		entry := EditField{
			ID:         id,
			Name:       f.Name,
			Required:   f.Required,
			Type:       editFieldType(f.Schema.Type, f.Schema.Items),
			Operations: f.Operations,
		}

		for _, value := range f.AllowedValues {
			if len(entry.AllowedValues) == maxAllowedValues {
				entry.AllowedValuesNote = fmt.Sprintf("showing %d of %d allowed values",
					maxAllowedValues, len(f.AllowedValues))
				break
			}
			if label := allowedValueLabel(value); label != "" {
				entry.AllowedValues = append(entry.AllowedValues, label)
			}
		}

		out = append(out, entry)
	}
	return out, nil
}

// GetEditFields reports which fields this issue's edit screen accepts, with
// their ids, types and allowed values. compact reduces each field to one entry;
// the full editmeta payload is returned verbatim when compact is false.
func (c *Client) GetEditFields(key string, compact bool) (string, error) {
	raw, err := c.get("/rest/api/2/issue/" + url.PathEscape(key) + "/editmeta")
	if err != nil {
		return "", err
	}
	if !compact {
		return raw, nil
	}

	fields, err := compactEditFields(raw)
	if err != nil {
		return "", err
	}
	out, err := json.Marshal(map[string]any{"issue_key": key, "fields": fields})
	if err != nil {
		return "", err
	}
	return string(out), nil
}
