package view

import (
	"html/template"
	"path"
	"strings"

	"github.com/reddec/md-web/internal/store"
)

// pageParams is the data the layout template renders.
type pageParams struct {
	Path      string
	Title     string
	Tags      []string
	Content   template.HTML // rendered markdown, marked safe for the layout
	ShowTitle bool
	Nav       *navView
}

// navView renders one level of the store tree as navigation sidebar entries.
// The store tree is the single source of truth; titles, links, and the active
// marker derive from the root node at render time. Descending is copying the
// view rooted one level deeper, so the template recurses over plain methods.
type navView struct {
	root    *store.Node
	current *store.Node // node of the rendered page; nil when it has no nav entry
}

// Name returns the display fallback label: the file name without extension;
// index.md resolves to Overview, or Home at the root; directories to their
// name.
func (v navView) Name() string {
	n := v.root
	name := strings.TrimSuffix(n.Path, ".md")
	if name != "index" {
		return name
	}
	if n.Parent.FullPath() == "/" {
		return "Home"
	}
	return "Overview"
}

// Metadata returns the frontmatter of the node; zero when unreadable — the
// sidebar never fails a page over metadata.
func (v navView) Metadata() store.Frontmatter {
	fm, _ := v.root.Metadata()
	return fm
}

// The entry's link, relative to the rendered page:
// the current page's RootPath plus the root-relative target. Directories
// link to their own page when it has an index.md, otherwise to their first
// page beneath it — a bare directory link would 404.
func (v navView) Link() string {
	n := v.root

	// what the entry points at: the extension-less page path, a directory
	// with trailing slash, or the first page beneath an index-less directory
	var target string
	switch {
	case !n.Directory:
		target = strings.TrimSuffix(n.FullPath(), ".md")
		if path.Base(target) == "index" {
			target = strings.TrimSuffix(target, "index") // an index page links as its directory
		}
	case n.File("index.md") != nil:
		target = n.FullPath() + "/"
	default:
		if fp, ok := firstPage(n); ok {
			target = fp
		} else {
			target = n.FullPath() + "/" // pageless dirs never reach the sidebar
		}
	}

	rel := strings.TrimPrefix(target, "/")
	if rel == "" {
		if root := v.current.RootPath(); root != "" {
			return root
		}
		return "." // the store root, reached from the root page itself
	}
	return v.current.RootPath() + rel
}

// Active reports whether the root node is the rendered page itself.

// Active reports whether the root node is the rendered page itself.
func (v navView) Active() bool { return v.root == v.current }

// Children returns the same view rooted at each renderable child: files and
// directories whose subtree contains at least one page.
func (v navView) Children() []navView {
	var out []navView
	for _, child := range v.root.Children {
		if child.Directory && !hasVisible(child) {
			continue
		}
		out = append(out, navView{root: child, current: v.current})
	}
	return out
}

// firstPage walks depth-first and returns the extension-less path of the
// first markdown page beneath n; ok is false when no page exists.
func firstPage(n *store.Node) (target string, ok bool) {
	for _, child := range n.Children {
		if child.Directory {
			if t, found := firstPage(child); found {
				return t, true
			}
			continue
		}
		t := strings.TrimSuffix(child.FullPath(), ".md")
		if path.Base(t) == "index" {
			t = strings.TrimSuffix(t, "index")
		}
		return t, true
	}
	return "", false
}

// hasVisible reports whether the subtree of n contains at least one markdown
// page.
func hasVisible(n *store.Node) bool {
	for _, child := range n.Children {
		if !child.Directory || hasVisible(child) {
			return true
		}
	}
	return false
}
