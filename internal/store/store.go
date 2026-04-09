package store

import (
	"os"
	"path"
	"path/filepath"
	"strings"
)

type Entry struct {
	Path      string
	Directory bool
}

type Store struct {
	baseDir string
}

func New(filePath string) (*Store, error) {
	f, err := filepath.Abs(filePath)
	if err != nil {
		return nil, err
	}
	return &Store{baseDir: f}, nil
}

func (s *Store) Open(fPath string) (*OpenedDocument, error) {
	if strings.HasSuffix(fPath, "/") {
		fPath += "index.md"
	} else if !strings.HasSuffix(fPath, ".md") {
		fPath = fPath + ".md"
	}
	pth := s.resolvePath(fPath)
	return OpenFile(pth)
}

func (s *Store) List(fPath string) ([]Entry, error) {
	realPath := s.resolvePath(fPath)
	stats, err := os.ReadDir(realPath)
	if err != nil {
		return nil, err
	}
	ans := make([]Entry, 0, len(stats))
	for _, stat := range stats {
		ans = append(ans, Entry{
			Path:      stat.Name(),
			Directory: stat.IsDir(),
		})
	}
	return ans, nil
}

func (s *Store) resolvePath(relPath string) string {
	return path.Join(s.baseDir, path.Clean(relPath))
}
