package jira

import (
	"strings"
	"testing"
)

func TestMatchIssueLinkType(t *testing.T) {
	catalog := []issueLinkType{
		{ID: "10000", Name: "Blocks", Inward: "is blocked by", Outward: "blocks"},
		{ID: "10001", Name: "Relates", Inward: "relates to", Outward: "relates to"},
		{ID: "10002", Name: "Duplicate", Inward: "is duplicated by", Outward: "duplicates"},
	}

	tests := []struct {
		name     string
		input    string
		wantID   string
		wantSwap bool
		wantErr  bool
	}{
		{name: "type name", input: "Blocks", wantID: "10000"},
		{name: "type name is case insensitive", input: "duplicate", wantID: "10002"},
		{name: "type id", input: "10001", wantID: "10001"},
		{name: "outward description keeps the order", input: "blocks", wantID: "10000"},
		{name: "inward description reverses the ends", input: "is blocked by", wantID: "10000", wantSwap: true},
		{name: "description is case insensitive", input: "Is Duplicated By", wantID: "10002", wantSwap: true},
		{name: "symmetric type does not swap", input: "relates to", wantID: "10001"},
		{name: "unknown type", input: "supersedes", wantErr: true},
		{name: "empty", input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, swap, err := matchIssueLinkType(catalog, tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("matchIssueLinkType(%q) = %+v, want an error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("matchIssueLinkType(%q) error: %v", tt.input, err)
			}
			if got.ID != tt.wantID || swap != tt.wantSwap {
				t.Errorf("matchIssueLinkType(%q) = id %s, swap %v; want id %s, swap %v",
					tt.input, got.ID, swap, tt.wantID, tt.wantSwap)
			}
		})
	}

	// A name must outrank another type's direction description, so a link type
	// called "Duplicate" is not resolved as "Blocks" by a colliding wording.
	collide := []issueLinkType{
		{ID: "1", Name: "Cloners", Inward: "is cloned by", Outward: "clones"},
		{ID: "2", Name: "clones", Inward: "is cloned from", Outward: "clones into"},
	}
	got, swap, err := matchIssueLinkType(collide, "clones")
	if err != nil {
		t.Fatalf("matchIssueLinkType(\"clones\") error: %v", err)
	}
	if got.ID != "2" || swap {
		t.Errorf("matchIssueLinkType(\"clones\") = id %s, swap %v; want the type named \"clones\" (id 2)", got.ID, swap)
	}

	// An unknown type should name the valid ones with both directions.
	_, _, err = matchIssueLinkType(catalog, "supersedes")
	if err == nil || !strings.Contains(err.Error(), `Blocks ("blocks" / "is blocked by")`) {
		t.Errorf("matchIssueLinkType error = %v, want it to list the configured link types", err)
	}

	if _, _, err := matchIssueLinkType(nil, "Blocks"); err == nil {
		t.Error("matchIssueLinkType(nil, \"Blocks\") = nil error, want an error")
	}
}
