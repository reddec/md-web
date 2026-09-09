package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/reddec/md-web/internal/store"
	"github.com/reddec/md-web/internal/view"
)

func createFile(t *testing.T, fPath string, content string) {
	t.Helper()
	err := os.MkdirAll(filepath.Dir(fPath), 0755)
	require.NoError(t, err)
	err = os.WriteFile(fPath, []byte(content), 0644)
	require.NoError(t, err)
}

// newNavFixture builds a docs tree:
//
//	index.md  a.md  notes.txt  .drafts/secret.md
//	x/index.md  x/b.md (frontmatter title: Beta)  x/sub/c.md
//	nested/deep/index.md
//	other/o.md
func newNavFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	createFile(t, filepath.Join(dir, "index.md"), "# Home")
	createFile(t, filepath.Join(dir, "a.md"), "# A")
	createFile(t, filepath.Join(dir, "notes.txt"), "not markdown")
	createFile(t, filepath.Join(dir, ".drafts", "secret.md"), "hidden")
	createFile(t, filepath.Join(dir, "x", "index.md"), "# X index")
	createFile(t, filepath.Join(dir, "x", "b.md"), "---\ntitle: Beta\n---\n# B")
	createFile(t, filepath.Join(dir, "x", "sub", "c.md"), "# C")
	createFile(t, filepath.Join(dir, "nested", "deep", "index.md"), "# Deep")
	createFile(t, filepath.Join(dir, "other", "o.md"), "# O")
	return dir
}

// newServer wires a Server for tests: view options as given, cache on demand.
func newServer(t *testing.T, dir string, opts view.Options, enableCache bool) *Server {
	t.Helper()
	st, err := store.New(dir)
	require.NoError(t, err)
	v, err := view.New(opts)
	require.NoError(t, err)
	return &Server{view: v, store: st, enableCache: enableCache}
}

func get(t *testing.T, srv *Server, target string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	require.Equal(t, http.StatusOK, rec.Code, "GET %s failed", target)
	return rec.Body.String()
}

func TestNavigationProgressive(t *testing.T) {
	srv := newServer(t, newNavFixture(t), view.Options{Nav: true}, false)

	body := get(t, srv, "/x/sub/c/")

	assert.Contains(t, body, "<nav class=\"sidebar\">")
	// the whole tree is rendered; CSS collapses everything off the current path
	assert.Contains(t, body, ".sidebar li:has(.active) > ul")
	assert.NotContains(t, body, "<details")
	// x/index.md: no frontmatter -> "Overview" fallback
	assert.Contains(t, body, "<a href=\"../../../x/\">Overview</a>")
	// current page is highlighted
	assert.Contains(t, body, "<a href=\"../../../x/sub/c\" class=\"active\">c</a>")
	// frontmatter title wins over filename
	assert.Contains(t, body, "<a href=\"../../../x/b\">Beta</a>")
	// filename fallback
	assert.Contains(t, body, "<a href=\"../../../a\">a</a>")
	// sibling branches are rendered too, just collapsed by CSS
	assert.Contains(t, body, "<a href=\"../../../other/o\">o</a>")
	// non-markdown and hidden entries are filtered out server-side
	assert.NotContains(t, body, "notes")
	assert.NotContains(t, body, "secret")
}

func TestNavigationRoot(t *testing.T) {
	srv := newServer(t, newNavFixture(t), view.Options{Nav: true}, false)

	body := get(t, srv, "/")

	assert.Contains(t, body, "<a href=\".\" class=\"active\">Home</a>")
	// only the home link is active, even though the whole tree is in the markup
	assert.Equal(t, 1, strings.Count(body, `class="active"`))
	// collapsed branches are present in the markup, hidden by CSS
	assert.Contains(t, body, "<a href=\"x/\">Overview</a>")
	assert.Contains(t, body, "<a href=\"other/o\">o</a>")
}

func TestNavigationDisabled(t *testing.T) {
	srv := newServer(t, newNavFixture(t), view.Options{Nav: false}, false)

	body := get(t, srv, "/")

	assert.NotContains(t, body, "<nav")
	assert.NotContains(t, body, "<input type=\"checkbox\"")
	assert.Contains(t, body, "<main>")
}

func TestNavigationCached(t *testing.T) {
	srv := newServer(t, newNavFixture(t), view.Options{Nav: true}, true)

	first := get(t, srv, "/x/sub/c/")
	second := get(t, srv, "/x/sub/c/")

	assert.Equal(t, first, second)
	assert.Contains(t, second, "<a href=\"../../../x/sub/c\" class=\"active\">c</a>")
}

func TestNavigationDirWithoutIndex(t *testing.T) {
	srv := newServer(t, newNavFixture(t), view.Options{Nav: true}, false)

	body := get(t, srv, "/x/")

	// sub has no index.md: linking its directory would 404, so it links to
	// its first page instead
	assert.Contains(t, body, "<a href=\"../x/sub/c\">sub</a>")
	assert.NotContains(t, body, "<a href=\"sub\">")
	// nested's first page is an index.md, linked as any other file
	assert.Contains(t, body, "<a href=\"../nested/deep/\">nested</a>")
	// x has its own index.md and keeps the directory link; on /x/ the index
	// page itself is the active entry
	assert.Contains(t, body, "<a href=\"../x/\">x</a>")
	assert.Contains(t, body, "<a href=\"../x/\" class=\"active\">Overview</a>")
}

func TestNavigationListing(t *testing.T) {
	srv := newServer(t, newNavFixture(t), view.Options{Listing: true, Nav: true}, false)

	body := get(t, srv, "/other/")

	// the listing page marks its directory node active; the link itself goes
	// to the directory's first page so that every nav link resolves
	assert.Contains(t, body, `<a href="../other/o" class="active">other</a>`)
	assert.Equal(t, 1, strings.Count(body, `class="active"`))
	assert.Contains(t, body, `<a href="../other/o">o</a>`)
}

func TestServeCanonicalRedirect(t *testing.T) {
	srv := newServer(t, newNavFixture(t), view.Options{Nav: true}, false)

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x/sub/c", nil))
	assert.Equal(t, http.StatusMovedPermanently, rec.Code)
	assert.Equal(t, "./c/", rec.Header().Get("Location"))

	// the canonical URL serves the page
	assert.Contains(t, get(t, srv, "/x/sub/c/"), `<h1 id="c">C</h1>`)

	// unknown paths redirect too; the destination then 404s — one extra
	// round trip is the accepted cost of the unconditional canonical redirect
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/missing", nil))
	assert.Equal(t, http.StatusMovedPermanently, rec.Code)
	assert.Equal(t, "./missing/", rec.Header().Get("Location"))

	// and the redirect destination 404s
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/missing/", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)

	// excluded (dot-prefixed) files redirect then 404, same as unknown paths
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.drafts/secret", nil))
	assert.Equal(t, http.StatusMovedPermanently, rec.Code)

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.drafts/secret/", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestServeDirIndexWinsClash(t *testing.T) {
	dir := t.TempDir()
	createFile(t, filepath.Join(dir, "a.md"), "# Page A")
	createFile(t, filepath.Join(dir, "a", "index.md"), "# Dir A")

	srv := newServer(t, dir, view.Options{Nav: true}, false)

	// the directory index owns /a/
	assert.Contains(t, get(t, srv, "/a/"), `<h1 id="dir-a">Dir A</h1>`)
	// the shadowed page stays reachable directly
	assert.Contains(t, get(t, srv, "/a.md"), `<h1 id="page-a">Page A</h1>`)
}
