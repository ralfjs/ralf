package parser

import (
	"context"
	"testing"
)

func TestWalkCollectsNodeKinds(t *testing.T) {
	source := []byte("const x = 1;")
	p := NewParser(LangJS)
	t.Cleanup(p.Close)

	tree, err := p.Parse(context.Background(), source, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	t.Cleanup(tree.Close)

	var kinds []string
	Walk(tree, func(node Node, _ int) bool {
		kinds = append(kinds, node.Kind())
		return true
	})

	if len(kinds) == 0 {
		t.Fatal("Walk collected no nodes")
	}
	if kinds[0] != "program" {
		t.Errorf("first node kind = %q, want %q", kinds[0], "program")
	}

	// Verify anonymous nodes are included (e.g. "const", "=", ";")
	hasAnonymous := false
	for _, k := range kinds {
		if k == "const" || k == "=" || k == ";" {
			hasAnonymous = true
			break
		}
	}
	if !hasAnonymous {
		t.Error("Walk should include anonymous nodes")
	}
}

func TestWalkNamedSkipsAnonymous(t *testing.T) {
	source := []byte("const x = 1;")
	p := NewParser(LangJS)
	t.Cleanup(p.Close)

	tree, err := p.Parse(context.Background(), source, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	t.Cleanup(tree.Close)

	var kinds []string
	WalkNamed(tree, func(node Node, _ int) bool {
		kinds = append(kinds, node.Kind())
		return true
	})

	for _, k := range kinds {
		if k == "const" || k == "=" || k == ";" {
			t.Errorf("WalkNamed should skip anonymous node %q", k)
		}
	}

	if len(kinds) == 0 {
		t.Fatal("WalkNamed collected no nodes")
	}
}

func TestWalkSkipChildren(t *testing.T) {
	source := []byte("function foo() { return 1; }")
	p := NewParser(LangJS)
	t.Cleanup(p.Close)

	tree, err := p.Parse(context.Background(), source, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	t.Cleanup(tree.Close)

	var kinds []string
	WalkNamed(tree, func(node Node, _ int) bool {
		kinds = append(kinds, node.Kind())
		// Don't descend into the statement_block
		return node.Kind() != "statement_block"
	})

	for _, k := range kinds {
		if k == "return_statement" || k == "number" {
			t.Errorf("should have skipped children of statement_block, found %q", k)
		}
	}
}
