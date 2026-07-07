package markdown

import (
	"html"
	"regexp"

	htmltomd "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/JohannesKaufmann/html-to-markdown/plugin"
)

var (
	// Confluence code macro -> <pre><code>. Captures optional language and the
	// CDATA body. (?s) makes . match newlines.
	reCodeMacro = regexp.MustCompile(
		`(?s)<ac:structured-macro[^>]*ac:name="code".*?<ac:plain-text-body><!\[CDATA\[(.*?)\]\]></ac:plain-text-body>.*?</ac:structured-macro>`)
	reCodeLang = regexp.MustCompile(`<ac:parameter ac:name="language">(.*?)</ac:parameter>`)

	// Storage images -> <img>.
	reAttachImg = regexp.MustCompile(`<ac:image[^>]*>\s*<ri:attachment ri:filename="([^"]*)"[^>]*/>\s*</ac:image>`)
	reURLImg    = regexp.MustCompile(`<ac:image[^>]*>\s*<ri:url ri:value="([^"]*)"[^>]*/>\s*</ac:image>`)
)

// ToMarkdown converts Confluence storage XHTML to Markdown. Known macros (code,
// images) are pre-normalized to plain HTML; unknown macros fall through to the
// generic HTML-to-Markdown pass on a best-effort basis.
func ToMarkdown(storage string) (string, error) {
	h := preprocessStorage(storage)
	conv := htmltomd.NewConverter("", true, nil)
	// Confluence tables are standard HTML tables; without this plugin they are
	// stripped to concatenated cell text. Table renders them as GFM pipe tables.
	conv.Use(plugin.Table())
	return conv.ConvertString(h)
}

func preprocessStorage(s string) string {
	s = reCodeMacro.ReplaceAllStringFunc(s, func(m string) string {
		lang := ""
		if lm := reCodeLang.FindStringSubmatch(m); lm != nil {
			lang = lm[1]
		}
		body := reCodeMacro.FindStringSubmatch(m)[1]
		open := "<pre><code>"
		if lang != "" {
			open = `<pre><code class="language-` + lang + `">`
		}
		return open + html.EscapeString(body) + "</code></pre>"
	})
	s = reAttachImg.ReplaceAllString(s, `<img src="$1" alt="$1"/>`)
	s = reURLImg.ReplaceAllString(s, `<img src="$1"/>`)
	return s
}
