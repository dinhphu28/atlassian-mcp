package markdown

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// Diagram is a Mermaid block extracted from Markdown. Placeholder is the exact
// token left in the storage output for the caller to substitute.
type Diagram struct {
	Placeholder string
	Source      string
}

// ToStorage converts Markdown to Confluence storage XHTML. Fenced ```mermaid
// blocks are replaced with placeholders and returned as diagrams for the caller
// to render and substitute.
func ToStorage(md string) (string, []Diagram, error) {
	r := &storageRenderer{}
	gm := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRenderer(
			renderer.NewRenderer(
				renderer.WithNodeRenderers(
					util.Prioritized(html.NewRenderer(html.WithUnsafe()), 1000),
					util.Prioritized(r, 100),
				),
			),
		),
	)
	var buf bytes.Buffer
	if err := gm.Convert([]byte(md), &buf); err != nil {
		return "", nil, err
	}
	return buf.String(), r.diagrams, nil
}

type storageRenderer struct {
	diagrams []Diagram
}

func (r *storageRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindFencedCodeBlock, r.renderFenced)
	reg.Register(ast.KindCodeBlock, r.renderIndentedCode)
	reg.Register(ast.KindImage, r.renderImage)
}

func (r *storageRenderer) renderFenced(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	fc := n.(*ast.FencedCodeBlock)
	lang := string(fc.Language(source))
	code := nodeLines(n, source)
	if lang == "mermaid" {
		d := Diagram{
			Placeholder: fmt.Sprintf("<!--mermaid:%d-->", len(r.diagrams)),
			Source:      code,
		}
		r.diagrams = append(r.diagrams, d)
		_, _ = w.WriteString(d.Placeholder)
		return ast.WalkSkipChildren, nil
	}
	_, _ = w.WriteString(CodeMacro(lang, code))
	return ast.WalkSkipChildren, nil
}

func (r *storageRenderer) renderIndentedCode(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	_, _ = w.WriteString(CodeMacro("", nodeLines(n, source)))
	return ast.WalkSkipChildren, nil
}

func (r *storageRenderer) renderImage(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	img := n.(*ast.Image)
	dest := string(img.Destination)
	if strings.Contains(dest, "://") {
		_, _ = w.WriteString(URLImage(dest))
	} else {
		_, _ = w.WriteString(AttachmentImage(dest))
	}
	return ast.WalkSkipChildren, nil
}

// nodeLines concatenates the raw text lines of a code node.
func nodeLines(n ast.Node, source []byte) string {
	var b strings.Builder
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		b.Write(seg.Value(source))
	}
	return b.String()
}
