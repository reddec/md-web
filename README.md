# md-web

Lightweight Go web server that serves Markdown files as styled HTML pages. No build step, no JavaScript — just point it at a directory of `.md` files and go.

## Features

- Markdown to HTML rendering via [Goldmark](https://github.com/yuin/goldmark)
- GitHub Flavored Markdown (tables, strikethrough, autolinks, task lists)
- Relative, mount-agnostic links (works under any subpath)
- Syntax highlighting for code blocks
- Wiki links (`[[Page]]`)
- Math rendering (MathML)
- Mermaid diagrams
- YAML frontmatter for metadata
- HTML-to-Markdown URL rewriting (`.html` requests served as `.md`)
- Directory listing for folders without `index.md`
- Progressive navigation sidebar (drawer on mobile, enabled by default, no JS)
- Static site rendering (`render` subcommand) for GitHub Pages, Netlify and friends
- In-memory page cache (enabled by default)
- GZIP compression (enabled by default)
- TLS/HTTPS support
- OIDC authentication
- Graceful shutdown
- Mobile-friendly, no-JS HTML output
- Single binary, no external dependencies at runtime

Supports rootless Docker via `scratch` base image with static binary.

## Install

Linux one-liner (amd64 and arm64):

```bash
curl -fsSL "https://github.com/reddec/md-web/releases/latest/download/md-web_linux-$(uname -m)" | install -Dm755 /dev/stdin ~/.local/bin/md-web
```

Or with Go:

```bash
go install github.com/reddec/md-web@latest
```

Docker:

```bash
docker run -v ./docs:/data -p 8080:8080 ghcr.io/reddec/md-web
```

## Usage

`md-web` serves the current directory by default (`serve` is the default command):

```bash
# Serve current directory on :8080
md-web

# Serve a specific directory
md-web -d /path/to/markdown

# Show titles and enable directory listing
md-web -t -l

# Rewrite .html URLs to .md
md-web --html-rewrite

# With TLS
md-web --tls-enabled --tls-cert-file cert.crt --tls-key-file key.key

# Render a self-contained static site into ./dist
md-web render -d /path/to/markdown
```

## Configuration

All flags can also be set via environment variables.

### serve

| Flag | Short | Env | Default | Description |
|------|-------|-----|---------|-------------|
| `--bind` | `-b` | `MDWEB_BIND` | `:8080` | HTTP server bind address |
| `--data` | `-d` | `MDWEB_DATA` | `./` | Directory with Markdown files |
| `--disable-cache` | | `MDWEB_DISABLE_CACHE` | `false` | Disable in-memory page cache |
| `--title` | `-t` | `MDWEB_TITLE` | `false` | Show title from frontmatter or filename |
| `--html-rewrite` | | `MDWEB_HTML_REWRITE` | `false` | Rewrite `.html` URLs to `.md` |
| `--listing` | `-l` | `MDWEB_LISTING` | `false` | Enable directory listing if no `index.md` present |
| `--disable-nav` | | `MDWEB_DISABLE_NAV` | `false` | Disable navigation sidebar |
| `--tls-enabled` | | `MDWEB_TLS_ENABLED` | `false` | Enable HTTPS |
| `--tls-cert-file` | | `MDWEB_TLS_CERT` | `/etc/tls/tls.crt` | Path to TLS certificate |
| `--tls-key-file` | | `MDWEB_TLS_KEY` | `/etc/tls/tls.key` | Path to TLS private key |
| `--disable-gzip` | | `MDWEB_DISABLE_GZIP` | `false` | Disable gzip compression |
| `--graceful-shutdown` | | `MDWEB_GRACEFUL_SHUTDOWN` | `10s` | Graceful shutdown timeout |
| `--oidc-enabled` | | `MDWEB_OIDC_ENABLED` | `false` | Enable OIDC authentication |
| `--oidc-issuer` | | `MDWEB_OIDC_ISSUER` | | OIDC issuer URL |
| `--oidc-client-id` | | `MDWEB_OIDC_CLIENT_ID` | | OIDC client ID |
| `--oidc-client-secret` | | `MDWEB_OIDC_CLIENT_SECRET` | | OIDC client secret |
| `--oidc-trust-proxy` | | `MDWEB_OIDC_TRUST_PROXY` | `false` | Trust X-Forwarded-* headers |

### render

| Flag | Short | Env | Default | Description |
|------|-------|-----|---------|-------------|
| `--data` | `-d` | `MDWEB_DATA` | `./` | Directory with Markdown files |
| `--out` | `-o` | `MDWEB_OUT` | `dist` | Output directory for the static site |
| `--title` | `-t` | `MDWEB_TITLE` | `false` | Show title from frontmatter or filename |
| `--disable-nav` | | `MDWEB_DISABLE_NAV` | `false` | Disable navigation sidebar |

Rendering is idempotent: the output directory may live inside the data directory (its subtree is skipped during asset copying), files are overwritten in place, and nothing is ever deleted.

## URL Routing

Every page lives in its directory — the same canonical layout the `render` command writes:

- `/` and `/path/` serve `index.md` from the corresponding directory
- `/path/` serves `path.md` when the directory has no `index.md`; if both `path.md` and `path/index.md` exist, the directory index wins and `path.md` remains reachable at `/path.md`
- `/path/` shows a directory listing if neither exists (when `--listing` is enabled)
- `/page` redirects to `/page/`
- `/page.md` serves the page directly
- `/page.html` serves `page.md` (when `--html-rewrite` is enabled)
- Content links to `.md` pages are rewritten to the canonical `/dir/` form
- Hidden files and directories (dot-prefixed, e.g. `.drafts/`) are excluded from serving, rendering, listings, and asset copying

## Static Rendering

`md-web render` generates a self-contained static site from a markdown directory:

- every page is written to its canonical path: `a/b/c.md` becomes `a/b/c/index.html`, `x/index.md` becomes `x/index.html`
- non-markdown files (images, downloads) are copied verbatim
- directory listings are a serve-time feature; `render` only emits pages that exist
- content links are already canonical (the server rewrites `.md` links to `/dir/` form), so pages work unchanged
- pages are fully self-contained (styles are inlined), so the result works on any static hosting — GitHub Pages, Netlify and Cloudflare Pages resolve extension-less URLs like `/a/b/c` to the directory index automatically

The rendered site keeps the same URLs as the server: what you see under `serve` is what you get in `render`.

## Frontmatter

Pages can include YAML frontmatter:

```markdown
---
title: My Page
tags: [docs, example]
---

# Content here
```

(readme generated by LLM, code wasn't)

## License

MIT License, RedDec, 2026.
