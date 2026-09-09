package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/reddec/md-web/internal/store"
	"github.com/reddec/md-web/internal/view"
)

func newRenderer(t *testing.T, data string, opts view.Options) *renderer {
	t.Helper()
	st, err := store.New(data)
	require.NoError(t, err)
	v, err := view.New(opts)
	require.NoError(t, err)
	return &renderer{
		view:    v,
		store:   st,
		dataDir: data,
	}
}

func renderSite(t *testing.T, data string, opts view.Options, out string) {
	t.Helper()
	require.NoError(t, newRenderer(t, data, opts).renderTo(out))
}

func outFileSet(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		require.NoError(t, err)
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	require.NoError(t, err)
	return files
}

func TestRenderFileSet(t *testing.T) {
	data := newNavFixture(t)
	out := t.TempDir()

	renderSite(t, data, view.Options{Nav: true}, out)

	assert.ElementsMatch(t, []string{
		"index.html",             // /index.md
		"a/index.html",           // /a.md
		"x/index.html",           // /x/index.md
		"x/b/index.html",         // /x/b.md
		"x/sub/c/index.html",     // /x/sub/c.md
		"nested/deep/index.html", // /nested/deep/index.md
		"other/o/index.html",     // /other/o.md
		"notes.txt",              // copied verbatim; hidden files excluded
	}, outFileSet(t, out))
}

func TestRenderMatchesServe(t *testing.T) {
	data := newNavFixture(t)
	out := t.TempDir()

	renderSite(t, data, view.Options{Nav: true}, out)

	srv := newServer(t, data, view.Options{Nav: true}, false)

	// serve addresses pages at the same canonical directories the render
	// command writes, so the whole page — nav included — must be identical
	for _, target := range []string{"/", "/a/", "/x/", "/x/sub/c/", "/nested/deep/"} {
		rendered, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(strings.TrimPrefix(target, "/")), "index.html"))
		require.NoError(t, err, "GET %s counterpart", target)
		assert.Equal(t, get(t, srv, target), string(rendered), "GET %s", target)
	}
}

// mainSection extracts the rendered page body between <main> and </main>.
func mainSection(t *testing.T, page string) string {
	t.Helper()
	_, body, found := strings.Cut(page, "<main>")
	require.True(t, found, "no <main> in page")
	content, _, found := strings.Cut(body, "</main>")
	require.True(t, found, "no </main> in page")
	return content
}

func TestRenderMdLinksRewritten(t *testing.T) {
	data := t.TempDir()
	createFile(t, filepath.Join(data, "links.md"), "[page](other.md)\n\n[home](index.md)\n\n[sub](sub/page.md)\n\n![pic](pic.png)\n\n[ext](https://example.com/x.md)\n\n[here](#section)\n")
	createFile(t, filepath.Join(data, "other.md"), "# Other")
	createFile(t, filepath.Join(data, "sub", "page.md"), "# Sub")
	createFile(t, filepath.Join(data, "pic.png"), "PNGDATA")

	out := t.TempDir()
	renderSite(t, data, view.Options{Nav: true}, out)

	html, err := os.ReadFile(filepath.Join(out, "links", "index.html"))
	require.NoError(t, err)
	s := string(html)
	// every relative link climbs out of the page's canonical directory /links/
	assert.Contains(t, s, `href="../other/"`)
	assert.Contains(t, s, `href="../"`)
	assert.Contains(t, s, `href="../sub/page/"`)
	assert.Contains(t, s, `src="../pic.png"`)
	assert.Contains(t, s, `href="https://example.com/x.md"`)
	assert.Contains(t, s, `href="#section"`)

	assert.Contains(t, s, `href="../links" class="active"`) // self: /links/
	assert.Contains(t, s, `href="../other"`)

	// copied verbatim next to the page
	_, err = os.Stat(filepath.Join(out, "pic.png"))
	require.NoError(t, err)
}

// TestRenderPageWinsOverListing pins the serve/render agreement for a
// dir-named page: with listing enabled, /a/ must serve and render a.md even
// though the directory a/ also exists.
func TestRenderFilePageNavHrefs(t *testing.T) {
	// every link of a page is its RootPath climb plus the root-relative
	// target — self-links included
	data := newNavFixture(t)
	out := t.TempDir()

	renderSite(t, data, view.Options{Nav: true}, out)

	html, err := os.ReadFile(filepath.Join(out, "a", "index.html"))
	require.NoError(t, err)
	s := string(html)
	assert.Contains(t, s, `href="../a" class="active"`) // self: /a/
	assert.Contains(t, s, `href="../"`)                 // root index page
	assert.Contains(t, s, `href="../x/"`)               // directory with index
	assert.Contains(t, s, `href="../nested/deep/"`)
	assert.Contains(t, s, `href="../x/sub/c"`)
}

func TestRenderOutInsideDataIsIdempotent(t *testing.T) {
	// rendering into a subdirectory of the data dir is legal: the output
	// subtree is skipped during asset copying, so re-rendering never nests
	// or grows the output
	data := t.TempDir()
	createFile(t, filepath.Join(data, "a.md"), "# A")
	createFile(t, filepath.Join(data, "pic.png"), "PNG")

	out := filepath.Join(data, "dist")
	renderSite(t, data, view.Options{}, out)

	assert.FileExists(t, filepath.Join(out, "a", "index.html"))
	assert.FileExists(t, filepath.Join(out, "pic.png"))
	assert.NoFileExists(t, filepath.Join(out, "dist")) // no self-nesting

	// second render is idempotent
	renderSite(t, data, view.Options{}, out)
	assert.NoFileExists(t, filepath.Join(out, "dist"))
}

func TestRenderDirIndexWinsClash(t *testing.T) {
	dir := t.TempDir()
	createFile(t, filepath.Join(dir, "a.md"), "# Page A")
	createFile(t, filepath.Join(dir, "a", "index.md"), "# Dir A")

	out := t.TempDir()
	renderSite(t, dir, view.Options{Nav: true}, out)

	// the directory index owns a/index.html; the shadowed page is skipped
	html, err := os.ReadFile(filepath.Join(out, "a", "index.html"))
	require.NoError(t, err)
	assert.Contains(t, string(html), "Dir A")
	assert.NotContains(t, string(html), "Page A")
}
