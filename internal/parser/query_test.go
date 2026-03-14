package parser

import (
	"context"
	"testing"
)

func TestQueryCompileAndExec(t *testing.T) {
	source := []byte(`
function greet(name) {
  return name;
}
function add(a, b) {
  return a + b;
}
`)
	p := NewParser(LangJS)
	t.Cleanup(p.Close)

	tree, err := p.Parse(context.Background(), source, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	t.Cleanup(tree.Close)

	// Query: match all function declarations and capture their name
	q, err := NewQuery(LangJS, `(function_declaration name: (identifier) @fn-name) @fn`)
	if err != nil {
		t.Fatalf("NewQuery failed: %v", err)
	}
	t.Cleanup(q.Close)

	matches := q.Exec(tree.RootNode(), source)

	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}

	// Each match should have 2 captures: @fn-name and @fn
	for i, m := range matches {
		if len(m.Captures) != 2 {
			t.Errorf("match %d: expected 2 captures, got %d", i, len(m.Captures))
		}
	}

	// Verify captures by name (capture order in the slice follows position)
	name1 := findCapture(t, matches[0], "fn-name")
	if name1.Node.Text(source) != "greet" {
		t.Errorf("first fn name = %q, want %q", name1.Node.Text(source), "greet")
	}

	fn1 := findCapture(t, matches[0], "fn")
	if fn1.Node.Kind() != "function_declaration" {
		t.Errorf("@fn node kind = %q, want %q", fn1.Node.Kind(), "function_declaration")
	}

	name2 := findCapture(t, matches[1], "fn-name")
	if name2.Node.Text(source) != "add" {
		t.Errorf("second fn name = %q, want %q", name2.Node.Text(source), "add")
	}
}

func findCapture(t *testing.T, m Match, name string) Capture {
	t.Helper()
	for _, c := range m.Captures {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("capture %q not found in match", name)
	return Capture{}
}

func TestQueryCompileError(t *testing.T) {
	_, err := NewQuery(LangJS, "(nonexistent_node_type)")
	if err == nil {
		t.Fatal("expected compile error for invalid node type")
	}
}

func TestQueryNoMatches(t *testing.T) {
	source := []byte("const x = 1;")
	p := NewParser(LangJS)
	t.Cleanup(p.Close)

	tree, err := p.Parse(context.Background(), source, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	t.Cleanup(tree.Close)

	q, err := NewQuery(LangJS, `(function_declaration) @fn`)
	if err != nil {
		t.Fatalf("NewQuery failed: %v", err)
	}
	t.Cleanup(q.Close)

	matches := q.Exec(tree.RootNode(), source)
	if len(matches) != 0 {
		t.Errorf("expected 0 matches, got %d", len(matches))
	}
}
