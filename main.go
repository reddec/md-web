package main

import (
	"bytes"
	"cmp"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
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
	treeblood "github.com/wyatt915/goldmark-treeblood"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"go.abhg.dev/goldmark/frontmatter"
	"go.abhg.dev/goldmark/mermaid"
	"go.abhg.dev/goldmark/wikilink"

	"github.com/reddec/md-web/internal/store"
)

//go:embed layout.gohtml
var layout string

var config struct {
	Bind             string        `name:"bind" short:"b" env:"MDWEB_BIND" help:"Binding address" default:":8080"`
	GracefulShutdown time.Duration `name:"graceful-shutdown" env:"MDWEB_GRACEFUL_SHUTDOWN" help:"Graceful shutdown timeout for server" default:"10s"`
	Base             string        `name:"base" short:"B" env:"MDWEB_BASE" help:"Base URL for links"`
	Data             string        `name:"data" short:"d" env:"MDWEB_DATA" help:"Serving directory" default:"./"`
	Cache            bool          `name:"cache" short:"c" env:"MDWEB_CACHE" help:"Enable caching"`
	Title            bool          `name:"title" short:"t" env:"MDWEB_TITLE" help:"Show title from metadata or filepath"`
	DisableGZIP      bool          `help:"Disable gzip compression for HTTP" env:"MDWEB_DISABLE_GZIP"`
	HTMLRewrite      bool          `name:"html-rewrite" env:"MDWEB_HTML_REWRITE" help:"Re-write .html to .md"`
	Listing          bool          `name:"listing" short:"l" env:"MDWEB_LISTING" help:"Enable directory listing if no index.md there" `
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

func main() {
	kong.Parse(&config)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	srv, err := newServer(config.Data, config.Base, config.Cache, config.Listing, config.HTMLRewrite)
	if err != nil {
		slog.Error("failed to initialize service", "error", err)
		os.Exit(1)
	}

	var handler http.Handler = srv

	if config.OIDC.Enabled {

		auth, err := oidclogin.New(ctx, oidclogin.Config{
			IssuerURL:    config.OIDC.Issuer,
			ClientID:     config.OIDC.ClientID,
			ClientSecret: config.OIDC.ClientSecret,
			TrustProxy:   config.OIDC.TrustProxy,
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
		slog.Info("OIDC enabled", "issuer", config.OIDC.Issuer)
	}

	if !config.DisableGZIP {
		handler = gziphandler.GzipHandler(handler)
		slog.Info("gzip compression enabled")
	}

	httpServer := &http.Server{
		Addr:    config.Bind,
		Handler: handler,
	}

	go func() {
		<-ctx.Done()
		tCtx, tCancel := context.WithTimeout(context.Background(), config.GracefulShutdown)
		defer tCancel()
		if err := httpServer.Shutdown(tCtx); err != nil {
			slog.Error("failed to shutdown http server", "error", err)
		}
	}()

	slog.Info("ready")
	if config.TLS.Enabled {
		slog.Info("starting https server")
		err = httpServer.ListenAndServeTLS(config.TLS.CertFile, config.TLS.KeyFile)
	} else {
		slog.Info("starting http server")
		err = httpServer.ListenAndServe()
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("failed to start http server", "error", err)
		os.Exit(3)
	}
}

type Page struct {
	Path      string        `yaml:"-"`
	Title     string        `yaml:"title"`
	Tags      []string      `yaml:"tags"`
	Content   template.HTML `yaml:"-"`
	ShowTitle bool          `yaml:"-"`
	//CreatedAt time.Time
	//UpdatedAt time.Time
}

func newServer(baseDir string, baseURL string, enableCache, enableListing, rewriteHTML bool) (*Server, error) {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			&wikilink.Extender{},
			treeblood.MathML(),
			&mermaid.Extender{},
			&frontmatter.Extender{},
			highlighting.Highlighting,
		),
		goldmark.WithParserOptions(
			parser.WithASTTransformers(
				util.Prioritized(&linkReWriter{basePath: []byte(baseURL)}, 100),
			),
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
		),
	)

	storage, err := store.New(baseDir)
	if err != nil {
		return nil, err
	}

	tpl, err := template.New("").Parse(layout)
	if err != nil {
		return nil, err
	}

	return &Server{
		storage:       storage,
		enableCache:   enableCache,
		enableListing: enableListing,
		rewriteHTML:   rewriteHTML,
		templ:         tpl,
		md:            md,
	}, nil
}

type Server struct {
	storage       *store.Store
	cache         sync.Map // string -> bytes
	enableCache   bool
	enableListing bool
	rewriteHTML   bool
	templ         *template.Template
	md            goldmark.Markdown
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const HTML = ".html"

	var pageContent []byte
	if content, ok := s.cache.Load(r.URL.Path); ok {
		pageContent = content.([]byte)
	} else {
		var err error
		var page []byte
		if s.enableListing && s.storage.IsDir(r.URL.Path) {
			if !strings.HasSuffix(r.URL.Path, "/") {
				// redirect to `./`
				w.Header().Set("Location", "./"+path.Base(r.URL.Path)+"/")
				w.WriteHeader(http.StatusFound)
				return
			}

			if p := path.Join(r.URL.Path, "index.md"); s.storage.IsFile(p) {
				page, err = s.getPage(p)
			} else {
				page, err = s.getDirectory(r.URL.Path)
			}
		} else {
			p := r.URL.Path
			if strings.HasSuffix(r.URL.Path, "/") {
				p += "index.md"
			} else if s.rewriteHTML && strings.HasSuffix(r.URL.Path, HTML) {
				p = p[:len(p)-len(HTML)] + ".md"
			} else if !strings.HasSuffix(r.URL.Path, ".md") {
				p = p + ".md"
			}

			page, err = s.getPage(p)
		}

		if err != nil {
			slog.Error("failed to get page", "path", r.URL.Path, "error", err)
			if errors.Is(err, os.ErrNotExist) {
				w.WriteHeader(http.StatusNotFound)
			} else {
				w.WriteHeader(http.StatusInternalServerError)
			}
			return
		}
		if s.enableCache {
			s.cache.Store(r.URL.Path, page)
		}
		pageContent = page
	}

	w.Header().Add("Content-Type", "text/html; charset=utf-8")
	w.Header().Add("Content-Length", strconv.Itoa(len(pageContent)))
	_, _ = w.Write(pageContent)
}

func (s *Server) getPage(p string) ([]byte, error) {
	doc, err := s.storage.Open(p)
	if err != nil {

		return nil, fmt.Errorf("open page %q: %w", p, err)

	}
	defer doc.Close()
	content, err := doc.ReadBytes()
	if err != nil {
		return nil, fmt.Errorf("read file %q: %w", p, err)
	}
	var output bytes.Buffer
	if err := s.md.Convert(content, &output); err != nil {
		return nil, fmt.Errorf("convert file %q: %w", p, err)
	}

	title := strings.TrimSuffix(path.Base(p), ".md")

	page := &Page{
		Path:      p,
		Title:     cmp.Or(doc.Front().Title, title),
		Content:   template.HTML(output.String()),
		ShowTitle: config.Title,
		Tags:      doc.Front().Tags,
	}

	var buffer bytes.Buffer
	if err := s.templ.Execute(&buffer, page); err != nil {
		return nil, fmt.Errorf("execute layout %q: %w", p, err)
	}

	return buffer.Bytes(), nil
}

func (s *Server) getDirectory(p string) ([]byte, error) {
	list, err := s.storage.List(p)
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	var output bytes.Buffer
	if err := s.md.Convert([]byte(renderListing(!s.storage.IsRoot(p), list)), &output); err != nil {
		return nil, fmt.Errorf("convert file %q: %w", p, err)
	}

	page := &Page{
		Path:      p,
		Title:     path.Base(p) + "/",
		Content:   template.HTML(output.String()),
		ShowTitle: config.Title,
	}

	var buffer bytes.Buffer
	if err := s.templ.Execute(&buffer, page); err != nil {
		return nil, fmt.Errorf("execute layout %q: %w", p, err)
	}

	return buffer.Bytes(), nil
}

type linkReWriter struct {
	basePath []byte
}

func (r *linkReWriter) Transform(node *ast.Document, _ text.Reader, _ parser.Context) {
	if len(r.basePath) == 0 {
		return
	}
	ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *ast.Link:
			dest := v.Destination
			if bytes.HasSuffix(dest, []byte{'/'}) {
				v.Destination = append(r.basePath, dest...)
			}
		case *ast.Image:
			dest := v.Destination
			if bytes.HasSuffix(dest, []byte{'/'}) {
				v.Destination = append(r.basePath, dest...)
			}
		}
		return ast.WalkContinue, nil
	})
}

func renderListing(hasParent bool, list []store.Entry) string {
	var buffer strings.Builder

	if hasParent {
		buffer.WriteString("- [⤴️ ..](../)\n")
	}

	for _, entry := range list {
		if !entry.Directory && !strings.HasSuffix(entry.Path, ".md") {
			continue
		}
		buffer.WriteString("- [")
		if entry.Directory {
			buffer.WriteString("🗀 ")
		} else {
			buffer.WriteString("🖹 ")
		}
		buffer.WriteString(entry.Path)
		buffer.WriteString("](")
		buffer.WriteString(entry.Path)
		buffer.WriteString(")\n")
	}
	return buffer.String()
}
