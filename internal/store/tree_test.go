package store_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/reddec/md-web/internal/store"
)

func TestTree(t *testing.T) {
	dir := t.TempDir()
	createFile(t, filepath.Join(dir, "a.md"), "hello")
	createFile(t, filepath.Join(dir, "docs/readme.md"), "readme")
	createFile(t, filepath.Join(dir, "docs/guide/deep.md"), "deep")

	st, err := store.New(dir)
	require.NoError(t, err)

	root, err := st.Tree()
	require.NoError(t, err)

	// root
	assert.Equal(t, "/", root.Path)
	assert.True(t, root.Directory)
	assert.Nil(t, root.Parent)

	// root children, in os.ReadDir (sorted) order
	require.Len(t, root.Children, 2)
	a, docs := root.Children[0], root.Children[1]
	assert.Equal(t, store.Entry{Path: "a.md", Directory: false}, a.Entry)
	assert.Same(t, root, a.Parent)
	assert.Empty(t, a.Children)
	assert.Equal(t, store.Entry{Path: "docs", Directory: true}, docs.Entry)
	assert.Same(t, root, docs.Parent)

	// second level
	require.Len(t, docs.Children, 2)
	guide, readme := docs.Children[0], docs.Children[1]
	assert.Equal(t, store.Entry{Path: "guide", Directory: true}, guide.Entry)
	assert.Same(t, docs, guide.Parent)
	assert.Equal(t, store.Entry{Path: "readme.md", Directory: false}, readme.Entry)
	assert.Same(t, docs, readme.Parent)

	// third level
	require.Len(t, guide.Children, 1)
	deep := guide.Children[0]
	assert.Equal(t, store.Entry{Path: "deep.md", Directory: false}, deep.Entry)
	assert.Same(t, guide, deep.Parent)
}

func TestTreeFullPath(t *testing.T) {
	dir := t.TempDir()
	createFile(t, filepath.Join(dir, "docs/guide/deep.md"), "deep")

	st, err := store.New(dir)
	require.NoError(t, err)

	root, err := st.Tree()
	require.NoError(t, err)

	docs := child(t, root, "docs")
	guide := child(t, docs, "guide")
	deep := child(t, guide, "deep.md")

	assert.Equal(t, "/", root.FullPath())
	assert.Equal(t, "/docs", docs.FullPath())
	assert.Equal(t, "/docs/guide", guide.FullPath())
	assert.Equal(t, "/docs/guide/deep.md", deep.FullPath())
}

func TestTreeEmpty(t *testing.T) {
	st, err := store.New(t.TempDir())
	require.NoError(t, err)

	root, err := st.Tree()
	require.NoError(t, err)

	assert.Equal(t, "/", root.Path)
	assert.True(t, root.Directory)
	assert.Empty(t, root.Children)
}

// A nested directory whose name also exists at the top level must list its own
// children, not the top-level namesake's.
func TestTreeSameDirNameAtDifferentDepths(t *testing.T) {
	dir := t.TempDir()
	createFile(t, filepath.Join(dir, "guide/top.md"), "top")
	createFile(t, filepath.Join(dir, "docs/guide/nested.md"), "nested")

	st, err := store.New(dir)
	require.NoError(t, err)

	root, err := st.Tree()
	require.NoError(t, err)

	assert.Equal(t, []string{"top.md"}, childNames(child(t, root, "guide")))
	assert.Equal(t, []string{"nested.md"}, childNames(child(t, child(t, root, "docs"), "guide")))
}

func TestTreeWithoutHidden(t *testing.T) {
	dir := t.TempDir()
	createFile(t, filepath.Join(dir, "a.md"), "hello")
	createFile(t, filepath.Join(dir, ".secret"), "secret")
	createFile(t, filepath.Join(dir, ".hidden/inside.md"), "inside")
	createFile(t, filepath.Join(dir, "docs/readme.md"), "readme")

	st, err := store.New(dir)
	require.NoError(t, err)

	root, err := st.Tree(store.WithoutHidden)
	require.NoError(t, err)

	assert.Equal(t, []string{"a.md", "docs"}, childNames(root))
	assert.Equal(t, []string{"readme.md"}, childNames(child(t, root, "docs")))
}

func TestTreeFiles(t *testing.T) {
	dir := t.TempDir()
	createFile(t, filepath.Join(dir, "a.md"), "a")
	createFile(t, filepath.Join(dir, "b.txt"), "b")
	createFile(t, filepath.Join(dir, "docs/readme.md"), "readme")
	createFile(t, filepath.Join(dir, "docs/notes.txt"), "notes")
	createFile(t, filepath.Join(dir, "docs/sub/deep.md"), "deep")

	st, err := store.New(dir)
	require.NoError(t, err)

	root, err := st.Tree(store.Files("*.md"))
	require.NoError(t, err)

	// directories always pass, so traversal reaches every level
	assert.Equal(t, []string{"a.md", "docs"}, childNames(root))
	assert.Equal(t, []string{"readme.md", "sub"}, childNames(child(t, root, "docs")))
	assert.Equal(t, []string{"deep.md"}, childNames(child(t, child(t, root, "docs"), "sub")))
}

func TestTreeCombinedFilters(t *testing.T) {
	dir := t.TempDir()
	createFile(t, filepath.Join(dir, "a.md"), "a")
	createFile(t, filepath.Join(dir, ".draft.md"), "draft")
	createFile(t, filepath.Join(dir, "docs/readme.md"), "readme")
	createFile(t, filepath.Join(dir, "docs/.ignore.md"), "ignore")
	createFile(t, filepath.Join(dir, "docs/notes.txt"), "notes")

	st, err := store.New(dir)
	require.NoError(t, err)

	root, err := st.Tree(store.WithoutHidden, store.Files("*.md"))
	require.NoError(t, err)

	assert.Equal(t, []string{"a.md", "docs"}, childNames(root))
	assert.Equal(t, []string{"readme.md"}, childNames(child(t, root, "docs")))
}

func TestTreeMissingRoot(t *testing.T) {
	dir := t.TempDir()
	st, err := store.New(dir)
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(dir))

	_, err = st.Tree()
	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)
	assert.Contains(t, err.Error(), `list "/"`)
}

func TestNodeFindFunc(t *testing.T) {
	dir := t.TempDir()
	createFile(t, filepath.Join(dir, "a.md"), "a")
	createFile(t, filepath.Join(dir, "docs/guide/deep.md"), "deep")

	st, err := store.New(dir)
	require.NoError(t, err)

	root, err := st.Tree()
	require.NoError(t, err)

	// lookup by full path
	docs := root.FindFunc(func(n *store.Node) bool { return n.FullPath() == "/docs" })
	require.NotNil(t, docs)
	assert.Equal(t, "docs", docs.Path)

	deep := root.FindFunc(func(n *store.Node) bool { return n.FullPath() == "/docs/guide/deep.md" })
	require.NotNil(t, deep)
	assert.Equal(t, "deep.md", deep.Path)

	// the receiver itself is a candidate
	assert.Same(t, root, root.FindFunc(func(n *store.Node) bool { return n.Parent == nil }))

	// first match in depth-first order wins (a.md sorts before docs but is a file;
	// the root itself is excluded from the predicate)
	assert.Same(t, docs, root.FindFunc(func(n *store.Node) bool { return n.Directory && n.Parent != nil }))

	// identity predicate
	assert.Same(t, deep, root.FindFunc(func(n *store.Node) bool { return n == deep }))

	// no match
	assert.Nil(t, root.FindFunc(func(n *store.Node) bool { return false }))
}

func TestNodeMetadata(t *testing.T) {
	dir := t.TempDir()
	createFile(t, filepath.Join(dir, "docs/b.md"), "---\ntitle: Beta\n---\nB")
	createFile(t, filepath.Join(dir, "docs/a.md"), "plain")

	st, err := store.New(dir)
	require.NoError(t, err)
	root, err := st.Tree(store.WithoutHidden, store.Files("*.md"))
	require.NoError(t, err)

	docs := root.FindFunc(func(n *store.Node) bool { return n.FullPath() == "/docs" })
	b := root.FindFunc(func(n *store.Node) bool { return n.FullPath() == "/docs/b.md" })

	// directories have no metadata
	fm, err := docs.Metadata()
	require.NoError(t, err)
	assert.Equal(t, store.Frontmatter{}, fm)

	fm, err = b.Metadata()
	require.NoError(t, err)
	assert.Equal(t, "Beta", fm.Title)

	// detached nodes cannot be read
	detached := &store.Node{Entry: store.Entry{Path: "x.md"}}
	fm, err = detached.Metadata()
	assert.Error(t, err)
	assert.Equal(t, store.Frontmatter{}, fm)

	// attaching a Store to a hand-built node makes it readable
	manual := &store.Node{Entry: store.Entry{Path: "docs/b.md"}, Store: st}
	fm, err = manual.Metadata()
	require.NoError(t, err)
	assert.Equal(t, "Beta", fm.Title)
}

func child(t *testing.T, parent *store.Node, name string) *store.Node {
	t.Helper()
	for _, c := range parent.Children {
		if c.Path == name {
			return c
		}
	}
	t.Fatalf("no child %q under %q: %v", name, parent.FullPath(), childNames(parent))
	return nil
}

func childNames(parent *store.Node) []string {
	names := make([]string, 0, len(parent.Children))
	for _, c := range parent.Children {
		names = append(names, c.Path)
	}
	return names
}
