package confluence

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestSpaceEnumValue(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty means unset", input: "", want: ""},
		{name: "exact value", input: "global", want: "global"},
		{name: "case is normalised", input: "PERSONAL", want: "personal"},
		{name: "surrounding space is ignored", input: "  global  ", want: "global"},
		{name: "unknown value", input: "team", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := spaceEnumValue("space type", tt.input, "global", "personal")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("spaceEnumValue(%q) = %q, want an error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("spaceEnumValue(%q) error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("spaceEnumValue(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}

	// The caller cannot guess a fixed enum, so the error has to name it.
	_, err := spaceEnumValue("space type", "team", "global", "personal")
	if err == nil || !strings.Contains(err.Error(), "global or personal") {
		t.Errorf("spaceEnumValue error = %v, want it to list the valid values", err)
	}
}

func TestNormalizeSpaceKey(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "already uppercase", input: "DEV", want: "DEV"},
		{name: "lowercase is uppercased", input: "dev", want: "DEV"},
		{name: "digits are allowed", input: "team2", want: "TEAM2"},
		{name: "surrounding space is ignored", input: "  dev  ", want: "DEV"},
		{name: "empty", input: "", wantErr: true},
		{name: "space inside", input: "MY SPACE", wantErr: true},
		{name: "punctuation", input: "DEV-OPS", wantErr: true},
		{name: "underscore", input: "DEV_OPS", wantErr: true},
		{name: "personal space tilde", input: "~jane", wantErr: true},
		{name: "accent", input: "DÉV", wantErr: true},
		{name: "too long", input: strings.Repeat("A", 256), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeSpaceKey(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalizeSpaceKey(%q) = %q, want an error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeSpaceKey(%q) error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("normalizeSpaceKey(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}

	// The message has to teach the rule, since Confluence's own is unhelpful.
	_, err := normalizeSpaceKey("DEV-OPS")
	if err == nil || !strings.Contains(err.Error(), "letters and digits only") {
		t.Errorf("normalizeSpaceKey error = %v, want it to describe a valid key", err)
	}
}

func TestSpaceListQuery(t *testing.T) {
	tests := []struct {
		name      string
		spaceType string
		status    string
		label     string
		limit     int
		start     int
		want      string
		wantErr   bool
	}{
		{name: "nothing set", want: ""},
		{name: "limit only", limit: 25, want: "?limit=25"},
		{name: "start is dropped at zero", limit: 25, start: 0, want: "?limit=25"},
		{
			name:      "all filters",
			spaceType: "global",
			status:    "current",
			label:     "team",
			limit:     10,
			start:     20,
			want:      "?label=team&limit=10&start=20&status=current&type=global",
		},
		{name: "unknown type", spaceType: "team", wantErr: true},
		{name: "unknown status", status: "deleted", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := spaceListQuery(tt.spaceType, tt.status, tt.label, tt.limit, tt.start)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("spaceListQuery = %q, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("spaceListQuery error: %v", err)
			}
			if got != tt.want {
				t.Errorf("spaceListQuery = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSpaceContentPath(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		contentType string
		limit       int
		start       int
		want        string
		wantErr     bool
	}{
		{name: "no type", key: "DEV", limit: 25, want: "/rest/api/space/DEV/content?depth=root&limit=25"},
		{name: "pages only", key: "DEV", contentType: "page", limit: 25, want: "/rest/api/space/DEV/content/page?depth=root&limit=25"},
		{name: "blogposts with paging", key: "DEV", contentType: "blogpost", limit: 5, start: 10,
			want: "/rest/api/space/DEV/content/blogpost?depth=root&limit=5&start=10"},
		{name: "no paging", key: "DEV", want: "/rest/api/space/DEV/content?depth=root"},
		{name: "personal space key passes through", key: "~jane", want: "/rest/api/space/~jane/content?depth=root"},
		{name: "empty key", key: "", wantErr: true},
		{name: "unknown type", key: "DEV", contentType: "comment", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := spaceContentPath(tt.key, tt.contentType, tt.limit, tt.start)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("spaceContentPath = %q, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("spaceContentPath error: %v", err)
			}
			if got != tt.want {
				t.Errorf("spaceContentPath = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCompactSpaceList(t *testing.T) {
	full := `{
		"results": [
			{"id": 98305, "key": "DEV", "name": "Developer Docs", "type": "global",
			 "_links": {"self": "https://example/rest/api/space/DEV"},
			 "_expandable": {"homepage": "/rest/api/content/12345"}},
			{"id": 65537, "key": "~jane", "name": "Jane", "type": "personal"}
		],
		"start": 0, "limit": 25, "size": 2,
		"_links": {"base": "https://example"}
	}`

	got, err := compactSpaceList(full)
	if err != nil {
		t.Fatalf("compactSpaceList error: %v", err)
	}

	var listing struct {
		Results []spaceSummary `json:"results"`
		Size    int            `json:"size"`
		Limit   int            `json:"limit"`
	}
	if err := json.Unmarshal([]byte(got), &listing); err != nil {
		t.Fatalf("compactSpaceList produced invalid JSON %q: %v", got, err)
	}
	if len(listing.Results) != 2 || listing.Size != 2 || listing.Limit != 25 {
		t.Fatalf("compactSpaceList = %s, want 2 results and the paging envelope", got)
	}
	if listing.Results[0].Key != "DEV" || listing.Results[0].Name != "Developer Docs" ||
		listing.Results[0].Type != "global" || listing.Results[0].ID.String() != "98305" {
		t.Errorf("first entry = %+v, want the key, name, type and id", listing.Results[0])
	}

	// The point of compacting is dropping the link payload.
	if strings.Contains(got, "_links") || strings.Contains(got, "_expandable") {
		t.Errorf("compactSpaceList kept the link payload: %s", got)
	}
}

func TestCompactSpaceListEdgeCases(t *testing.T) {
	// An empty listing must stay a listing rather than marshalling results as
	// null, which reads as "the call failed" to a caller.
	got, err := compactSpaceList(`{"results": [], "start": 0, "limit": 25, "size": 0}`)
	if err != nil {
		t.Fatalf("compactSpaceList error: %v", err)
	}
	if !strings.Contains(got, `"results":[]`) {
		t.Errorf("compactSpaceList = %s, want an empty results array", got)
	}

	// A space with no id must not produce an invalid number literal.
	got, err = compactSpaceList(`{"results": [{"key": "DEV", "name": "D", "type": "global"}]}`)
	if err != nil {
		t.Fatalf("compactSpaceList error: %v", err)
	}
	if !json.Valid([]byte(got)) {
		t.Errorf("compactSpaceList produced invalid JSON: %s", got)
	}

	if _, err := compactSpaceList("<html>not json</html>"); err == nil {
		t.Error("compactSpaceList(non-JSON) = nil error, want an error")
	}
}

func TestUserSearchCQL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "plain name", input: "Jane Doe", want: `user.fullname ~ "Jane Doe"`},
		{name: "surrounding space is ignored", input: "  Jane  ", want: `user.fullname ~ "Jane"`},
		{name: "quotes are escaped", input: `Jane "JD" Doe`, want: `user.fullname ~ "Jane \"JD\" Doe"`},
		{name: "backslash is escaped", input: `DOMAIN\jane`, want: `user.fullname ~ "DOMAIN\\jane"`},
		{name: "empty", input: "   ", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := userSearchCQL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("userSearchCQL(%q) = %q, want an error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("userSearchCQL(%q) error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("userSearchCQL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// Which endpoint serves user CQL differs by Confluence version, so SearchUsers
// tries both. These cover each instance shape it can meet.
func TestSearchUsersEndpointFallback(t *testing.T) {
	t.Run("dedicated endpoint wins", func(t *testing.T) {
		var paths []string
		client, srv := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			paths = append(paths, r.URL.Path)
			w.Write([]byte(`{"results":["dedicated"]}`))
		}))
		defer srv.Close()

		got, err := client.SearchUsers("Jane", 0)
		if err != nil {
			t.Fatalf("SearchUsers error: %v", err)
		}
		if !strings.Contains(got, "dedicated") {
			t.Errorf("SearchUsers = %q, want the dedicated endpoint's body", got)
		}
		if len(paths) != 1 || paths[0] != "/rest/api/search/user" {
			t.Errorf("requested %v, want only /rest/api/search/user", paths)
		}
	})

	t.Run("falls back to the general endpoint", func(t *testing.T) {
		client, srv := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/rest/api/search/user" {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			w.Write([]byte(`{"results":["general"]}`))
		}))
		defer srv.Close()

		got, err := client.SearchUsers("Jane", 0)
		if err != nil {
			t.Fatalf("SearchUsers error: %v", err)
		}
		if !strings.Contains(got, "general") {
			t.Errorf("SearchUsers = %q, want the general endpoint's body", got)
		}
	})

	t.Run("neither endpoint", func(t *testing.T) {
		client, srv := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "not found", http.StatusNotFound)
		}))
		defer srv.Close()

		_, err := client.SearchUsers("Jane", 0)
		if err == nil {
			t.Fatal("SearchUsers = nil error, want one naming confluence_get_user")
		}
		if !strings.Contains(err.Error(), "confluence_get_user") {
			t.Errorf("SearchUsers error = %v, want it to point at confluence_get_user", err)
		}
	})
}
