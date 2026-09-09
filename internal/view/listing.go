package view

import (
	"strings"

	"github.com/reddec/md-web/internal/store"
)

// renderListing builds the markdown of a directory listing page; it runs
// through the regular markdown pipeline, so nav and link rewriting apply to
// it like to any page. Hidden (dot-prefixed) entries are excluded.
func renderListing(hasParent bool, list []store.Entry) string {
	var buffer strings.Builder

	if hasParent {
		buffer.WriteString("- [⤴️ ..](../)\n")
	}

	for _, entry := range list {
		if strings.HasPrefix(entry.Path, ".") {
			continue
		}
		if !entry.Directory && !strings.HasSuffix(entry.Path, ".md") {
			continue
		}
		buffer.WriteString("- [")
		if entry.Directory {
			buffer.WriteString("🗀 ")
		} else {
			buffer.WriteString("\U0001F5B9 ")
		}
		buffer.WriteString(entry.Path)
		buffer.WriteString("](")
		buffer.WriteString(entry.Path)
		buffer.WriteString(")\n")
	}
	return buffer.String()
}
