package main

import (
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/reddec/md-web/internal/store"
	"github.com/reddec/md-web/internal/view"
)

type renderCmd struct {
	Data       string `name:"data" short:"d" env:"MDWEB_DATA" help:"Serving directory" default:"./"`
	Out        string `name:"out" short:"o" env:"MDWEB_OUT" help:"Output directory" default:"dist"`
	Title      bool   `name:"title" short:"t" env:"MDWEB_TITLE" help:"Show title from metadata or filepath"`
	DisableNav bool   `help:"Disable navigation sidebar" env:"MDWEB_DISABLE_NAV"`
}

func (c *renderCmd) Run() error {
	st, err := store.New(c.Data)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	v, err := view.New(view.Options{
		Nav:   !c.DisableNav,
		Title: c.Title,
	})
	if err != nil {
		return fmt.Errorf("build view: %w", err)
	}
	w := &renderer{
		view:    v,
		store:   st,
		dataDir: c.Data,
	}
	return w.renderTo(c.Out)
}

// renderer writes every markdown page of a store to its canonical output
// location.
type renderer struct {
	view    *view.View
	store   *store.Store
	dataDir string
}

// Write the whole site as static files: every markdown page lands
// on its canonical directory path (…/index.html), non-markdown files are
// copied verbatim. The result is self-contained and uploadable to any static
// hosting.
func (r *renderer) renderTo(out string) error {
	dataAbs, err := filepath.Abs(r.dataDir)
	if err != nil {
		return fmt.Errorf("resolve data dir: %w", err)
	}
	outAbs, err := filepath.Abs(out)
	if err != nil {
		return fmt.Errorf("resolve output dir: %w", err)
	}
	if err := os.MkdirAll(outAbs, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	tree, err := r.store.Tree(store.WithoutHidden, store.Files("*.md"))
	if err != nil {
		return fmt.Errorf("build tree: %w", err)
	}

	if err := r.node(tree, outAbs); err != nil {
		return err
	}
	return copyAssets(dataAbs, outAbs, outAbs)
}

// node walks n and everything beneath it, writing one output file per
// markdown page. A dir-named page (x.md next to x/) yields to the
// directory's index (x/index.md owns x/), matching serve resolution.
func (r *renderer) node(n *store.Node, parentOut string) error {
	if n.Directory {
		ownOut := filepath.Join(parentOut, n.Entry.Path) // Join drops the root's "/"
		for _, child := range n.Children {
			if err := r.node(child, ownOut); err != nil {
				return err
			}
		}
		return nil
	}
	// a dir-named page yields to the directory's index (x/index.md owns x/),
	// matching serve resolution
	name := strings.TrimSuffix(n.Entry.Path, ".md")
	if dir := n.Parent.Dir(name); dir != nil && dir.File("index.md") != nil {
		slog.Warn("page shadowed by directory index", "page", n.FullPath())
		return nil
	}

	content, err := r.view.Render(n)
	if err != nil {
		return fmt.Errorf("render %q: %w", n.FullPath(), err)
	}
	return r.writePage(r.pagePath(n, parentOut), content)
}

// pagePath maps a node to its canonical output file: a/b/c.md →
// a/b/c/index.html, x/index.md → x/index.html, a directory → its own
// index.html.
func (r *renderer) pagePath(n *store.Node, parentDir string) string {
	if n.Directory {
		return filepath.Join(parentDir, n.Entry.Path, "index.html")
	}
	name := strings.TrimSuffix(n.Entry.Path, ".md")
	if name == "index" {
		return filepath.Join(parentDir, "index.html")
	}
	return filepath.Join(parentDir, name, "index.html")
}

// writePage writes a rendered page file.
func (r *renderer) writePage(dst string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create dir for %q: %w", dst, err)
	}
	if err := os.WriteFile(dst, content, 0o644); err != nil {
		return fmt.Errorf("write %q: %w", dst, err)
	}
	slog.Info("rendered", "file", dst)
	return nil
}

// copyAssets duplicates every non-markdown file from src into dst keeping
// relative paths, so images and downloads referenced by pages keep working.
// Hidden (dot-prefixed) entries are excluded, same as the pages, and the
// skip directory (the output itself) is never walked — rendering into a
// subdirectory of the source stays idempotent.
func copyAssets(src, dst, skip string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %q: %w", p, err)
		}
		if p == skip {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if p != src && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() || strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return fmt.Errorf("relativize %q: %w", p, err)
		}
		target := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create dir for %q: %w", target, err)
		}
		if err := copyFile(p, target); err != nil {
			return err
		}
		slog.Info("copied", "file", target)
		return nil
	})
}

// copyFile copies src to dst as a stream — asset sizes are unbounded.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %q: %w", src, err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create %q: %w", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("copy into %q: %w", dst, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close %q: %w", dst, err)
	}
	return nil
}
