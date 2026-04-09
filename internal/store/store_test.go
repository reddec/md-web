package store_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/reddec/md-web/internal/store"
)

func TestNew(t *testing.T) {
	dir := t.TempDir()

	createFile(t, filepath.Join(dir, "a.md"), `---
title: hello
---
Hello world
`)
	const brokenContent = `It works?
It works!
`
	createFile(t, filepath.Join(dir, "b/broken.md"), brokenContent)
	testStore, err := store.New(dir)
	require.NoError(t, err)

	list, err := testStore.List("/")
	require.NoError(t, err)
	require.Equal(t, 2, len(list))

	assert.Equal(t, store.Entry{
		Path:      "a.md",
		Directory: false,
	}, list[0])
	assert.Equal(t, store.Entry{
		Path:      "b",
		Directory: true,
	}, list[1])

	// sub dir

	list, err = testStore.List("/b")
	require.NoError(t, err)
	require.Equal(t, 1, len(list))

	assert.Equal(t, store.Entry{
		Path:      "broken.md",
		Directory: false,
	}, list[0])

	// open

	doc, err := testStore.Open("/b/broken.md")
	require.NoError(t, err)
	assert.Equal(t, "", doc.Front().Title)
	val, err := doc.ReadString()
	require.NoError(t, err)
	assert.Equal(t, brokenContent, val)
	assert.NoError(t, doc.Close())
}

func createFile(t *testing.T, fPath string, content string) {
	t.Helper()
	pth := filepath.Join(fPath)
	err := os.MkdirAll(filepath.Dir(pth), 0755)
	require.NoError(t, err)
	err = os.WriteFile(pth, []byte(content), 0644)
	require.NoError(t, err)
}
