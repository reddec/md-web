package store

import (
	"fmt"
	"path"
	"strings"
)

// Node is one entry of the store together with its position in the tree.
// Entry.Path holds the base name relative to the parent; FullPath resolves it
// against the ancestors. Children are ordered by name.
type Node struct {
	Entry
	Parent   *Node
	Children []*Node
}

// FullPath returns the logical path of the node from the root, for example
// "/docs/guide/readme.md". For the root it returns "/".
func (n *Node) FullPath() string {
	if n.Parent != nil {
		return path.Join(n.Parent.FullPath(), n.Entry.Path)
	}
	return n.Entry.Path
}

// Tree builds the whole store as a tree rooted at a synthetic directory with
// path "/" and a nil Parent. An entry joins the tree when every filter accepts
// it; a rejected directory is left out together with its whole subtree.
// Without filters every entry is kept.
//
//	root, err := st.Tree(store.WithoutHidden, store.Files("*.md"))
//
// Listing errors are wrapped together with the directory path.
func (s *Store) Tree(filters ...Filter) (*Node, error) {
	allFilters := And(filters...)
	var items = make([]*Node, 0, 1)
	root := &Node{
		Entry: Entry{
			Path:      "/",
			Directory: true,
		},
	}
	items = append(items, root)

	for len(items) > 0 {
		parent := items[len(items)-1]
		items = items[:len(items)-1] // cheap stack
		fullPath := parent.FullPath()

		entries, err := s.List(fullPath)

		if err != nil {
			return nil, fmt.Errorf("list %q: %w", fullPath, err)
		}

		for _, entry := range entries {
			if !allFilters(entry) {
				continue
			}
			item := &Node{
				Entry:  entry,
				Parent: parent,
			}
			parent.Children = append(parent.Children, item)
			if entry.Directory {
				items = append(items, item)
			}
		}
	}

	return root, nil
}

// Filter reports whether an entry is accepted into the tree.
type Filter func(entry Entry) bool

// WithoutHidden accepts entries whose base name does not start with ".". It
// applies to files and directories alike, so hidden directories are skipped
// with their contents.
func WithoutHidden(entry Entry) bool {
	return !strings.HasPrefix(entry.Path, ".")
}

// Files accepts every directory, so traversal continues, and files whose base
// name matches the glob pattern (path.Match syntax). A malformed pattern
// matches no files.
func Files(pattern string) Filter {
	return func(entry Entry) bool {
		if entry.Directory {
			return true
		}
		ok, _ := path.Match(pattern, entry.Path)
		return ok
	}
}

// And accepts an entry when every filter accepts it. Without filters it
// accepts everything.
func And(filters ...Filter) Filter {
	return func(entry Entry) bool {
		for _, filter := range filters {
			if !filter(entry) {
				return false
			}
		}
		return true
	}
}

// Or accepts an entry when at least one filter accepts it. Without filters it
// accepts nothing.
func Or(filters ...Filter) Filter {
	return func(entry Entry) bool {
		for _, filter := range filters {
			if filter(entry) {
				return true
			}
		}
		return false
	}
}
