package main

import (
	"bytes"
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
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/NYTimes/gziphandler"
	"github.com/alecthomas/kong"
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
	TLS              struct {
		Enabled  bool   `help:"Enable TLS" env:"ENABLED"`
		KeyFile  string `help:"Key file" env:"KEY" default:"/etc/tls/tls.key"`
		CertFile string `help:"Certificate file" env:"CERT" default:"/etc/tls/tls.crt"`
	} `embed:"" prefix:"tls-" envprefix:"MDWEB_TLS_"`
}

func main() {
	kong.Parse(&config)

	srv, err := newServer(config.Data, config.Base, config.Cache)
	if err != nil {
		slog.Error("failed to initialize service", "error", err)
		os.Exit(1)
	}

	var handler http.Handler = srv

	if !config.DisableGZIP {
		handler = gziphandler.GzipHandler(handler)
		slog.Info("gzip compression enabled")
	}

	httpServer := &http.Server{
		Addr:    config.Bind,
		Handler: handler,
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

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
		os.Exit(2)
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

func newServer(baseDir string, baseURL string, enableCache bool) (*Server, error) {
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

	rootDir, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, err
	}

	tpl, err := template.New("").Parse(layout)
	if err != nil {
		return nil, err
	}

	return &Server{
		baseDir:     rootDir,
		enableCache: enableCache,
		templ:       tpl,
		md:          md,
	}, nil
}

type Server struct {
	baseDir     string
	cache       sync.Map // string -> bytes
	enableCache bool
	templ       *template.Template
	md          goldmark.Markdown
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := path.Clean(r.URL.Path)
	if p == "" || p == "/" || strings.HasSuffix(r.URL.Path, "/") {
		p = path.Join(p, "index.md")
	} else if !strings.HasSuffix(p, ".md") {
		p += ".md"
	}

	var pageContent []byte
	if content, ok := s.cache.Load(p); ok {
		pageContent = content.([]byte)
	} else {
		page, err := s.getPage(p)
		if err != nil {
			slog.Error("failed to get page", "path", p, "error", err)
			if errors.Is(err, os.ErrNotExist) {
				w.WriteHeader(http.StatusNotFound)
			} else {
				w.WriteHeader(http.StatusInternalServerError)
			}
			return
		}
		if s.enableCache {
			s.cache.Store(p, page)
		}
		pageContent = page
	}

	w.Header().Add("Content-Type", "text/html; charset=utf-8")
	w.Header().Add("Content-Length", strconv.Itoa(len(pageContent)))
	_, _ = w.Write(pageContent)
}

func (s *Server) getPage(p string) ([]byte, error) {
	content, err := os.ReadFile(filepath.Join(s.baseDir, p))
	if err != nil {
		return nil, fmt.Errorf("read file %q: %w", p, err)
	}
	ctx := parser.NewContext()
	var output bytes.Buffer
	if err := s.md.Convert(content, &output, parser.WithContext(ctx)); err != nil {
		return nil, fmt.Errorf("convert file %q: %w", p, err)
	}

	title := strings.TrimSuffix(path.Base(p), ".md")

	page := &Page{
		Path:      p,
		Title:     title,
		Content:   template.HTML(output.String()),
		ShowTitle: config.Title,
	}
	if fm := frontmatter.Get(ctx); fm != nil {
		if err := fm.Decode(page); err != nil {
			// we can not fail here
			slog.Error("failed to decode frontmatter", "path", p, "error", err)
		}
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
