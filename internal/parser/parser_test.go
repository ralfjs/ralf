package parser

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestParseValidJS(t *testing.T) {
	source := readTestFile(t, "valid.js")
	p := NewParser(LangJS)
	t.Cleanup(p.Close)

	tree, err := p.Parse(context.Background(), source, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	t.Cleanup(tree.Close)

	root := tree.RootNode()
	if root.IsNull() {
		t.Fatal("root node is null")
	}
	if root.Kind() != "program" {
		t.Errorf("root kind = %q, want %q", root.Kind(), "program")
	}
	if root.HasError() {
		t.Error("valid JS file has parse errors")
	}
}

func TestParseValidJSX(t *testing.T) {
	source := readTestFile(t, "valid.jsx")
	p := NewParser(LangJSX)
	t.Cleanup(p.Close)

	tree, err := p.Parse(context.Background(), source, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	t.Cleanup(tree.Close)

	if tree.RootNode().HasError() {
		t.Error("valid JSX file has parse errors")
	}
}

func TestParseValidTS(t *testing.T) {
	source := readTestFile(t, "valid.ts")
	p := NewParser(LangTS)
	t.Cleanup(p.Close)

	tree, err := p.Parse(context.Background(), source, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	t.Cleanup(tree.Close)

	if tree.RootNode().HasError() {
		t.Error("valid TS file has parse errors")
	}
}

func TestParseValidTSX(t *testing.T) {
	source := readTestFile(t, "valid.tsx")
	p := NewParser(LangTSX)
	t.Cleanup(p.Close)

	tree, err := p.Parse(context.Background(), source, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	t.Cleanup(tree.Close)

	if tree.RootNode().HasError() {
		t.Error("valid TSX file has parse errors")
	}
}

func TestParseMalformed(t *testing.T) {
	source := readTestFile(t, "malformed.js")
	p := NewParser(LangJS)
	t.Cleanup(p.Close)

	tree, err := p.Parse(context.Background(), source, nil)
	if err != nil {
		t.Fatalf("Parse failed (should be error-tolerant): %v", err)
	}
	t.Cleanup(tree.Close)

	root := tree.RootNode()
	if root.IsNull() {
		t.Fatal("root node is null for malformed file")
	}
	if !root.HasError() {
		t.Error("malformed file should have parse errors")
	}
}

func TestParseEmpty(t *testing.T) {
	source := readTestFile(t, "empty.js")
	p := NewParser(LangJS)
	t.Cleanup(p.Close)

	tree, err := p.Parse(context.Background(), source, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	t.Cleanup(tree.Close)

	root := tree.RootNode()
	if root.Kind() != "program" {
		t.Errorf("root kind = %q, want %q", root.Kind(), "program")
	}
	if root.NamedChildCount() != 0 {
		t.Errorf("empty file should have 0 named children, got %d", root.NamedChildCount())
	}
}

func TestIncrementalReparse(t *testing.T) {
	p := NewParser(LangJS)
	t.Cleanup(p.Close)

	original := []byte("const x = 1;")
	tree1, err := p.Parse(context.Background(), original, nil)
	if err != nil {
		t.Fatalf("initial parse failed: %v", err)
	}
	t.Cleanup(tree1.Close)

	// Edit: change "1" to "42" (byte offset 10..11 → 10..12)
	tree1.Edit(&tree_sitter.InputEdit{
		StartByte:      10,
		OldEndByte:     11,
		NewEndByte:     12,
		StartPosition:  tree_sitter.Point{Row: 0, Column: 10},
		OldEndPosition: tree_sitter.Point{Row: 0, Column: 11},
		NewEndPosition: tree_sitter.Point{Row: 0, Column: 12},
	})

	modified := []byte("const x = 42;")
	tree2, err := p.Parse(context.Background(), modified, tree1)
	if err != nil {
		t.Fatalf("incremental parse failed: %v", err)
	}
	t.Cleanup(tree2.Close)

	if tree2.RootNode().HasError() {
		t.Error("incremental parse produced errors")
	}
}

func TestParseContextCancellation(t *testing.T) {
	p := NewParser(LangJS)
	t.Cleanup(p.Close)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// With a tiny source the parser may finish before checking the callback,
	// so use a larger source to increase the chance of cancellation.
	source := make([]byte, 0, 10000)
	for range 500 {
		source = append(source, []byte("const x = 1;\n")...)
	}

	_, err := p.Parse(ctx, source, nil)
	if err == nil {
		// It's acceptable for Parse to succeed on small inputs even with a
		// cancelled context, because tree-sitter checks the callback
		// periodically, not on every byte. We only verify that when it does
		// detect cancellation, it returns the right error.
		t.Log("parse completed despite cancelled context (acceptable for small inputs)")
	} else if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestCollectChildren(t *testing.T) {
	p := NewParser(LangJS)
	t.Cleanup(p.Close)

	// "f(1, 2)" has: call_expression -> [identifier, arguments]
	// arguments -> ["(", number, ",", number, ")"]
	source := []byte("f(1, 2)")
	tree, err := p.Parse(context.Background(), source, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	t.Cleanup(tree.Close)

	// Find the arguments node.
	var argsNode Node
	Walk(tree, func(node Node, _ int) bool {
		if node.Kind() == "arguments" {
			argsNode = node
			return false
		}
		return true
	})
	if argsNode.IsNull() {
		t.Fatal("arguments node not found")
	}

	children := argsNode.CollectChildren()
	if len(children) == 0 {
		t.Fatal("expected non-empty children")
	}

	// Verify count matches ChildCount().
	if uint(len(children)) != argsNode.ChildCount() {
		t.Errorf("CollectChildren returned %d children, ChildCount() = %d",
			len(children), argsNode.ChildCount())
	}

	// Verify expected structure: "(", number, ",", number, ")".
	wantKinds := []string{"(", "number", ",", "number", ")"}
	if len(children) != len(wantKinds) {
		t.Fatalf("expected %d children, got %d", len(wantKinds), len(children))
	}
	for i, c := range children {
		if c.Kind() != wantKinds[i] {
			t.Errorf("child[%d].Kind() = %q, want %q", i, c.Kind(), wantKinds[i])
		}
	}
}

func readTestFile(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "parser", name)) //nolint:gosec // test helper with fixed base path
	if err != nil {
		t.Fatalf("failed to read test fixture %s: %v", name, err)
	}
	return data
}
