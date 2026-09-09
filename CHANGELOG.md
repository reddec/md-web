# Changelog

## 1.2.0

### Added

- `render` subcommand that generates a self-contained static site for static hosting: pages are written to canonical paths (`a/b/c.md` → `a/b/c/index.html`), content links ending with `.md` are rewritten to the same paths, and non-markdown files are copied verbatim. `serve` is now the default command, so existing invocations keep working.

### Changed

- Removed `--base` / `-B` (`MDWEB_BASE`). Links are relative and canonical now, so the site works under any subpath without a base prefix.
- Hidden files and directories (dot-prefixed) are excluded from serving, rendering, listings, and asset copying.
- Serving uses canonical URLs: `/page` redirects to `/page/`, matching the layout `render` writes. Markdown content links are canonicalized too (`page.md` → `page/`). When both `page.md` and `page/index.md` exist, the directory index owns `/page/` and the page remains reachable at `/page.md`.
- Page caching is enabled by default. Disable it with `--disable-cache` (`MDWEB_DISABLE_CACHE`); the previous `--cache` / `-c` flags were removed. The navigation sidebar is costly to rebuild on every request, which is why caching is now the default.
