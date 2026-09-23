package jira

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestClient points a Client at an httptest server.
func newTestClient(h http.Handler) (*Client, *httptest.Server) {
	srv := httptest.NewServer(h)
	return NewClient(srv.URL, "tok"), srv
}

// TestUpdateCommentKeepsVisibility covers the edit that would otherwise publish
// a restricted comment: the PUT replaces the comment, so the restriction has to
// be read back and resent.
func TestUpdateCommentKeepsVisibility(t *testing.T) {
	var put map[string]any
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			io.WriteString(w, `{"id":"10500","body":"internal note",
				"visibility":{"type":"role","value":"Administrators"}}`)
		case http.MethodPut:
			b, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(b, &put); err != nil {
				t.Errorf("PUT body is not JSON: %v", err)
			}
			io.WriteString(w, `{"id":"10500"}`)
		}
	})
	c, srv := newTestClient(h)
	defer srv.Close()

	if _, err := c.UpdateComment("DEV-1", "10500", "internal note (corrected)", "", ""); err != nil {
		t.Fatalf("UpdateComment error: %v", err)
	}

	vis, ok := put["visibility"].(map[string]any)
	if !ok {
		t.Fatalf("PUT body = %v, want it to carry the comment's visibility", put)
	}
	if vis["type"] != "role" || vis["value"] != "Administrators" {
		t.Errorf("visibility = %v, want the role restriction the comment had", vis)
	}
}

func TestUpdateCommentVisibilityOverrides(t *testing.T) {
	cases := []struct {
		name            string
		visibilityType  string
		visibilityValue string
		wantVisibility  any
	}{
		{name: "an explicit restriction is used as given", visibilityType: "group",
			visibilityValue: "jira-developers",
			wantVisibility:  map[string]any{"type": "group", "value": "jira-developers"}},
		{name: "none drops the restriction", visibilityType: "none", wantVisibility: nil},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			var put map[string]any
			gets := 0
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					gets++
					io.WriteString(w, `{"visibility":{"type":"role","value":"Administrators"}}`)
				case http.MethodPut:
					b, _ := io.ReadAll(r.Body)
					_ = json.Unmarshal(b, &put)
					io.WriteString(w, `{"id":"10500"}`)
				}
			})
			c, srv := newTestClient(h)
			defer srv.Close()

			if _, err := c.UpdateComment("DEV-1", "10500", "text", tt.visibilityType, tt.visibilityValue); err != nil {
				t.Fatalf("UpdateComment error: %v", err)
			}
			if gets != 0 {
				t.Errorf("read the comment %d time(s); an explicit visibility needs no read", gets)
			}

			got, present := put["visibility"]
			if tt.wantVisibility == nil {
				if present {
					t.Errorf("visibility = %v, want it left out", got)
				}
				return
			}
			want := tt.wantVisibility.(map[string]any)
			vis, ok := got.(map[string]any)
			if !ok || vis["type"] != want["type"] || vis["value"] != want["value"] {
				t.Errorf("visibility = %v, want %v", got, want)
			}
		})
	}
}

// TestAddRemoteLinkPreservesExistingFields covers the re-link the tool
// recommends: Jira replaces the whole link, so the fields the caller did not
// resupply have to be carried over from the link already stored.
func TestAddRemoteLinkPreservesExistingFields(t *testing.T) {
	const pageURL = "https://confluence.example.com/display/DOC/Page"

	var post map[string]any
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if got := r.URL.Query().Get("globalId"); got != pageURL {
				t.Errorf("globalId = %q, want %q", got, pageURL)
			}
			io.WriteString(w, `[{"id":1,"globalId":"`+pageURL+`","relationship":"documentation",
				"application":{"type":"com.atlassian.confluence","name":"Confluence"},
				"object":{"url":"`+pageURL+`","title":"Old","summary":"What shipped in 4.2"}}]`)
		case http.MethodPost:
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &post)
			io.WriteString(w, `{"id":1}`)
		}
	})
	c, srv := newTestClient(h)
	defer srv.Close()

	if _, err := c.AddRemoteLink("DEV-1", pageURL, "Release Notes", "", "", "", "", ""); err != nil {
		t.Fatalf("AddRemoteLink error: %v", err)
	}

	if post["relationship"] != "documentation" {
		t.Errorf("relationship = %v, want the existing link's relationship", post["relationship"])
	}
	if _, ok := post["application"].(map[string]any); !ok {
		t.Errorf("application = %v, want the existing application object", post["application"])
	}
	object, _ := post["object"].(map[string]any)
	if object["summary"] != "What shipped in 4.2" {
		t.Errorf("summary = %v, want the existing summary", object["summary"])
	}
	if object["title"] != "Release Notes" {
		t.Errorf("title = %v, want the supplied title", object["title"])
	}
}

// A first-time link has nothing to read back; a 404 on the lookup must not stop
// the write.
func TestAddRemoteLinkWithoutExistingLink(t *testing.T) {
	var post map[string]any
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.WriteHeader(http.StatusNotFound)
		case http.MethodPost:
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &post)
			io.WriteString(w, `{"id":1}`)
		}
	})
	c, srv := newTestClient(h)
	defer srv.Close()

	if _, err := c.AddRemoteLink("DEV-1", "https://example.com/p", "P", "", "", "", "", ""); err != nil {
		t.Fatalf("AddRemoteLink error: %v", err)
	}
	if _, present := post["relationship"]; present {
		t.Errorf("POST body = %v, want no relationship invented", post)
	}
}

// TestSetFieldsReadsTheSiblingEstimate covers JRASERVER-30459: setting one
// estimate alone makes Jira rewrite the other, so the other is read and resent.
func TestSetFieldsReadsTheSiblingEstimate(t *testing.T) {
	var put map[string]any
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if got := r.URL.Query().Get("fields"); got != "timetracking" {
				t.Errorf("fields = %q, want only timetracking", got)
			}
			io.WriteString(w, `{"fields":{"timetracking":{"originalEstimate":"5d","remainingEstimate":"2d"}}}`)
		case http.MethodPut:
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &put)
			io.WriteString(w, `{}`)
		}
	})
	c, srv := newTestClient(h)
	defer srv.Close()

	if _, err := c.SetFields("DEV-1", FieldEdits{RemainingEstimate: "1d"}); err != nil {
		t.Fatalf("SetFields error: %v", err)
	}

	fields, _ := put["fields"].(map[string]any)
	tt, _ := fields["timetracking"].(map[string]any)
	if tt["originalEstimate"] != "5d" || tt["remainingEstimate"] != "1d" {
		t.Errorf("timetracking = %v, want the stored original estimate alongside the new remaining one", tt)
	}
}

// Both estimates supplied: nothing needs reading.
func TestSetFieldsSkipsTheReadWhenBothEstimatesAreGiven(t *testing.T) {
	gets := 0
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			gets++
		}
		io.WriteString(w, `{}`)
	})
	c, srv := newTestClient(h)
	defer srv.Close()

	if _, err := c.SetFields("DEV-1", FieldEdits{OriginalEstimate: "3d", RemainingEstimate: "1d"}); err != nil {
		t.Fatalf("SetFields error: %v", err)
	}
	if gets != 0 {
		t.Errorf("read the issue %d time(s); both estimates were supplied", gets)
	}
}
