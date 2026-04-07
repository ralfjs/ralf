package lsp

import (
	"net/url"
	"strings"
)

// URIToPath converts a file:// URI to an absolute filesystem path.
func URIToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return strings.TrimPrefix(uri, "file://")
	}
	return u.Path
}
