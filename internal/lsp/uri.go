package lsp

import (
	"net/url"
	"strings"
)

// URIToPath converts a file:// URI to an absolute filesystem path.
// Handles URL-encoded characters (e.g. %20 for spaces).
func URIToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return strings.TrimPrefix(uri, "file://")
	}

	// url.Parse already decodes percent-encoding in Path.
	return u.Path
}

// PathToURI converts an absolute filesystem path to a file:// URI.
// It percent-encodes special characters so the result round-trips with URIToPath.
func PathToURI(path string) string {
	u := &url.URL{
		Scheme: "file",
		Path:   path,
	}
	return u.String()
}
