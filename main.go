package main

import (
	"bytes"
	_ "embed"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

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
	Bind  string `name:"bind" short:"b" env:"BIND" help:"Binding address" default:":8080"`
	Base  string `name:"base" short:"B" env:"BASE" help:"Base URL for links"`
	Data  string `name:"data" short:"d" env:"DATA" help:"Serving directory" default:"./"`
	Cache bool   `name:"cache" short:"c" env:"CACHE" help:"Enable caching"`
	Title bool   `name:"title" short:"t" env:"TITLE" help:"Show title from metadata or filepath"`
}

func main() {
	kong.Parse(&config)
	var cache sync.Map // string -> *Page

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
				util.Prioritized(&linkReWriter{basePath: []byte(config.Base)}, 100),
			),
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
		),
	)

	rootDir, err := filepath.Abs(config.Data)
	if err != nil {
		panic(err)
	}

	tpl := template.Must(template.New("").Parse(layout))

	http.HandleFunc("GET /", func(writer http.ResponseWriter, request *http.Request) {
		p := path.Clean(request.URL.Path)
		if p == "" || p == "/" || strings.HasSuffix(request.URL.Path, "/") {
			p = path.Join(p, "index.md")
		} else if !strings.HasSuffix(p, ".md") {
			p += ".md"
		}

		var page *Page
		if c, ok := cache.Load(p); ok {
			page = c.(*Page)
		} else {
			content, err := os.ReadFile(filepath.Join(rootDir, p))
			if errors.Is(err, os.ErrNotExist) {
				writer.WriteHeader(http.StatusNotFound)
				return
			}
			if err != nil {
				slog.Error("failed to open file", "path", p, "error", err)
				writer.WriteHeader(http.StatusInternalServerError)
				return
			}
			ctx := parser.NewContext()
			var output bytes.Buffer
			if err := md.Convert(content, &output, parser.WithContext(ctx)); err != nil {
				slog.Error("failed to convert file", "path", p, "error", err)
				writer.WriteHeader(http.StatusInternalServerError)
				return
			}

			title := strings.TrimSuffix(path.Base(p), ".md")

			page = &Page{
				Path:      p,
				Title:     title,
				Tags:      nil,
				Content:   template.HTML(output.String()),
				ShowTitle: config.Title,
			}
			if fm := frontmatter.Get(ctx); fm != nil {
				if err := fm.Decode(page); err != nil {
					slog.Error("failed to decode frontmatter", "path", p, "error", err)
				}
			}

			var buffer bytes.Buffer
			if err := tpl.Execute(&buffer, page); err != nil {
				slog.Error("failed to execute template", "path", p, "error", err)
				writer.WriteHeader(http.StatusInternalServerError)
				return
			}

			page.Rendered = buffer.Bytes()

			if config.Cache {
				cache.Store(p, page)
			}
		}

		writer.Header().Add("Content-Type", "text/html; charset=utf-8")
		writer.Header().Add("Content-Length", strconv.Itoa(len(page.Rendered)))
		_, _ = writer.Write(page.Rendered)
	})

	slog.Info("ready")
	http.ListenAndServe(config.Bind, nil)
}

type Page struct {
	Path      string        `yaml:"-"`
	Title     string        `yaml:"title"`
	Tags      []string      `yaml:"tags"`
	Content   template.HTML `yaml:"-"`
	Rendered  []byte        `yaml:"-"`
	ShowTitle bool          `yaml:"-"`
	//CreatedAt time.Time
	//UpdatedAt time.Time
}

type linkReWriter struct {
	basePath []byte
}

func (r *linkReWriter) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
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
