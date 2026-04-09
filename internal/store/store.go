package store

import (
	"os"
	"path"
	"path/filepath"
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

func (s *Store) IsDir(fPath string) bool {
	realPath := s.resolvePath(fPath)
	info, err := os.Stat(realPath)
	return err == nil && info.IsDir()
}

func (s *Store) IsFile(fPath string) bool {
	realPath := s.resolvePath(fPath)
	info, err := os.Stat(realPath)
	return err == nil && !info.IsDir()
}

func (s *Store) IsRoot(fPath string) bool {
	realPath := s.resolvePath(fPath)
	return realPath == s.baseDir
}

func (s *Store) resolvePath(relPath string) string {
	return path.Join(s.baseDir, path.Clean(relPath))
}
