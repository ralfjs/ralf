package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/Hideart/ralf/internal/config"
	"github.com/Hideart/ralf/internal/parser"
)

func TestCompileStructuralRules(t *testing.T) {
	t.Run("valid AST rule", func(t *testing.T) {
		t.Parallel()
		rules := map[string]config.RuleConfig{
			"no-nested-fn": {
				Severity: config.SeverityError,
				AST:      &config.ASTMatcher{Kind: "function_declaration", Parent: &config.ASTMatcher{Kind: "function_declaration"}},
				Message:  "No nested functions",
			},
		}
		compiled, errs := compileStructuralRules(rules)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if len(compiled) != 1 {
			t.Fatalf("expected 1 compiled rule, got %d", len(compiled))
		}
		if compiled[0].name != "no-nested-fn" {
			t.Errorf("name = %q, want %q", compiled[0].name, "no-nested-fn")
		}
	})

	t.Run("skips off severity", func(t *testing.T) {
		t.Parallel()
		rules := map[string]config.RuleConfig{
			"off-rule": {
				Severity: config.SeverityOff,
				AST:      &config.ASTMatcher{Kind: "function_declaration"},
				Message:  "Should be skipped",
			},
		}
		compiled, errs := compileStructuralRules(rules)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if len(compiled) != 0 {
			t.Fatalf("expected 0 compiled rules, got %d", len(compiled))
		}
	})

	t.Run("skips non-AST rules", func(t *testing.T) {
		t.Parallel()
		rules := map[string]config.RuleConfig{
			"regex-rule": {
				Severity: config.SeverityError,
				Regex:    `\bvar\b`,
				Message:  "No var",
			},
		}
		compiled, errs := compileStructuralRules(rules)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if len(compiled) != 0 {
			t.Fatalf("expected 0 compiled rules, got %d", len(compiled))
		}
	})

	t.Run("invalid name type returns error", func(t *testing.T) {
		t.Parallel()
		rules := map[string]config.RuleConfig{
			"bad-name": {
				Severity: config.SeverityError,
				AST:      &config.ASTMatcher{Kind: "identifier", Name: 42},
				Message:  "Bad",
			},
		}
		_, errs := compileStructuralRules(rules)
		if len(errs) != 1 {
			t.Fatalf("expected 1 error, got %d", len(errs))
		}
		if !errors.Is(errs[0], ErrStructuralCompile) {
			t.Errorf("error should wrap ErrStructuralCompile, got: %v", errs[0])
		}
	})
}

func TestCompileASTMatcher(t *testing.T) {
	t.Run("kind only", func(t *testing.T) {
		t.Parallel()
		m, err := compileASTMatcher("test", &config.ASTMatcher{Kind: "function_declaration"}, 0)
		if err != nil {
			t.Fatal(err)
		}
		if m.kind != "function_declaration" {
			t.Errorf("kind = %q, want %q", m.kind, "function_declaration")
		}
		if m.name != nil || m.parent != nil || m.not != nil {
			t.Error("expected no name/parent/not constraints")
		}
	})

	t.Run("name exact", func(t *testing.T) {
		t.Parallel()
		m, err := compileASTMatcher("test", &config.ASTMatcher{Name: "foo"}, 0)
		if err != nil {
			t.Fatal(err)
		}
		if m.name == nil {
			t.Fatal("name should be set")
		}
		if m.name.exact != "foo" {
			t.Errorf("exact = %q, want %q", m.name.exact, "foo")
		}
	})

	t.Run("name regex", func(t *testing.T) {
		t.Parallel()
		m, err := compileASTMatcher("test", &config.ASTMatcher{Name: "/^debug/"}, 0)
		if err != nil {
			t.Fatal(err)
		}
		if m.name == nil || m.name.re == nil {
			t.Fatal("name regex should be set")
		}
		if m.name.exact != "" {
			t.Error("exact should be empty for regex")
		}
	})

	t.Run("name invalid regex", func(t *testing.T) {
		t.Parallel()
		_, err := compileASTMatcher("test", &config.ASTMatcher{Name: "/[invalid/"}, 0)
		if err == nil {
			t.Fatal("expected error for invalid regex")
		}
		if !errors.Is(err, ErrStructuralCompile) {
			t.Errorf("error should wrap ErrStructuralCompile, got: %v", err)
		}
	})

	t.Run("name invalid type", func(t *testing.T) {
		t.Parallel()
		_, err := compileASTMatcher("test", &config.ASTMatcher{Name: true}, 0)
		if err == nil {
			t.Fatal("expected error for non-string name")
		}
	})

	t.Run("parent constraint", func(t *testing.T) {
		t.Parallel()
		m, err := compileASTMatcher("test", &config.ASTMatcher{
			Kind:   "function_declaration",
			Parent: &config.ASTMatcher{Kind: "class_body"},
		}, 0)
		if err != nil {
			t.Fatal(err)
		}
		if m.parent == nil {
			t.Fatal("parent should be set")
		}
		if m.parent.kind != "class_body" {
			t.Errorf("parent.kind = %q, want %q", m.parent.kind, "class_body")
		}
	})

	t.Run("not constraint", func(t *testing.T) {
		t.Parallel()
		m, err := compileASTMatcher("test", &config.ASTMatcher{
			Kind: "function_declaration",
			Not:  &config.ASTMatcher{Kind: "function_declaration"},
		}, 0)
		if err != nil {
			t.Fatal(err)
		}
		if m.not == nil {
			t.Fatal("not should be set")
		}
	})

	t.Run("depth limit", func(t *testing.T) {
		t.Parallel()
		// Build a deeply nested parent chain.
		ast := &config.ASTMatcher{Kind: "a"}
		for range maxASTMatcherDepth {
			ast = &config.ASTMatcher{Kind: "a", Parent: ast}
		}
		_, err := compileASTMatcher("test", ast, 0)
		if err == nil {
			t.Fatal("expected error for excessive depth")
		}
		if !errors.Is(err, ErrStructuralCompile) {
			t.Errorf("error should wrap ErrStructuralCompile, got: %v", err)
		}
	})
}

func TestMatchASTNode(t *testing.T) {
	// Helper to parse JS and get a specific node.
	parseAndFindNode := func(t *testing.T, src, kind string) (parser.Node, *parser.Tree, []byte) {
		t.Helper()
		source := []byte(src)
		p := parser.NewParser(parser.LangJS)
		tree, err := p.Parse(context.Background(), source, nil)
		p.Close()
		if err != nil {
			t.Fatal(err)
		}
		var found parser.Node
		parser.WalkNamed(tree, func(node parser.Node, _ int) bool {
			if node.Kind() == kind && found.IsNull() {
				found = node
				return false
			}
			return true
		})
		if found.IsNull() {
			t.Fatalf("could not find node of kind %q in %q", kind, src)
		}
		return found, tree, source
	}

	// resolveIDs resolves kind strings to symbol IDs using the given node's grammar.
	resolveIDs := func(root parser.Node, m *compiledASTMatcher) {
		resolveMatcherSymbolIDs(root, m)
	}

	t.Run("kind match", func(t *testing.T) {
		node, tree, source := parseAndFindNode(t, "function foo() {}", "function_declaration")
		defer tree.Close()
		m := compiledASTMatcher{kind: "function_declaration"}
		resolveIDs(tree.RootNode(), &m)
		if !matchASTNode(&m, node, node.KindID(), source, 0) {
			t.Error("expected match")
		}
	})

	t.Run("kind mismatch", func(t *testing.T) {
		node, tree, source := parseAndFindNode(t, "function foo() {}", "function_declaration")
		defer tree.Close()
		m := compiledASTMatcher{kind: "class_declaration"}
		resolveIDs(tree.RootNode(), &m)
		if matchASTNode(&m, node, node.KindID(), source, 0) {
			t.Error("expected no match")
		}
	})

	t.Run("name exact match", func(t *testing.T) {
		node, tree, source := parseAndFindNode(t, "function foo() {}", "function_declaration")
		defer tree.Close()
		m := compiledASTMatcher{name: &compiledNameMatch{exact: "foo"}}
		if !matchASTNode(&m, node, node.KindID(), source, 0) {
			t.Error("expected match")
		}
	})

	t.Run("name exact mismatch", func(t *testing.T) {
		node, tree, source := parseAndFindNode(t, "function foo() {}", "function_declaration")
		defer tree.Close()
		m := compiledASTMatcher{name: &compiledNameMatch{exact: "bar"}}
		if matchASTNode(&m, node, node.KindID(), source, 0) {
			t.Error("expected no match")
		}
	})

	t.Run("name regex match", func(t *testing.T) {
		node, tree, source := parseAndFindNode(t, "function debugFoo() {}", "function_declaration")
		defer tree.Close()
		re, _ := compileNameMatch("test", "/^debug/")
		m := compiledASTMatcher{name: &re}
		if !matchASTNode(&m, node, node.KindID(), source, 0) {
			t.Error("expected match")
		}
	})

	t.Run("name regex mismatch", func(t *testing.T) {
		node, tree, source := parseAndFindNode(t, "function foo() {}", "function_declaration")
		defer tree.Close()
		re, _ := compileNameMatch("test", "/^debug/")
		m := compiledASTMatcher{name: &re}
		if matchASTNode(&m, node, node.KindID(), source, 0) {
			t.Error("expected no match")
		}
	})

	t.Run("parent constraint", func(t *testing.T) {
		// Nested function: inner function_declaration has parent statement_block
		// whose parent is function_declaration.
		source := []byte("function outer() { function inner() {} }")
		p := parser.NewParser(parser.LangJS)
		tree, err := p.Parse(context.Background(), source, nil)
		p.Close()
		if err != nil {
			t.Fatal(err)
		}
		defer tree.Close()

		// Find the inner function_declaration (named "inner").
		var inner parser.Node
		parser.WalkNamed(tree, func(node parser.Node, _ int) bool {
			if node.Kind() == "function_declaration" {
				nameChild := node.ChildByFieldName("name")
				if !nameChild.IsNull() && nameChild.Text(source) == "inner" {
					inner = node
					return false
				}
			}
			return true
		})
		if inner.IsNull() {
			t.Fatal("could not find inner function")
		}

		// Match: parent chain must include a function_declaration.
		// inner's parent is statement_block, its parent is function_declaration.
		// We use parent->parent to skip statement_block.
		m := compiledASTMatcher{
			kind: "function_declaration",
			parent: &compiledASTMatcher{
				parent: &compiledASTMatcher{kind: "function_declaration"},
			},
		}
		resolveIDs(tree.RootNode(), &m)
		if !matchASTNode(&m, inner, inner.KindID(), source, 0) {
			t.Error("expected match for nested function with parent constraint")
		}
	})

	t.Run("not negation", func(t *testing.T) {
		node, tree, source := parseAndFindNode(t, "function foo() {}", "function_declaration")
		defer tree.Close()

		// Not with matching kind should negate.
		m := compiledASTMatcher{not: &compiledASTMatcher{kind: "function_declaration"}}
		resolveIDs(tree.RootNode(), &m)
		if matchASTNode(&m, node, node.KindID(), source, 0) {
			t.Error("expected no match due to negation")
		}

		// Not with non-matching kind should pass.
		m2 := compiledASTMatcher{not: &compiledASTMatcher{kind: "class_declaration"}}
		resolveIDs(tree.RootNode(), &m2)
		if !matchASTNode(&m2, node, node.KindID(), source, 0) {
			t.Error("expected match — negated condition doesn't hold")
		}
	})

	t.Run("combined kind + name + not", func(t *testing.T) {
		node, tree, source := parseAndFindNode(t, "function foo() {}", "function_declaration")
		defer tree.Close()

		m := compiledASTMatcher{
			kind: "function_declaration",
			name: &compiledNameMatch{exact: "foo"},
			not:  &compiledASTMatcher{kind: "class_declaration"},
		}
		resolveIDs(tree.RootNode(), &m)
		if !matchASTNode(&m, node, node.KindID(), source, 0) {
			t.Error("expected match for combined constraints")
		}
	})

	t.Run("null node returns false", func(t *testing.T) {
		m := compiledASTMatcher{kind: "function_declaration", kindID: 1} // non-zero kindID
		if matchASTNode(&m, parser.Node{}, 0, []byte(""), 0) {
			t.Error("expected false for null node")
		}
	})
}

func TestMatchStructural(t *testing.T) {
	t.Run("basic match with dedup", func(t *testing.T) {
		source := []byte("function outer() { function inner() {} }")
		p := parser.NewParser(parser.LangJS)
		tree, err := p.Parse(context.Background(), source, nil)
		p.Close()
		if err != nil {
			t.Fatal(err)
		}
		defer tree.Close()

		lineStarts := buildLineIndex(source)
		rules := []compiledStructural{{
			name: "no-nested-fn",
			matcher: compiledASTMatcher{
				kind: "function_declaration",
				parent: &compiledASTMatcher{
					parent: &compiledASTMatcher{kind: "function_declaration"},
				},
			},
			message:  "No nested functions",
			severity: config.SeverityError,
		}}

		diags := matchStructural(context.Background(), rules, tree, source, lineStarts)
		if len(diags) != 1 {
			t.Fatalf("expected 1 diagnostic, got %d", len(diags))
		}
		if diags[0].Rule != "no-nested-fn" {
			t.Errorf("rule = %q, want %q", diags[0].Rule, "no-nested-fn")
		}
	})

	t.Run("multiple rules", func(t *testing.T) {
		source := []byte("function foo() {}\nclass Bar {}")
		p := parser.NewParser(parser.LangJS)
		tree, err := p.Parse(context.Background(), source, nil)
		p.Close()
		if err != nil {
			t.Fatal(err)
		}
		defer tree.Close()

		lineStarts := buildLineIndex(source)
		rules := []compiledStructural{
			{
				name:     "no-fn",
				matcher:  compiledASTMatcher{kind: "function_declaration"},
				message:  "No functions",
				severity: config.SeverityWarn,
			},
			{
				name:     "no-class",
				matcher:  compiledASTMatcher{kind: "class_declaration"},
				message:  "No classes",
				severity: config.SeverityError,
			},
		}

		diags := matchStructural(context.Background(), rules, tree, source, lineStarts)
		if len(diags) != 2 {
			t.Fatalf("expected 2 diagnostics, got %d", len(diags))
		}
		ruleSet := map[string]bool{}
		for _, d := range diags {
			ruleSet[d.Rule] = true
		}
		if !ruleSet["no-fn"] {
			t.Error("missing no-fn diagnostic")
		}
		if !ruleSet["no-class"] {
			t.Error("missing no-class diagnostic")
		}
	})

	t.Run("fix attachment", func(t *testing.T) {
		source := []byte("function foo() {}")
		p := parser.NewParser(parser.LangJS)
		tree, err := p.Parse(context.Background(), source, nil)
		p.Close()
		if err != nil {
			t.Fatal(err)
		}
		defer tree.Close()

		lineStarts := buildLineIndex(source)
		rules := []compiledStructural{{
			name:     "remove-fn",
			matcher:  compiledASTMatcher{kind: "function_declaration"},
			message:  "Remove function",
			severity: config.SeverityError,
			fix:      "/* removed */",
		}}

		diags := matchStructural(context.Background(), rules, tree, source, lineStarts)
		if len(diags) != 1 {
			t.Fatalf("expected 1 diagnostic, got %d", len(diags))
		}
		if diags[0].Fix == nil {
			t.Fatal("expected fix to be attached")
		}
		if diags[0].Fix.NewText != "/* removed */" {
			t.Errorf("fix text = %q, want %q", diags[0].Fix.NewText, "/* removed */")
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		source := []byte("function foo() {}\nfunction bar() {}")
		p := parser.NewParser(parser.LangJS)
		tree, err := p.Parse(context.Background(), source, nil)
		p.Close()
		if err != nil {
			t.Fatal(err)
		}
		defer tree.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		lineStarts := buildLineIndex(source)
		rules := []compiledStructural{{
			name:     "no-fn",
			matcher:  compiledASTMatcher{kind: "function_declaration"},
			message:  "No functions",
			severity: config.SeverityError,
		}}

		diags := matchStructural(ctx, rules, tree, source, lineStarts)
		// Cancelled context may produce 0 or partial results.
		if len(diags) > 2 {
			t.Fatalf("expected at most 2 diagnostics, got %d", len(diags))
		}
	})

	t.Run("dedup same line", func(t *testing.T) {
		// Two function declarations on the same line — should produce only one diagnostic.
		source := []byte("function a() {} function b() {}")
		p := parser.NewParser(parser.LangJS)
		tree, err := p.Parse(context.Background(), source, nil)
		p.Close()
		if err != nil {
			t.Fatal(err)
		}
		defer tree.Close()

		lineStarts := buildLineIndex(source)
		rules := []compiledStructural{{
			name:     "no-fn",
			matcher:  compiledASTMatcher{kind: "function_declaration"},
			message:  "No functions",
			severity: config.SeverityError,
		}}

		diags := matchStructural(context.Background(), rules, tree, source, lineStarts)
		if len(diags) != 1 {
			t.Fatalf("expected 1 diagnostic (dedup by line), got %d", len(diags))
		}
	})

	t.Run("name extraction via ChildByFieldName", func(t *testing.T) {
		// function_declaration has a "name" field child in tree-sitter.
		source := []byte("function myFunc() {}\nfunction otherFunc() {}")
		p := parser.NewParser(parser.LangJS)
		tree, err := p.Parse(context.Background(), source, nil)
		p.Close()
		if err != nil {
			t.Fatal(err)
		}
		defer tree.Close()

		lineStarts := buildLineIndex(source)
		rules := []compiledStructural{{
			name:     "only-myFunc",
			matcher:  compiledASTMatcher{kind: "function_declaration", name: &compiledNameMatch{exact: "myFunc"}},
			message:  "Found myFunc",
			severity: config.SeverityWarn,
		}}

		diags := matchStructural(context.Background(), rules, tree, source, lineStarts)
		if len(diags) != 1 {
			t.Fatalf("expected 1 diagnostic for myFunc only, got %d", len(diags))
		}
		if diags[0].Line != 1 {
			t.Errorf("line = %d, want 1", diags[0].Line)
		}
	})

	t.Run("name regex across multiple nodes", func(t *testing.T) {
		source := []byte("function debugA() {}\nfunction processB() {}\nfunction debugC() {}")
		p := parser.NewParser(parser.LangJS)
		tree, err := p.Parse(context.Background(), source, nil)
		p.Close()
		if err != nil {
			t.Fatal(err)
		}
		defer tree.Close()

		re, _ := compileNameMatch("test", "/^debug/")
		lineStarts := buildLineIndex(source)
		rules := []compiledStructural{{
			name:     "no-debug-fn",
			matcher:  compiledASTMatcher{kind: "function_declaration", name: &re},
			message:  "No debug functions",
			severity: config.SeverityError,
		}}

		diags := matchStructural(context.Background(), rules, tree, source, lineStarts)
		if len(diags) != 2 {
			t.Fatalf("expected 2 diagnostics for debug* functions, got %d", len(diags))
		}
	})

	t.Run("delete-statement fix", func(t *testing.T) {
		source := []byte("function foo() {}\nconst x = 1;\n")
		p := parser.NewParser(parser.LangJS)
		tree, err := p.Parse(context.Background(), source, nil)
		p.Close()
		if err != nil {
			t.Fatal(err)
		}
		defer tree.Close()

		lineStarts := buildLineIndex(source)
		rules := []compiledStructural{{
			name:     "remove-fn",
			matcher:  compiledASTMatcher{kind: "function_declaration"},
			message:  "Remove function",
			severity: config.SeverityError,
			fix:      fixDeleteStatement,
		}}

		diags := matchStructural(context.Background(), rules, tree, source, lineStarts)
		if len(diags) != 1 {
			t.Fatalf("expected 1 diagnostic, got %d", len(diags))
		}
		if diags[0].Fix == nil {
			t.Fatal("expected fix to be attached")
		}
		if diags[0].Fix.NewText != "" {
			t.Errorf("fix text = %q, want empty (delete)", diags[0].Fix.NewText)
		}
	})

	t.Run("not with name constraint", func(t *testing.T) {
		// Match functions NOT named "main".
		source := []byte("function main() {}\nfunction helper() {}")
		p := parser.NewParser(parser.LangJS)
		tree, err := p.Parse(context.Background(), source, nil)
		p.Close()
		if err != nil {
			t.Fatal(err)
		}
		defer tree.Close()

		lineStarts := buildLineIndex(source)
		rules := []compiledStructural{{
			name: "no-non-main-fn",
			matcher: compiledASTMatcher{
				kind: "function_declaration",
				not:  &compiledASTMatcher{name: &compiledNameMatch{exact: "main"}},
			},
			message:  "Only main function allowed",
			severity: config.SeverityError,
		}}

		diags := matchStructural(context.Background(), rules, tree, source, lineStarts)
		if len(diags) != 1 {
			t.Fatalf("expected 1 diagnostic for non-main function, got %d", len(diags))
		}
		if diags[0].Line != 2 {
			t.Errorf("line = %d, want 2 (helper function)", diags[0].Line)
		}
	})

	t.Run("class method matching", func(t *testing.T) {
		// Match methods inside class bodies.
		source := []byte("class Foo { bar() {} baz() {} }")
		p := parser.NewParser(parser.LangJS)
		tree, err := p.Parse(context.Background(), source, nil)
		p.Close()
		if err != nil {
			t.Fatal(err)
		}
		defer tree.Close()

		lineStarts := buildLineIndex(source)
		rules := []compiledStructural{{
			name: "class-method",
			matcher: compiledASTMatcher{
				kind:   "method_definition",
				parent: &compiledASTMatcher{kind: "class_body"},
			},
			message:  "Found class method",
			severity: config.SeverityWarn,
		}}

		diags := matchStructural(context.Background(), rules, tree, source, lineStarts)
		// Both methods are on line 1, so dedup will keep only one.
		if len(diags) != 1 {
			t.Fatalf("expected 1 diagnostic (dedup by line), got %d", len(diags))
		}
	})

	t.Run("ERROR nodes skipped", func(t *testing.T) {
		// Malformed JS but function declaration still parseable.
		source := []byte("function foo() {}\n{{{bad;;;")
		p := parser.NewParser(parser.LangJS)
		tree, err := p.Parse(context.Background(), source, nil)
		p.Close()
		if err != nil {
			t.Fatal(err)
		}
		defer tree.Close()

		lineStarts := buildLineIndex(source)
		rules := []compiledStructural{{
			name:     "no-fn",
			matcher:  compiledASTMatcher{kind: "function_declaration"},
			message:  "No functions",
			severity: config.SeverityError,
		}}

		diags := matchStructural(context.Background(), rules, tree, source, lineStarts)
		if len(diags) != 1 {
			t.Fatalf("expected 1 diagnostic, got %d", len(diags))
		}
	})

	t.Run("empty source no matches", func(t *testing.T) {
		source := []byte("")
		p := parser.NewParser(parser.LangJS)
		tree, err := p.Parse(context.Background(), source, nil)
		p.Close()
		if err != nil {
			t.Fatal(err)
		}
		defer tree.Close()

		lineStarts := buildLineIndex(source)
		rules := []compiledStructural{{
			name:     "no-fn",
			matcher:  compiledASTMatcher{kind: "function_declaration"},
			message:  "No functions",
			severity: config.SeverityError,
		}}

		diags := matchStructural(context.Background(), rules, tree, source, lineStarts)
		if len(diags) != 0 {
			t.Fatalf("expected 0 diagnostics, got %d", len(diags))
		}
	})

	t.Run("empty rules", func(t *testing.T) {
		diags := matchStructural(context.Background(), nil, nil, nil, nil)
		if len(diags) != 0 {
			t.Fatalf("expected 0 diagnostics, got %d", len(diags))
		}
	})
}

func TestMatchStructural_NamingConvention(t *testing.T) {
	// naming: ^[a-z] should flag "Foo" but not "foo".
	source := []byte("function Foo() {}\nfunction foo() {}")
	p := parser.NewParser(parser.LangJS)
	tree, err := p.Parse(context.Background(), source, nil)
	p.Close()
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Close()

	nm, err := compileNaming("camelcase-fn", &config.NamingMatcher{Match: "^[a-z]"})
	if err != nil {
		t.Fatal(err)
	}

	lineStarts := buildLineIndex(source)
	rules := []compiledStructural{{
		name:     "camelcase-fn",
		matcher:  compiledASTMatcher{kind: "function_declaration"},
		naming:   nm,
		message:  "default message",
		severity: config.SeverityError,
	}}

	diags := matchStructural(context.Background(), rules, tree, source, lineStarts)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic (Foo only), got %d", len(diags))
	}
	if diags[0].Line != 1 {
		t.Errorf("line = %d, want 1 (Foo)", diags[0].Line)
	}
}

func TestMatchStructural_NamingCustomMessage(t *testing.T) {
	source := []byte("function BadName() {}")
	p := parser.NewParser(parser.LangJS)
	tree, err := p.Parse(context.Background(), source, nil)
	p.Close()
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Close()

	nm, err := compileNaming("camelcase-fn", &config.NamingMatcher{
		Match:   "^[a-z]",
		Message: "must be camelCase",
	})
	if err != nil {
		t.Fatal(err)
	}

	lineStarts := buildLineIndex(source)
	rules := []compiledStructural{{
		name:     "camelcase-fn",
		matcher:  compiledASTMatcher{kind: "function_declaration"},
		naming:   nm,
		message:  "default message",
		severity: config.SeverityError,
	}}

	diags := matchStructural(context.Background(), rules, tree, source, lineStarts)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].Message != "must be camelCase" {
		t.Errorf("message = %q, want %q", diags[0].Message, "must be camelCase")
	}
}

func TestMatchStructural_NamingWithKindAndName(t *testing.T) {
	// AST name filter (exact match "foo") + naming regex (must start lowercase).
	// "foo" matches both AST name and naming → no diagnostic.
	// Only "foo" is matched by AST name filter, so no diagnostics at all.
	source := []byte("function foo() {}\nfunction Bar() {}")
	p := parser.NewParser(parser.LangJS)
	tree, err := p.Parse(context.Background(), source, nil)
	p.Close()
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Close()

	nm, err := compileNaming("test", &config.NamingMatcher{Match: "^[a-z]"})
	if err != nil {
		t.Fatal(err)
	}

	lineStarts := buildLineIndex(source)
	rules := []compiledStructural{{
		name:     "test",
		matcher:  compiledASTMatcher{kind: "function_declaration", name: &compiledNameMatch{exact: "foo"}},
		naming:   nm,
		message:  "bad name",
		severity: config.SeverityError,
	}}

	diags := matchStructural(context.Background(), rules, tree, source, lineStarts)
	// "foo" is matched by AST name filter and conforms to naming → no diagnostic.
	// "Bar" is not matched by AST name filter → skipped.
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestCompileStructuralRules_WithNaming(t *testing.T) {
	t.Parallel()
	rules := map[string]config.RuleConfig{
		"camelcase-fn": {
			Severity: config.SeverityError,
			AST:      &config.ASTMatcher{Kind: "function_declaration"},
			Naming:   &config.NamingMatcher{Match: "^[a-z]", Message: "must be camelCase"},
			Message:  "default",
		},
	}
	compiled, errs := compileStructuralRules(rules)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(compiled) != 1 {
		t.Fatalf("expected 1 compiled rule, got %d", len(compiled))
	}
	if compiled[0].naming == nil {
		t.Fatal("naming should be compiled")
	}
	if compiled[0].naming.message != "must be camelCase" {
		t.Errorf("naming.message = %q, want %q", compiled[0].naming.message, "must be camelCase")
	}
}

func TestCompileStructuralRules_InvalidNamingRegex(t *testing.T) {
	t.Parallel()
	rules := map[string]config.RuleConfig{
		"bad-naming": {
			Severity: config.SeverityError,
			AST:      &config.ASTMatcher{Kind: "function_declaration"},
			Naming:   &config.NamingMatcher{Match: "[invalid"},
			Message:  "bad",
		},
	}
	_, errs := compileStructuralRules(rules)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if !errors.Is(errs[0], ErrNamingCompile) {
		t.Errorf("error should wrap ErrNamingCompile, got: %v", errs[0])
	}
}

func TestExtractNodeName(t *testing.T) {
	t.Run("function declaration has name field", func(t *testing.T) {
		source := []byte("function myFunc() {}")
		p := parser.NewParser(parser.LangJS)
		tree, err := p.Parse(context.Background(), source, nil)
		p.Close()
		if err != nil {
			t.Fatal(err)
		}
		defer tree.Close()

		var fn parser.Node
		parser.WalkNamed(tree, func(node parser.Node, _ int) bool {
			if node.Kind() == "function_declaration" {
				fn = node
				return false
			}
			return true
		})
		if fn.IsNull() {
			t.Fatal("function_declaration not found")
		}

		name := extractNodeName(fn, source)
		if name != "myFunc" {
			t.Errorf("name = %q, want %q", name, "myFunc")
		}
	})

	t.Run("node without name field falls back to text", func(t *testing.T) {
		source := []byte("x + y")
		p := parser.NewParser(parser.LangJS)
		tree, err := p.Parse(context.Background(), source, nil)
		p.Close()
		if err != nil {
			t.Fatal(err)
		}
		defer tree.Close()

		var binExpr parser.Node
		parser.WalkNamed(tree, func(node parser.Node, _ int) bool {
			if node.Kind() == "binary_expression" {
				binExpr = node
				return false
			}
			return true
		})
		if binExpr.IsNull() {
			t.Fatal("binary_expression not found")
		}

		name := extractNodeName(binExpr, source)
		if name != "x + y" {
			t.Errorf("name = %q, want %q (full text fallback)", name, "x + y")
		}
	})

	t.Run("class declaration name field", func(t *testing.T) {
		source := []byte("class MyClass {}")
		p := parser.NewParser(parser.LangJS)
		tree, err := p.Parse(context.Background(), source, nil)
		p.Close()
		if err != nil {
			t.Fatal(err)
		}
		defer tree.Close()

		var cls parser.Node
		parser.WalkNamed(tree, func(node parser.Node, _ int) bool {
			if node.Kind() == "class_declaration" {
				cls = node
				return false
			}
			return true
		})
		if cls.IsNull() {
			t.Fatal("class_declaration not found")
		}

		name := extractNodeName(cls, source)
		if name != "MyClass" {
			t.Errorf("name = %q, want %q", name, "MyClass")
		}
	})
}
