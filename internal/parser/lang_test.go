package parser

import "testing"

func TestLangFromPath(t *testing.T) {
	tests := []struct {
		path string
		want Lang
		ok   bool
	}{
		{"app.js", LangJS, true},
		{"lib.mjs", LangJS, true},
		{"lib.cjs", LangJS, true},
		{"component.jsx", LangJSX, true},
		{"util.ts", LangTS, true},
		{"util.mts", LangTS, true},
		{"util.cts", LangTS, true},
		{"page.tsx", LangTSX, true},
		{"dir/nested/file.ts", LangTS, true},
		{"FILE.JS", LangJS, true},
		{"style.css", 0, false},
		{"readme.md", 0, false},
		{"noext", 0, false},
		{"", 0, false},
	}

	for _, tt := range tests {
		got, ok := LangFromPath(tt.path)
		if ok != tt.ok {
			t.Errorf("LangFromPath(%q) ok = %v, want %v", tt.path, ok, tt.ok)
		}
		if ok && got != tt.want {
			t.Errorf("LangFromPath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestLangString(t *testing.T) {
	tests := []struct {
		lang Lang
		want string
	}{
		{LangJS, "JavaScript"},
		{LangJSX, "JSX"},
		{LangTS, "TypeScript"},
		{LangTSX, "TSX"},
	}

	for _, tt := range tests {
		if got := tt.lang.String(); got != tt.want {
			t.Errorf("Lang(%d).String() = %q, want %q", tt.lang, got, tt.want)
		}
	}
}

func TestLangLanguage(t *testing.T) {
	langs := []Lang{LangJS, LangJSX, LangTS, LangTSX}
	for _, l := range langs {
		grammar := l.Language()
		if grammar == nil {
			t.Errorf("Lang(%d).Language() returned nil", l)
		}
	}
}
