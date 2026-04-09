package store

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

var ErrMalformedDocument = errors.New("malformed document")

type Frontmatter struct {
	Title string   `yaml:"title"`
	Tags  []string `yaml:"tags"`
}

type Document struct {
	data  *bufio.Reader
	front Frontmatter
}

type OpenedDocument struct {
	*Document
	io.Closer
}

func Virtual(front Frontmatter, content string) *Document {
	return &Document{
		data:  bufio.NewReader(strings.NewReader(content)),
		front: front,
	}
}

// OpenFile parses markdown file in two phases: as frontmatter, then as raw file.
// Returned document must be closed by caller.
func OpenFile(path string) (*OpenedDocument, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	doc, err := Open(f)
	if err != nil {

		if errors.Is(err, ErrMalformedDocument) {
			// treat it as raw
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				_ = f.Close()
				return nil, err
			}

			return &OpenedDocument{&Document{
				data: bufio.NewReader(f),
			}, f}, nil
		}

		_ = f.Close()
		return nil, err
	}
	return &OpenedDocument{doc, f}, nil
}

func Open(src io.Reader) (*Document, error) {
	var front Frontmatter
	body := bufio.NewReader(src)

	data, err := body.ReadString('\n')
	if err != nil {
		if errors.Is(err, io.EOF) {
			// too short
			return nil, ErrMalformedDocument
		}
		return nil, fmt.Errorf("peek: %w", err)
	}

	if string(data) != "---\n" {
		return nil, ErrMalformedDocument
	}

	var header bytes.Buffer
	for {
		content, err := body.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				// no last delimiter - raw doc
				return nil, ErrMalformedDocument
			}
			return nil, fmt.Errorf("read: %w", err)
		}

		if string(content) == "---\n" {
			break
		}
		header.Write(content)
	}

	if err := yaml.Unmarshal(header.Bytes(), &front); err != nil {
		return nil, ErrMalformedDocument
	}

	return &Document{data: body, front: front}, nil
}

func (doc *Document) Front() Frontmatter {
	return doc.front
}

func (doc *Document) Data() *bufio.Reader {
	return doc.data
}

func (doc *Document) ReadString() (string, error) {
	v, err := io.ReadAll(doc.Data())

	if err != nil {
		return "", err
	}
	return string(v), nil
}

func (doc *Document) ReadBytes() ([]byte, error) {
	return io.ReadAll(doc.data)
}
