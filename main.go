package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/NYTimes/gziphandler"
	"github.com/alecthomas/kong"
	oidclogin "github.com/reddec/oidc-login"

	"github.com/reddec/md-web/internal/store"
	"github.com/reddec/md-web/internal/view"
)

type serveCmd struct {
	Bind             string        `name:"bind" short:"b" env:"MDWEB_BIND" help:"Binding address" default:":8080"`
	GracefulShutdown time.Duration `name:"graceful-shutdown" env:"MDWEB_GRACEFUL_SHUTDOWN" help:"Graceful shutdown timeout for server" default:"10s"`
	Data             string        `name:"data" short:"d" env:"MDWEB_DATA" help:"Serving directory" default:"./"`
	DisableCache     bool          `env:"MDWEB_DISABLE_CACHE" help:"Disable in-memory page cache"`
	Title            bool          `name:"title" short:"t" env:"MDWEB_TITLE" help:"Show title from metadata or filepath"`
	DisableGZIP      bool          `help:"Disable gzip compression for HTTP" env:"MDWEB_DISABLE_GZIP"`
	HTMLRewrite      bool          `name:"html-rewrite" env:"MDWEB_HTML_REWRITE" help:"Re-write .html to .md"`
	Listing          bool          `name:"listing" short:"l" env:"MDWEB_LISTING" help:"Enable directory listing if no index.md there" `
	DisableNav       bool          `help:"Disable navigation sidebar" env:"MDWEB_DISABLE_NAV"`
	TLS              struct {
		Enabled  bool   `help:"Enable TLS" env:"ENABLED"`
		KeyFile  string `help:"Key file" env:"KEY" default:"/etc/tls/tls.key"`
		CertFile string `help:"Certificate file" env:"CERT" default:"/etc/tls/tls.crt"`
	} `embed:"" prefix:"tls-" envprefix:"MDWEB_TLS_"`
	OIDC struct {
		Enabled      bool   `help:"Enable OIDC" env:"ENABLED"`
		Issuer       string `help:"Issuer URL" env:"ISSUER"`
		ClientID     string `help:"Client ID" env:"CLIENT_ID"`
		ClientSecret string `help:"Client secret" env:"CLIENT_SECRET"`
		TrustProxy   bool   `name:"trust-proxy" env:"TRUST_PROXY" help:"Trust X-Forwarded-For from downstream proxies"`
	} `embed:"" prefix:"oidc-" envprefix:"MDWEB_OIDC_"`
}

// Run serves the markdown directory over HTTP: every page lives at its
// canonical directory (/page → /page/), matching the render command's output.
func (c *serveCmd) Run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	srv, err := c.newServer()
	if err != nil {
		slog.Error("failed to initialize service", "error", err)
		os.Exit(1)
	}

	var handler http.Handler = srv

	if c.OIDC.Enabled {

		auth, err := oidclogin.New(ctx, oidclogin.Config{
			IssuerURL:    c.OIDC.Issuer,
			ClientID:     c.OIDC.ClientID,
			ClientSecret: c.OIDC.ClientSecret,
			TrustProxy:   c.OIDC.TrustProxy,
			Logger: oidclogin.LoggerFunc(func(level oidclogin.Level, msg string) {
				switch level {
				case oidclogin.LogInfo:
					slog.Info("oidc login", "message", msg)
				case oidclogin.LogWarn:
					slog.Warn("oidc login", "message", msg)
				case oidclogin.LogError:
					slog.Error("oidc login", "message", msg)
				default:
					slog.Info("oidc login", "level", level, "message", msg)
				}
			}),
		})
		if err != nil {
			slog.Error("failed to initialize oidc login", "error", err)
			os.Exit(2)
		}
		handler = auth.Secure(handler)
		slog.Info("OIDC enabled", "issuer", c.OIDC.Issuer)
	}

	if !c.DisableGZIP {
		handler = gziphandler.GzipHandler(handler)
		slog.Info("gzip compression enabled")
	}

	httpServer := &http.Server{
		Addr:    c.Bind,
		Handler: handler,
	}

	go func() {
		<-ctx.Done()
		tCtx, tCancel := context.WithTimeout(context.Background(), c.GracefulShutdown)
		defer tCancel()
		if err := httpServer.Shutdown(tCtx); err != nil {
			slog.Error("failed to shutdown http server", "error", err)
		}
	}()

	slog.Info("ready")
	if c.TLS.Enabled {
		slog.Info("starting https server")
		err = httpServer.ListenAndServeTLS(c.TLS.CertFile, c.TLS.KeyFile)
	} else {
		slog.Info("starting http server")
		err = httpServer.ListenAndServe()
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("failed to start http server", "error", err)
		os.Exit(3)
	}
	return nil
}

// newServer wires the view layer with the HTTP wrapper: caching, redirects,
// and status codes are serve concerns; rendering lives in the view.
func (c *serveCmd) newServer() (*Server, error) {
	st, err := store.New(c.Data)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	v, err := view.New(view.Options{
		Listing: c.Listing,
		Nav:     !c.DisableNav,
		Title:   c.Title,
	})
	if err != nil {
		return nil, fmt.Errorf("build view: %w", err)
	}
	return &Server{
		view:        v,
		store:       st,
		enableCache: !c.DisableCache,
		rewriteHTML: c.HTMLRewrite,
	}, nil
}

// HTTP wrapper around the view: caching, canonical redirects, and status codes.
type Server struct {
	view        *view.View
	store       *store.Store
	cache       sync.Map // canonical URL path -> rendered page
	enableCache bool
	rewriteHTML bool
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path

	// canonical URLs: every page lives in its directory (/a → /a/); requests
	// with an extension (/a.md, /a.html) are direct file requests and exempt
	if !strings.HasSuffix(p, "/") && path.Ext(p) == "" {
		w.Header().Set("Location", "./"+path.Base(p)+"/")
		w.WriteHeader(http.StatusMovedPermanently)
		return
	}

	page, err := s.page(p)
	if err != nil {
		slog.Error("failed to render page", "path", p, "error", err)
		if errors.Is(err, os.ErrNotExist) {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	w.Header().Add("Content-Type", "text/html; charset=utf-8")
	w.Header().Add("Content-Length", strconv.Itoa(len(page)))
	_, _ = w.Write(page)
}

// page returns the rendered HTML for the canonical path p — from the cache
// when enabled, rendering and storing it otherwise. The error wraps
// os.ErrNotExist when p has no page.
func (s *Server) page(p string) ([]byte, error) {
	if content, ok := s.cache.Load(p); ok {
		return content.([]byte), nil
	}

	tree, err := s.store.Tree(store.WithoutHidden, store.Files("*.md"))
	if err != nil {
		return nil, fmt.Errorf("build tree: %w", err)
	}
	node := s.resolve(tree, p)
	if node == nil {
		return nil, fmt.Errorf("resolve %q: %w", p, os.ErrNotExist)
	}

	page, err := s.view.Render(node)
	if err != nil {
		return nil, fmt.Errorf("render %q: %w", p, err)
	}
	if s.enableCache {
		s.cache.Store(p, page)
	}
	return page, nil
}

// resolve maps a canonical request path to the node to render: a directory
// URL renders its index page, else the directory-named page, else the
// directory itself (whose rendering falls back to the listing). Direct
// /x.md and /x.html requests render their file.
func (s *Server) resolve(tree *store.Node, p string) *store.Node {
	if strings.HasSuffix(p, "/") {
		if idx := find(tree, path.Join(p, "index.md")); idx != nil {
			return idx
		}
		if p != "/" {
			if page := find(tree, strings.TrimSuffix(p, "/")+".md"); page != nil {
				return page
			}
		}
		return find(tree, strings.TrimSuffix(p, "/"))
	}
	if s.rewriteHTML && strings.HasSuffix(p, ".html") {
		p = p[:len(p)-len(".html")] + ".md"
	} else if !strings.HasSuffix(p, ".md") {
		p += ".md"
	}
	return find(tree, p)
}

// find returns the tree node at the store path, or nil.
func find(tree *store.Node, p string) *store.Node {
	return tree.FindFunc(func(n *store.Node) bool { return n.FullPath() == p })
}

var cli struct {
	Serve  serveCmd  `cmd:"" default:"withargs" help:"Serve the markdown directory over HTTP"`
	Render renderCmd `cmd:"" help:"Render self-contained static site into a directory"`
}

func main() {
	kongCtx := kong.Parse(&cli)
	if err := kongCtx.Run(); err != nil {
		slog.Error("command failed", "error", err)
		os.Exit(1)
	}
}
