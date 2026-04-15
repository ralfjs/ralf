package lsp

import (
	"net/url"
	"path/filepath"
	"strings"
)

// URIToPath converts a file:// URI to an absolute filesystem path.
// Handles URL-encoded characters (e.g. %20 for spaces), Windows drive letters,
// and UNC paths.
func URIToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return strings.TrimPrefix(uri, "file://")
	}

	path := u.Path

	// UNC paths: file://server/share/... → //server/share/...
	if u.Host != "" {
		path = "//" + u.Host + path
	}

	// Windows drive letter: /C:/... → C:/...
	if len(path) >= 3 && path[0] == '/' && isLetter(path[1]) && path[2] == ':' {
		path = path[1:]
	}

	return filepath.FromSlash(path)
}

// PathToURI converts an absolute filesystem path to a file:// URI.
// It percent-encodes special characters, normalises path separators, and
// handles Windows drive letters and UNC paths so the result round-trips
// with URIToPath.
func PathToURI(path string) string {
	slashPath := filepath.ToSlash(path)

	// UNC path (starts with double slash) → file URI with host component.
	if strings.HasPrefix(slashPath, "//") {
		host, rest, _ := strings.Cut(slashPath[2:], "/")
		u := &url.URL{Scheme: "file", Host: host, Path: "/" + rest}
		return u.String()
	}

	// Windows drive letter: C:/... → /C:/... (URI path must be absolute).
	if len(slashPath) >= 2 && isLetter(slashPath[0]) && slashPath[1] == ':' {
		slashPath = "/" + slashPath
	}

	u := &url.URL{Scheme: "file", Path: slashPath}
	return u.String()
}

func isLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
