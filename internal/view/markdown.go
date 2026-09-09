package view

import (
	"bytes"
	"path"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
	"go.abhg.dev/goldmark/frontmatter"
	"go.abhg.dev/goldmark/mermaid"
	"go.abhg.dev/goldmark/wikilink"

	treeblood "github.com/wyatt915/goldmark-treeblood"

	"github.com/reddec/md-web/internal/store"
)

// convertMarkdown renders the markdown of a node's page through a fresh
// pipeline; the canonicalizer is scoped to that node, so nothing is shared
// between conversions and callers never need to synchronize.
func convertMarkdown(n *store.Node, content []byte) (*bytes.Buffer, error) {
	canon := &mdCanonicalizer{node: n}
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			&wikilink.Extender{},
			treeblood.MathML(),
			&mermaid.Extender{},
			&frontmatter.Extender{},
			highlighting.Highlighting,
		),
		goldmark.WithParserOptions(
			parser.WithASTTransformers(
				util.Prioritized(canon, 90),
			),
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
		),
	)

	var output bytes.Buffer
	if err := md.Convert(content, &output); err != nil {
		return nil, err
	}
	return &output, nil
}

// mdCanonicalizer rewrites relative link destinations of the page to the
// page's canonical directory: .md destinations map to their directory form,
// so the same href works when the page is served (/a) and when it is written
// for static hosting (/a/index.html). Absolute URLs, anchors and
// site-absolute paths are left untouched.
type mdCanonicalizer struct {
	node *store.Node // the page being converted
}

func (m *mdCanonicalizer) Transform(node *ast.Document, _ text.Reader, _ parser.Context) {
	ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *ast.Link:
			v.Destination = m.canonicalDest(v.Destination)
		case *ast.Image:
			v.Destination = m.canonicalDest(v.Destination)
		}
		return ast.WalkContinue, nil
	})
}

// parentDir is the directory relative destinations resolve against when
// served: the parent of a file page, the directory itself for a directory
// page.
func (m *mdCanonicalizer) parentDir() string {
	if m.node.Directory {
		return m.node.FullPath() + "/"
	}
	return m.node.Parent.FullPath() + "/"
}

// canonicalDest re-anchors dest at the page's canonical location: relative
// destinations resolve against the page's parent directory, site-absolute
// ones against the store root; .md targets map to their directory form. The
// result is the page's RootPath followed by the target relative to the root.
func (m *mdCanonicalizer) canonicalDest(dest []byte) []byte {
	target := string(dest)
	if target == "" || target[0] == '#' || strings.Contains(target, "://") {
		return dest
	}
	isMD := strings.HasSuffix(target, ".md")
	isDir := isMD || strings.HasSuffix(target, "/")

	if !strings.HasPrefix(target, "/") {
		target = path.Join(m.parentDir(), target)
	} else {
		target = path.Clean(target)
	}
	if isMD {
		target = strings.TrimSuffix(target, ".md")
		if path.Base(target) == "index" {
			target = path.Dir(target)
		}
	}
	if isDir {
		target = strings.TrimSuffix(target, "/") + "/"
	}

	rel := strings.TrimPrefix(target, "/")
	if rel == "" {
		return []byte(m.node.RootPath())
	}
	return []byte(m.node.RootPath() + rel)
}
