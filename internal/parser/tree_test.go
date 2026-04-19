package parser

import (
	"context"
	"testing"
)

func TestNamedDescendantForByteRange(t *testing.T) {
	t.Parallel()

	source := []byte("const foo = 1;\nconst bar = foo + 2;")
	p := NewParser(LangJS)
	t.Cleanup(p.Close)
	tree, err := p.Parse(context.Background(), source, nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	t.Cleanup(tree.Close)
	root := tree.RootNode()

	tests := []struct {
		name     string
		start    uint
		end      uint
		wantKind string
		wantText string
	}{
		{"whole program", 0, uint(len(source)), "program", string(source)},
		{"identifier foo def", 6, 9, "identifier", "foo"},
		{"number literal 1", 12, 13, "number", "1"},
		{"identifier bar", 21, 24, "identifier", "bar"},
		{"identifier foo use", 27, 30, "identifier", "foo"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			n := root.NamedDescendantForByteRange(tc.start, tc.end)
			if n.IsNull() {
				t.Fatalf("got null node at [%d,%d]", tc.start, tc.end)
			}
			if got := n.Kind(); got != tc.wantKind {
				t.Errorf("kind: got %q, want %q", got, tc.wantKind)
			}
			if got := n.Text(source); got != tc.wantText {
				t.Errorf("text: got %q, want %q", got, tc.wantText)
			}
		})
	}
}

func TestNamedDescendantForByteRange_PointQuery(t *testing.T) {
	t.Parallel()

	source := []byte("const identifier = value;")
	p := NewParser(LangJS)
	t.Cleanup(p.Close)
	tree, err := p.Parse(context.Background(), source, nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	t.Cleanup(tree.Close)
	root := tree.RootNode()

	// Point query (start == end) at byte 8 — inside "identifier".
	n := root.NamedDescendantForByteRange(8, 8)
	if n.IsNull() {
		t.Fatal("point query returned null node")
	}
	if got := n.Text(source); got != "identifier" {
		t.Errorf("point query: got %q, want %q", got, "identifier")
	}
}
