// Package markdown converts between Markdown and Confluence storage format.
package markdown

import "strings"

// CodeMacro renders source as a Confluence code macro. An empty lang omits the
// language parameter.
func CodeMacro(lang, source string) string {
	var b strings.Builder
	b.WriteString(`<ac:structured-macro ac:name="code">`)
	if lang != "" {
		b.WriteString(`<ac:parameter ac:name="language">`)
		b.WriteString(lang)
		b.WriteString(`</ac:parameter>`)
	}
	b.WriteString(`<ac:plain-text-body><![CDATA[`)
	b.WriteString(source)
	b.WriteString(`]]></ac:plain-text-body></ac:structured-macro>`)
	return b.String()
}

// AttachmentImage renders a storage-format image referencing a page attachment.
func AttachmentImage(filename string) string {
	return `<ac:image><ri:attachment ri:filename="` + filename + `"/></ac:image>`
}

// URLImage renders a storage-format image referencing an external URL.
func URLImage(url string) string {
	return `<ac:image><ri:url ri:value="` + url + `"/></ac:image>`
}
