// Package view turns store nodes into HTML pages: markdown conversion, the
// navigation sidebar, and the layout template. It is HTTP-agnostic — callers
// resolve paths to nodes (the server probes the tree, the render command
// walks it) and handle caching, redirects, and output themselves.
package view

import (
	"bytes"
	"cmp"
	"fmt"
	"html/template"
	"os"
	"path"
	"strings"

	"github.com/reddec/md-web/internal/store"
)

// Options configures rendering; transport concerns (binding, TLS,
// compression, caching) and output handling stay with the callers.
type Options struct {
	Listing bool // render directory listing pages when a dir has no index
	Nav     bool // include the navigation sidebar
	Title   bool // show title from frontmatter or filepath
}

// View renders store nodes to self-contained HTML pages. Every render builds
// its own conversion pipeline, so View instances share nothing.
type View struct {
	opts  Options
	templ *template.Template
}

// New prepares the view: parses the layout template.
func New(opts Options) (*View, error) {
	tpl, err := template.New("").Parse(layout)
	if err != nil {
		return nil, fmt.Errorf("parse layout: %w", err)
	}
	return &View{opts: opts, templ: tpl}, nil
}

// Render a store node into a self-contained HTML page: a markdown
// file page, or a directory's index page — falling back to the directory
// listing when enabled. The returned error wraps os.ErrNotExist when the
// node has no page (an index-less directory with listing disabled).
func (v *View) Render(n *store.Node) ([]byte, error) {
	if !n.Directory {
		return v.renderPage(n)
	}
	if idx := n.File("index.md"); idx != nil {
		return v.renderPage(idx)
	}
	if v.opts.Listing {
		return v.renderDirPage(n)
	}
	return nil, fmt.Errorf("render %q: %w", n.FullPath(), os.ErrNotExist)
}

// One markdown file page, sidebar included.
func (v *View) renderPage(n *store.Node) ([]byte, error) {
	doc, err := n.Store.Open(n.FullPath())
	if err != nil {
		return nil, fmt.Errorf("open page %q: %w", n.FullPath(), err)
	}
	defer doc.Close()

	content, err := doc.ReadBytes()
	if err != nil {
		return nil, fmt.Errorf("read file %q: %w", n.FullPath(), err)
	}
	output, err := convertMarkdown(n, content)
	if err != nil {
		return nil, fmt.Errorf("convert file %q: %w", n.FullPath(), err)
	}

	return v.renderHTML(pageParams{
		Path:      n.FullPath(),
		Title:     cmp.Or(doc.Front().Title, strings.TrimSuffix(n.Path, ".md")),
		Content:   template.HTML(output.String()),
		ShowTitle: v.opts.Title,
		Tags:      doc.Front().Tags,
		Nav:       v.nav(n.Store, n.FullPath()),
	})
}

// Directory listing page, sidebar included.
func (v *View) renderDirPage(n *store.Node) ([]byte, error) {
	list, err := n.Store.List(n.FullPath() + "/")
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	output, err := convertMarkdown(n, []byte(renderListing(!n.Store.IsRoot(n.FullPath()), list)))
	if err != nil {
		return nil, fmt.Errorf("convert listing %q: %w", n.FullPath(), err)
	}

	return v.renderHTML(pageParams{
		Path:      n.FullPath() + "/",
		Title:     n.Path + "/",
		Content:   template.HTML(output.String()),
		ShowTitle: v.opts.Title,
		Nav:       v.nav(n.Store, n.FullPath()+"/"),
	})
}

// nav builds the sidebar view for the rendered page path unless disabled.
// The tree is rebuilt per page so the sidebar always reflects the store.
func (v *View) nav(st *store.Store, p string) *navView {
	if !v.opts.Nav {
		return nil
	}
	tree, err := st.Tree(store.WithoutHidden, store.Files("*.md"))
	if err != nil {
		return nil // nav is auxiliary; never fail the page because of it
	}
	target := path.Clean(p)
	return &navView{
		root:    tree,
		current: tree.FindFunc(func(n *store.Node) bool { return n.FullPath() == target }),
	}
}

// renderHTML applies the layout template to the page params.
func (v *View) renderHTML(params pageParams) ([]byte, error) {
	var buf bytes.Buffer
	if err := v.templ.Execute(&buf, params); err != nil {
		return nil, fmt.Errorf("execute layout %q: %w", params.Path, err)
	}
	return buf.Bytes(), nil
}
