package store_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/reddec/md-web/internal/store"
)

func TestOpen(t *testing.T) {

	t.Run("valid document", func(t *testing.T) {
		const validDoc = `---
title: "Hello World"
---
Is all ok?
`
		doc, err := store.Open(strings.NewReader(validDoc))
		require.NoError(t, err)
		content, err := doc.ReadString()
		require.NoError(t, err)

		assert.Equal(t, "Hello World", doc.Front().Title)
		assert.Equal(t, "Is all ok?", strings.TrimSpace(content))
	})

	t.Run("raw document", func(t *testing.T) {
		const validDoc = `


Is all ok?
`
		_, err := store.Open(strings.NewReader(validDoc))
		require.ErrorIs(t, err, store.ErrMalformedDocument)
	})

	t.Run("no new line front matter document", func(t *testing.T) {
		const validDoc = `---
title: "Hello World"
---
Is all ok?`
		doc, err := store.Open(strings.NewReader(validDoc))
		require.NoError(t, err)
		content, err := doc.ReadString()
		require.NoError(t, err)

		assert.Equal(t, "Hello World", doc.Front().Title)
		assert.Equal(t, "Is all ok?", strings.TrimSpace(content))
	})

	t.Run("malformed document - wrong first delimiter", func(t *testing.T) {
		const raw = `===
title: "Hello World"
---
Is all ok?`
		_, err := store.Open(strings.NewReader(raw))
		require.ErrorIs(t, err, store.ErrMalformedDocument)
	})

	t.Run("malformed document - no last delimiter", func(t *testing.T) {
		const raw = `---
title: "Hello World"
-=-
Is all ok?`
		_, err := store.Open(strings.NewReader(raw))
		require.ErrorIs(t, err, store.ErrMalformedDocument)
	})

	t.Run("malformed document - invalid yaml", func(t *testing.T) {
		const raw = `---
title: "Hello World"
wrong
---
Is all ok?`
		_, err := store.Open(strings.NewReader(raw))
		require.ErrorIs(t, err, store.ErrMalformedDocument)
	})
}

func TestOpenFile(t *testing.T) {
	// unlike Open, OpenFile retries malformed documents and treats them as raw files

	t.Run("valid document", func(t *testing.T) {
		const raw = `---
title: "Hello World"
---
Is all ok?
`
		file := saveFile(t, raw)
		doc, err := store.OpenFile(file)
		require.NoError(t, err)
		content, err := doc.ReadString()
		require.NoError(t, err)

		assert.Equal(t, "Hello World", doc.Front().Title)
		assert.Equal(t, "Is all ok?", strings.TrimSpace(content))
	})

	t.Run("raw document", func(t *testing.T) {
		const raw = `


Is all ok?
`
		file := saveFile(t, raw)
		doc, err := store.OpenFile(file)
		require.NoError(t, err)
		content, err := doc.ReadString()
		require.NoError(t, err)

		assert.Equal(t, "", doc.Front().Title)
		assert.Equal(t, "Is all ok?", strings.TrimSpace(content))
	})

	t.Run("no new line front matter document", func(t *testing.T) {
		const raw = `---
title: "Hello World"
---
Is all ok?`
		file := saveFile(t, raw)
		doc, err := store.OpenFile(file)
		require.NoError(t, err)
		content, err := doc.ReadString()
		require.NoError(t, err)

		assert.Equal(t, "Hello World", doc.Front().Title)
		assert.Equal(t, "Is all ok?", strings.TrimSpace(content))
	})

	t.Run("malformed document - wrong first delimiter", func(t *testing.T) {
		const raw = `===
title: "Hello World"
---
Is all ok?`
		file := saveFile(t, raw)
		doc, err := store.OpenFile(file)
		require.NoError(t, err)
		content, err := doc.ReadString()
		require.NoError(t, err)

		assert.Equal(t, "", doc.Front().Title)
		assert.Equal(t, raw, strings.TrimSpace(content))
	})

	t.Run("malformed document - no last delimiter", func(t *testing.T) {
		const raw = `---
title: "Hello World"
-=-
Is all ok?`
		file := saveFile(t, raw)
		doc, err := store.OpenFile(file)
		require.NoError(t, err)
		content, err := doc.ReadString()
		require.NoError(t, err)

		assert.Equal(t, "", doc.Front().Title)
		assert.Equal(t, raw, strings.TrimSpace(content))
	})

	t.Run("malformed document - invalid yaml", func(t *testing.T) {
		const raw = `---
title: "Hello World"
wrong
---
Is all ok?`
		file := saveFile(t, raw)
		doc, err := store.OpenFile(file)
		require.NoError(t, err)
		content, err := doc.ReadString()
		require.NoError(t, err)

		assert.Equal(t, "", doc.Front().Title)
		assert.Equal(t, raw, strings.TrimSpace(content))
	})

	// coverage happiness

	t.Run("when something goes wrong", func(t *testing.T) {
		const raw = `---
title: "Hello World"
wrong
---
Is all ok?`
		file := saveFile(t, raw)
		doc, err := store.OpenFile(file)
		require.NoError(t, err)

		// kill stream and expect best... foolish
		require.NoError(t, doc.Close())

		_, err = doc.ReadString()
		require.Error(t, err)
	})

}

func saveFile(t *testing.T, content string) string {
	t.Helper()
	pth := filepath.Join(t.TempDir(), "content.md")
	err := os.WriteFile(pth, []byte(content), 0644)
	require.NoError(t, err)
	return pth
}
