package markdown

import (
	"regexp"
	"sort"
	"strings"
)

var (
	// Any Confluence macro, to read its name out of ac:name.
	reMacroName = regexp.MustCompile(`<ac:structured-macro[^>]*\bac:name="([^"]*)"`)

	// Any remaining Confluence-specific element.
	reConfluenceElement = regexp.MustCompile(`<((?:ac|ri):[a-zA-Z0-9-]+)`)
)

// macroStructureElements are the wrappers that only ever carry another
// construct's content; naming the construct itself is more useful than naming
// its plumbing.
var macroStructureElements = map[string]bool{
	"ac:structured-macro":     true,
	"ac:parameter":            true,
	"ac:plain-text-body":      true,
	"ac:rich-text-body":       true,
	"ac:plain-text-link-body": true,
}

// UntranslatedMacros lists the Confluence-specific constructs in a storage body
// that ToMarkdown does not translate, so a caller can tell that editing the
// Markdown and writing it back would drop them. Macros are named by their
// ac:name ("info", "toc"); anything else by its element name ("ac:task-list").
// The constructs ToMarkdown does handle — the code macro and plain attachment
// or URL images — are excluded, so a faithfully converted page reports nothing.
func UntranslatedMacros(storage string) []string {
	// Remove what the converter understands, so only the losses remain.
	rest := reCodeMacro.ReplaceAllString(storage, "")
	rest = reAttachImg.ReplaceAllString(rest, "")
	rest = reURLImg.ReplaceAllString(rest, "")

	found := map[string]bool{}
	for _, m := range reMacroName.FindAllStringSubmatch(rest, -1) {
		if name := strings.TrimSpace(m[1]); name != "" {
			found[name] = true
		}
	}
	for _, m := range reConfluenceElement.FindAllStringSubmatch(rest, -1) {
		if !macroStructureElements[m[1]] {
			found[m[1]] = true
		}
	}

	if len(found) == 0 {
		return nil
	}

	names := make([]string, 0, len(found))
	for n := range found {
		names = append(names, n)
	}
	sort.Strings(names)

	return names
}
