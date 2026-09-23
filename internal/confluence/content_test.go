package confluence

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "404", err: fmt.Errorf("confluence error 404: no such resource"), want: true},
		{name: "403 is not a 404", err: fmt.Errorf("confluence error 403: forbidden"), want: false},
		{name: "transport error", err: errors.New("dial tcp: connection refused"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := notFound(tt.err); got != tt.want {
				t.Errorf("notFound(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestParseAncestors(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    []Breadcrumb
		wantErr bool
	}{
		{
			name: "root first, extra fields ignored",
			raw: `{"id":"40","title":"Leaf","ancestors":[
				{"id":"10","type":"page","title":"Home","_links":{"webui":"/x"}},
				{"id":"20","type":"page","title":"Section"},
				{"id":"30","type":"page","title":"Parent"}]}`,
			want: []Breadcrumb{{ID: "10", Title: "Home"}, {ID: "20", Title: "Section"}, {ID: "30", Title: "Parent"}},
		},
		{
			name: "a space root has no ancestors",
			raw:  `{"id":"10","title":"Home","ancestors":[]}`,
			want: []Breadcrumb{},
		},
		{
			name: "missing ancestors is not an error",
			raw:  `{"id":"10","title":"Home"}`,
			want: []Breadcrumb{},
		},
		{name: "not JSON", raw: "<html>login</html>", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAncestors(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseAncestors(%q) = %+v, want an error", tt.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseAncestors error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseAncestors = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSplitList(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "single", input: "alice", want: []string{"alice"}},
		{name: "several with spaces", input: "alice, bob ,carol", want: []string{"alice", "bob", "carol"}},
		{name: "empty entries dropped", input: "alice,,bob,", want: []string{"alice", "bob"}},
		{name: "empty", input: "", want: []string{}},
		{name: "none clears", input: "none", want: []string{}},
		{name: "none is case insensitive", input: " NONE ", want: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := splitList(tt.input); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitList(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseRestrictions(t *testing.T) {
	raw := `{
		"read": {"operation":"read","restrictions":{
			"user":{"results":[{"type":"known","username":"alice","userKey":"k1"}],"size":1},
			"group":{"results":[{"type":"group","name":"devs"}],"size":1}}},
		"update": {"operation":"update","restrictions":{
			"user":{"results":[],"size":0},
			"group":{"results":[],"size":0}}}}`

	got, err := parseRestrictions(raw)
	if err != nil {
		t.Fatalf("parseRestrictions error: %v", err)
	}

	want := map[string]restrictionSet{
		"read": {
			Users:  []restrictionUser{{Username: "alice", UserKey: "k1"}},
			Groups: []string{"devs"},
		},
		"update": {Users: []restrictionUser{}, Groups: []string{}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseRestrictions = %+v, want %+v", got, want)
	}

	if _, err := parseRestrictions("not json"); err == nil {
		t.Error("parseRestrictions(\"not json\") = nil error, want an error")
	}
}

// names pulls the usernames and group names back out of a payload entry, so the
// expectations below read as the lists a caller would think in.
func names(entry map[string]any) (operation string, users, groups []string) {
	operation, _ = entry["operation"].(string)
	restrictions, _ := entry["restrictions"].(map[string]any)

	for _, u := range restrictions["user"].([]map[string]any) {
		if name, ok := u["username"].(string); ok {
			users = append(users, name)
			continue
		}
		users = append(users, "key:"+u["userKey"].(string))
	}
	for _, g := range restrictions["group"].([]map[string]any) {
		groups = append(groups, g["name"].(string))
	}
	return operation, users, groups
}

func TestBuildRestrictionPayload(t *testing.T) {
	current := map[string]restrictionSet{
		"read": {
			Users:  []restrictionUser{{Username: "alice"}, {UserKey: "k2"}},
			Groups: []string{"devs"},
		},
		"update": {
			Users:  []restrictionUser{{Username: "bob"}},
			Groups: []string{},
		},
	}

	tests := []struct {
		name                              string
		current                           map[string]restrictionSet
		read, update                      restrictionInput
		wantReadUsers, wantReadGroups     []string
		wantUpdateUsers, wantUpdateGroups []string
	}{
		{
			name:    "an untouched operation is carried over verbatim",
			current: current,
			read:    restrictionInput{Users: "carol"},
			// The read groups and the whole update operation were not
			// mentioned, so they must survive the wholesale replace.
			wantReadUsers: []string{"carol"}, wantReadGroups: []string{"devs"},
			wantUpdateUsers: []string{"bob"}, wantUpdateGroups: nil,
		},
		{
			name:          "a userKey-only member is carried over as a userKey",
			current:       current,
			read:          restrictionInput{Groups: "admins"},
			wantReadUsers: []string{"alice", "key:k2"}, wantReadGroups: []string{"admins"},
			wantUpdateUsers: []string{"bob"}, wantUpdateGroups: nil,
		},
		{
			name:          "none clears one operation",
			current:       current,
			read:          restrictionInput{Users: "none", Groups: "none"},
			wantReadUsers: nil, wantReadGroups: nil,
			wantUpdateUsers: []string{"bob"}, wantUpdateGroups: nil,
		},
		{
			name:          "a page with no restrictions yet",
			current:       map[string]restrictionSet{},
			update:        restrictionInput{Groups: "devs, admins"},
			wantReadUsers: nil, wantReadGroups: nil,
			wantUpdateUsers: nil, wantUpdateGroups: []string{"devs", "admins"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := buildRestrictionPayload(tt.current, tt.read, tt.update)

			// Both operations are always sent: omitting one would unrestrict it.
			if len(payload) != 2 {
				t.Fatalf("payload has %d entries, want 2", len(payload))
			}

			op, users, groups := names(payload[0])
			if op != "read" {
				t.Errorf("first entry operation = %q, want read", op)
			}
			if !reflect.DeepEqual(users, tt.wantReadUsers) {
				t.Errorf("read users = %v, want %v", users, tt.wantReadUsers)
			}
			if !reflect.DeepEqual(groups, tt.wantReadGroups) {
				t.Errorf("read groups = %v, want %v", groups, tt.wantReadGroups)
			}

			op, users, groups = names(payload[1])
			if op != "update" {
				t.Errorf("second entry operation = %q, want update", op)
			}
			if !reflect.DeepEqual(users, tt.wantUpdateUsers) {
				t.Errorf("update users = %v, want %v", users, tt.wantUpdateUsers)
			}
			if !reflect.DeepEqual(groups, tt.wantUpdateGroups) {
				t.Errorf("update groups = %v, want %v", groups, tt.wantUpdateGroups)
			}
		})
	}
}

func TestCopyTitleAndSpace(t *testing.T) {
	titles := []struct {
		name     string
		newTitle string
		source   string
		want     string
	}{
		{name: "given title wins", newTitle: "Release plan", source: "Template", want: "Release plan"},
		{name: "blank title falls back", newTitle: "   ", source: "Template", want: "Copy of Template"},
		{name: "no title falls back", newTitle: "", source: "Template", want: "Copy of Template"},
	}

	for _, tt := range titles {
		t.Run(tt.name, func(t *testing.T) {
			if got := copyTitle(tt.newTitle, tt.source); got != tt.want {
				t.Errorf("copyTitle(%q, %q) = %q, want %q", tt.newTitle, tt.source, got, tt.want)
			}
		})
	}

	spaces := []struct {
		name   string
		target string
		source string
		want   string
	}{
		{name: "target wins", target: "OPS", source: "DEV", want: "OPS"},
		{name: "no target copies in place", target: "", source: "DEV", want: "DEV"},
		{name: "blank target copies in place", target: " ", source: "DEV", want: "DEV"},
	}

	for _, tt := range spaces {
		t.Run(tt.name, func(t *testing.T) {
			if got := copySpace(tt.target, tt.source); got != tt.want {
				t.Errorf("copySpace(%q, %q) = %q, want %q", tt.target, tt.source, got, tt.want)
			}
		})
	}
}

// TestCopyPageKeepsTheSourceType covers copying a blog post: re-publishing its
// body as a "page" would produce content that never appears in the blog feed.
func TestCopyPageKeepsTheSourceType(t *testing.T) {
	var post string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			io.WriteString(w, `{"id":"12345","type":"blogpost","title":"Release 4.2",
				"space":{"key":"DEV"},"version":{"number":3},"body":{"storage":{"value":"<p>hi</p>"}}}`)
		case http.MethodPost:
			b, _ := io.ReadAll(r.Body)
			post = string(b)
			io.WriteString(w, `{"id":"99"}`)
		}
	})
	c, srv := newTestClient(h)
	defer srv.Close()

	if _, err := c.CopyPage("12345", "", "", ""); err != nil {
		t.Fatalf("CopyPage error: %v", err)
	}
	if !strings.Contains(post, `"type":"blogpost"`) {
		t.Errorf("POST body = %s, want the copy created as a blogpost", post)
	}

	// A blog post has no ancestors, so a parent has to be refused by name
	// rather than reaching Confluence as an opaque error.
	if _, err := c.CopyPage("12345", "", "777", ""); err == nil {
		t.Error("CopyPage(blogpost, parent) = nil error, want it refused")
	}
}

// A page with no explicit type still copies as a page.
func TestCopyPageDefaultsToPage(t *testing.T) {
	var post string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			io.WriteString(w, `{"id":"12345","title":"Docs","space":{"key":"DEV"},
				"version":{"number":1},"body":{"storage":{"value":"<p>hi</p>"}}}`)
		case http.MethodPost:
			b, _ := io.ReadAll(r.Body)
			post = string(b)
			io.WriteString(w, `{"id":"99"}`)
		}
	})
	c, srv := newTestClient(h)
	defer srv.Close()

	if _, err := c.CopyPage("12345", "", "", ""); err != nil {
		t.Fatalf("CopyPage error: %v", err)
	}
	if !strings.Contains(post, `"type":"page"`) {
		t.Errorf("POST body = %s, want the copy created as a page", post)
	}
}
