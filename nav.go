package main

import (
	"strings"

	"github.com/reddec/md-web/internal/store"
)

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

// URL returns the href of the root node relative to the rendered page: ../
// hops up to the store root, then the path down to the node. Browsers
// normalize the climb, so the sidebar works under any mount base.
// Directories link to their own page when it has an index.md, otherwise to
// their first page beneath it — a bare directory link would 404. Pages
// without a node in the tree (hidden or special) fall back to absolute store
// paths.
func (v navView) URL() string {
	n := v.root
	down := strings.TrimPrefix(n.FullPath(), "/")
	if n.Directory {
		if hasIndex(n) {
			down += "/"
		} else {
			down, _ = firstPage(n) // rendered directories always have a page
			down = strings.TrimPrefix(down, "/")
		}
	} else {
		down = strings.TrimSuffix(down, ".md")
	}
	if v.current == nil {
		return "/" + down
	}
	return strings.Repeat("../", v.upHops()) + down
}

// upHops counts the ../ steps from the rendered page directory up to the
// store root. A file page lives in its parent directory, a directory page in
// the directory itself.
func (v navView) upHops() int {
	hops := strings.Count(strings.Trim(v.current.FullPath(), "/"), "/")
	if v.current.Directory {
		hops++
	}
	return hops
}

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

func hasIndex(n *store.Node) bool {
	for _, child := range n.Children {
		if !child.Directory && child.Path == "index.md" {
			return true
		}
	}
	return false
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
		return strings.TrimSuffix(child.FullPath(), ".md"), true
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
