package view_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/reddec/md-web/internal/store"
	"github.com/reddec/md-web/internal/view"
)

// newFixture builds a docs tree:
//
//	index.md  a.md  other.md
//	x/index.md  x/sub/c.md
func newFixture(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	write := func(name, content string) {
		p := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
	write("index.md", "# Home")
	write("a.md", "# A")
	write("other.md", "# Other")
	write("x/index.md", "# X index")
	write("x/sub/c.md", "# C")

	st, err := store.New(dir)
	require.NoError(t, err)
	return st
}

func nodeAt(t *testing.T, st *store.Store, p string) *store.Node {
	t.Helper()
	tree, err := st.Tree()
	require.NoError(t, err)
	n := tree.FindFunc(func(n *store.Node) bool { return n.FullPath() == p })
	require.NotNil(t, n, "node %q not found", p)
	return n
}

func newView(t *testing.T, mutate func(*view.Options)) *view.View {
	t.Helper()
	opts := view.Options{Nav: true}
	if mutate != nil {
		mutate(&opts)
	}
	v, err := view.New(opts)
	require.NoError(t, err)
	return v
}

func TestViewRenderFilePage(t *testing.T) {
	st := newFixture(t)
	v := newView(t, nil)

	content, err := v.Render(nodeAt(t, st, "/a.md"))
	require.NoError(t, err)
	s := string(content)

	assert.Contains(t, s, "<h1 id=\"a\">A</h1>")
	// nav links are the page's RootPath climb plus the root-relative target
	assert.Contains(t, s, `<a href="../a" class="active">a</a>`)
	assert.Contains(t, s, `<a href="../">Home</a>`)
	assert.Contains(t, s, `<a href="../x/">x</a>`)
	assert.Contains(t, s, `<a href="../x/sub/c">sub</a>`)
}

func TestViewRenderIndexPageAnchorsAtItsDirectory(t *testing.T) {
	st := newFixture(t)
	v := newView(t, nil)

	content, err := v.Render(nodeAt(t, st, "/x/index.md"))
	require.NoError(t, err)
	s := string(content)

	assert.Contains(t, s, "<h1 id=\"x-index\">X index</h1>")
	assert.Contains(t, s, `<a href="../x/" class="active">Overview</a>`)
	assert.Contains(t, s, `<a href="../a">a</a>`)
}

func TestViewRenderDirNodePrefersIndex(t *testing.T) {
	st := newFixture(t)
	v := newView(t, nil)

	content, err := v.Render(nodeAt(t, st, "/x"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "<h1 id=\"x-index\">X index</h1>")
}

func TestViewRenderListing(t *testing.T) {
	st := newFixture(t)
	v := newView(t, func(o *view.Options) { o.Listing = true })

	content, err := v.Render(nodeAt(t, st, "/x/sub"))
	require.NoError(t, err)
	// content links are canonical: c.md → c/, parent stays ../
	assert.Contains(t, string(content), `href="../../x/sub/c/"`)
	assert.Contains(t, string(content), `href="../../">`)
}

func TestViewRenderNoListing(t *testing.T) {
	st := newFixture(t)
	v := newView(t, nil)

	_, err := v.Render(nodeAt(t, st, "/x/sub"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

// The view renders whatever node it gets — ownership of canonical URLs
// (a.md shadowed by a/index.md) belongs to the callers.
func TestViewRenderShadowedPageStillRenders(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		p := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
	write("a.md", "# Page A")
	write("a/index.md", "# Dir A")
	st, err := store.New(dir)
	require.NoError(t, err)

	v := newView(t, nil)
	content, err := v.Render(nodeAt(t, st, "/a.md"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "Page A")
}

// ForRender canonicalizes content links relative to the page's canonical
// directory; serve mode keeps raw destinations (it serves .md directly).
func TestViewContentLinksCanonicalized(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		p := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
	write("a.md", "# A\n\n[Other](other.md)\n\n[Top](/index.md)\n")
	write("other.md", "# Other")
	st, err := store.New(dir)
	require.NoError(t, err)

	v := newView(t, nil)
	content, err := v.Render(nodeAt(t, st, "/a.md"))
	require.NoError(t, err)
	assert.Contains(t, string(content), `href="../other/"`)
	// site-absolute destinations re-anchor at the root climb too
	assert.Contains(t, string(content), `href="../"`)
}
