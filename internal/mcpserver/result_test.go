package mcpserver

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("result has no content")
	}
	text, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("result content is %T, want TextContent", res.Content[0])
	}
	return text.Text
}

// Storage XHTML must survive the round trip through the renderer: Go's default
// JSON escaping would turn every < into \u003c, tripling the size of a
// macro-heavy body and inviting the agent to write the escapes back.
func TestJSONResultDoesNotEscapeStorageMarkup(t *testing.T) {
	raw := `{"body":{"storage":{"value":"<ac:structured-macro ac:name=\"info\"><p>a & b</p></ac:structured-macro>"}}}`

	res, err := jsonResult(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := resultText(t, res)

	if strings.Contains(out, `\u003c`) || strings.Contains(out, `\u0026`) {
		t.Errorf("storage markup came back escaped: %s", out)
	}
	if !strings.Contains(out, `<ac:structured-macro ac:name=\"info\">`) {
		t.Errorf("expected verbatim macro markup, got: %s", out)
	}
}

// A non-JSON body (an HTML login page served with status 200) used to render as
// the literal "null", hiding the real response.
func TestJSONResultPassesNonJSONThrough(t *testing.T) {
	res, _ := jsonResult("<html>login</html>", nil)
	if out := resultText(t, res); out != "<html>login</html>" {
		t.Errorf("non-JSON body rendered as %q", out)
	}

	res, _ = jsonResult("", nil)
	if out := resultText(t, res); out != "(empty response)" {
		t.Errorf("empty body rendered as %q", out)
	}
}
