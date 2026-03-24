package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/ralfjs/ralf/internal/config"
	"github.com/ralfjs/ralf/internal/parser"
)

func TestCompilePatternRules(t *testing.T) {
	t.Run("valid pattern compiles", func(t *testing.T) {
		rules := map[string]config.RuleConfig{
			"no-console": {
				Severity: config.SeverityError,
				Pattern:  "console.log($$$ARGS)",
				Message:  "No console.log",
			},
		}

		compiled, errs := compilePatternRules(rules)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if len(compiled) != 1 {
			t.Fatalf("expected 1 compiled pattern, got %d", len(compiled))
		}
		if compiled[0].name != "no-console" {
			t.Errorf("name = %q, want %q", compiled[0].name, "no-console")
		}
	})

	t.Run("off severity skipped", func(t *testing.T) {
		rules := map[string]config.RuleConfig{
			"off-rule": {
				Severity: config.SeverityOff,
				Pattern:  "console.log($$$ARGS)",
				Message:  "Disabled",
			},
		}

		compiled, errs := compilePatternRules(rules)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if len(compiled) != 0 {
			t.Fatalf("expected 0 compiled patterns, got %d", len(compiled))
		}
	})

	t.Run("non-pattern rules skipped", func(t *testing.T) {
		rules := map[string]config.RuleConfig{
			"regex-rule": {
				Severity: config.SeverityError,
				Regex:    `\bvar\b`,
				Message:  "No var",
			},
		}

		compiled, errs := compilePatternRules(rules)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if len(compiled) != 0 {
			t.Fatalf("expected 0 compiled patterns, got %d", len(compiled))
		}
	})

	t.Run("syntax error returns wrapped ErrPatternCompile", func(t *testing.T) {
		rules := map[string]config.RuleConfig{
			"bad": {Severity: config.SeverityError, Pattern: "function(", Message: "Bad"},
		}

		compiled, errs := compilePatternRules(rules)
		if len(errs) != 1 {
			t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
		}
		if !errors.Is(errs[0], ErrPatternCompile) {
			t.Errorf("expected ErrPatternCompile, got %v", errs[0])
		}
		if len(compiled) != 0 {
			t.Fatalf("expected 0 compiled patterns, got %d", len(compiled))
		}
	})

	t.Run("collects errors without fail-fast", func(t *testing.T) {
		rules := map[string]config.RuleConfig{
			"good": {Severity: config.SeverityError, Pattern: "console.log()", Message: "A"},
			"bad":  {Severity: config.SeverityError, Pattern: "function(", Message: "B"},
		}

		compiled, errs := compilePatternRules(rules)
		if len(errs) != 1 {
			t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
		}
		if len(compiled) != 1 {
			t.Fatalf("expected 1 compiled pattern, got %d", len(compiled))
		}
	})
}

func TestBuildPatternNode(t *testing.T) {
	// Helper: parse pattern as JS and build patternNode.
	build := func(t *testing.T, pattern string) patternNode {
		t.Helper()
		p := parser.NewParser(parser.LangJS)
		defer p.Close()

		tree, err := p.Parse(context.Background(), []byte(pattern), nil)
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		defer tree.Close()

		inner := unwrapPattern(tree.RootNode())
		return buildPatternNode(inner, []byte(pattern))
	}

	t.Run("literal tree for console.log()", func(t *testing.T) {
		pn := build(t, "console.log()")
		if pn.kind != patternLiteral {
			t.Fatalf("root kind = %d, want patternLiteral", pn.kind)
		}
		if pn.nodeKind != "call_expression" {
			t.Fatalf("root nodeKind = %q, want %q", pn.nodeKind, "call_expression")
		}
		if len(pn.children) == 0 {
			t.Fatal("expected children for call_expression")
		}
	})

	t.Run("wild for $NAME", func(t *testing.T) {
		pn := build(t, "$NAME")
		if pn.kind != patternWild {
			t.Fatalf("kind = %d, want patternWild", pn.kind)
		}
		if pn.name != "NAME" {
			t.Errorf("name = %q, want %q", pn.name, "NAME")
		}
	})

	t.Run("variadic for $$$ARGS", func(t *testing.T) {
		pn := build(t, "$$$ARGS")
		if pn.kind != patternVariadic {
			t.Fatalf("kind = %d, want patternVariadic", pn.kind)
		}
		if pn.name != "ARGS" {
			t.Errorf("name = %q, want %q", pn.name, "ARGS")
		}
	})

	t.Run("mixed console.log($$$ARGS)", func(t *testing.T) {
		pn := build(t, "console.log($$$ARGS)")
		if pn.kind != patternLiteral {
			t.Fatalf("root kind = %d, want patternLiteral", pn.kind)
		}
		// Should be call_expression with children including arguments
		// that contain a variadic node.
		if pn.nodeKind != "call_expression" {
			t.Fatalf("root nodeKind = %q, want %q", pn.nodeKind, "call_expression")
		}

		// Find the arguments child and check it has a variadic.
		found := false
		for _, child := range pn.children {
			if child.nodeKind == "arguments" {
				for _, arg := range child.children {
					if arg.kind == patternVariadic && arg.name == "ARGS" {
						found = true
					}
				}
			}
		}
		if !found {
			t.Error("expected variadic $$$ARGS in arguments children")
		}
	})
}

func TestMatchNode(t *testing.T) {
	// Helper: parse source as JS and return root.
	parseJS := func(t *testing.T, source string) *parser.Tree {
		t.Helper()
		p := parser.NewParser(parser.LangJS)
		t.Cleanup(p.Close)

		tree, err := p.Parse(context.Background(), []byte(source), nil)
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		return tree
	}

	// Helper: compile a pattern and return its root patternNode.
	compilePN := func(t *testing.T, pattern string) patternNode {
		t.Helper()
		cp, err := compilePattern("test", &config.RuleConfig{
			Severity: config.SeverityError,
			Pattern:  pattern,
			Message:  "test",
		})
		if err != nil {
			t.Fatalf("compile pattern failed: %v", err)
		}
		return cp.root
	}

	t.Run("exact literal match", func(t *testing.T) {
		pn := compilePN(t, "console.log()")
		tree := parseJS(t, "console.log();")
		defer tree.Close()

		source := []byte("console.log();")
		var matched bool
		parser.Walk(tree, func(node parser.Node, _ int) bool {
			if node.Kind() == "call_expression" {
				matched = matchNode(&pn, node, source, nil)
				return false
			}
			return true
		})
		if !matched {
			t.Error("expected match for console.log()")
		}
	})

	t.Run("kind mismatch fails", func(t *testing.T) {
		pn := compilePN(t, "console.log()")
		tree := parseJS(t, "var x = 1;")
		defer tree.Close()

		source := []byte("var x = 1;")
		var anyMatch bool
		parser.Walk(tree, func(node parser.Node, _ int) bool {
			if node.Kind() == "variable_declaration" {
				anyMatch = matchNode(&pn, node, source, nil)
			}
			return true
		})
		if anyMatch {
			t.Error("expected no match for variable_declaration")
		}
	})

	t.Run("wild matches any node", func(t *testing.T) {
		pn := patternNode{kind: patternWild, name: "X"}
		tree := parseJS(t, "42")
		defer tree.Close()

		source := []byte("42")
		var matched bool
		parser.Walk(tree, func(node parser.Node, _ int) bool {
			if node.Kind() == "number" {
				matched = matchNode(&pn, node, source, nil)
				return false
			}
			return true
		})
		if !matched {
			t.Error("expected wild to match number node")
		}
	})

	t.Run("nested structural match", func(t *testing.T) {
		pn := compilePN(t, "console.log($$$ARGS)")
		tree := parseJS(t, `console.log("hello", 42);`)
		defer tree.Close()

		source := []byte(`console.log("hello", 42);`)
		var matched bool
		parser.Walk(tree, func(node parser.Node, _ int) bool {
			if node.Kind() == "call_expression" {
				matched = matchNode(&pn, node, source, nil)
				return false
			}
			return true
		})
		if !matched {
			t.Error("expected nested structural match")
		}
	})
}

func TestMatchChildren(t *testing.T) {
	parseAndGetChildren := func(t *testing.T, source, parentKind string) ([]parser.Node, []byte) {
		t.Helper()
		p := parser.NewParser(parser.LangJS)
		t.Cleanup(p.Close)

		tree, err := p.Parse(context.Background(), []byte(source), nil)
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		t.Cleanup(tree.Close)

		src := []byte(source)
		var children []parser.Node
		parser.Walk(tree, func(node parser.Node, _ int) bool {
			if node.Kind() == parentKind {
				children = node.CollectChildren()
				return false
			}
			return true
		})
		return children, src
	}

	t.Run("variadic matches zero children", func(t *testing.T) {
		// Pattern: ($$$ARGS), target: ()
		patterns := []patternNode{
			{kind: patternLiteral, nodeKind: "(", text: "("},
			{kind: patternVariadic, name: "ARGS"},
			{kind: patternLiteral, nodeKind: ")", text: ")"},
		}
		children, source := parseAndGetChildren(t, "f()", "arguments")
		if !matchChildren(patterns, children, source, nil) {
			t.Error("expected variadic to match zero children in ()")
		}
	})

	t.Run("variadic matches multiple children", func(t *testing.T) {
		// Pattern: ($$$ARGS), target: (1, 2, 3)
		patterns := []patternNode{
			{kind: patternLiteral, nodeKind: "(", text: "("},
			{kind: patternVariadic, name: "ARGS"},
			{kind: patternLiteral, nodeKind: ")", text: ")"},
		}
		children, source := parseAndGetChildren(t, "f(1, 2, 3)", "arguments")
		if !matchChildren(patterns, children, source, nil) {
			t.Error("expected variadic to match multiple children in (1, 2, 3)")
		}
	})

	t.Run("variadic in middle f($A, $$$REST)", func(t *testing.T) {
		// Compile pattern and extract argument children from pattern.
		p := parser.NewParser(parser.LangJS)
		defer p.Close()

		patternSrc := []byte("f($A, $$$REST)")
		tree, err := p.Parse(context.Background(), patternSrc, nil)
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		defer tree.Close()

		inner := unwrapPattern(tree.RootNode())
		pn := buildPatternNode(inner, patternSrc)

		// Find arguments children in pattern.
		var patternArgs []patternNode
		for _, child := range pn.children {
			if child.nodeKind == "arguments" {
				patternArgs = child.children
				break
			}
		}
		if len(patternArgs) == 0 {
			t.Fatal("could not find arguments in pattern")
		}

		children, source := parseAndGetChildren(t, "f(1, 2, 3)", "arguments")
		if !matchChildren(patternArgs, children, source, nil) {
			t.Error("expected f($A, $$$REST) to match arguments of f(1, 2, 3)")
		}
	})
}

func TestMatchPatterns(t *testing.T) {
	// Helper: compile a single pattern rule.
	compile := func(t *testing.T, name, pattern, message string) compiledPattern {
		t.Helper()
		cp, err := compilePattern(name, &config.RuleConfig{
			Severity: config.SeverityError,
			Pattern:  pattern,
			Message:  message,
		})
		if err != nil {
			t.Fatalf("compile pattern %q failed: %v", name, err)
		}
		return cp
	}

	// Helper: parse source as JS and return tree + source bytes.
	parseSource := func(t *testing.T, source string) (*parser.Tree, []byte) {
		t.Helper()
		p := parser.NewParser(parser.LangJS)
		t.Cleanup(p.Close)

		src := []byte(source)
		tree, err := p.Parse(context.Background(), src, nil)
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		return tree, src
	}

	t.Run("console.log matches various calls", func(t *testing.T) {
		cp := compile(t, "no-console", "console.log($$$ARGS)", "No console.log")
		tree, source := parseSource(t, "console.log(\"hello\");\nconsole.log(a, b);\nconsole.log();")
		defer tree.Close()

		lineStarts := buildLineIndex(source)
		diags := matchPatterns(context.Background(), []compiledPattern{cp}, tree, source, lineStarts)

		if len(diags) != 3 {
			t.Fatalf("expected 3 diagnostics, got %d", len(diags))
		}
		for _, d := range diags {
			if d.Rule != "no-console" {
				t.Errorf("rule = %q, want %q", d.Rule, "no-console")
			}
		}
	})

	t.Run("does not match console.warn", func(t *testing.T) {
		cp := compile(t, "no-console", "console.log($$$ARGS)", "No console.log")
		tree, source := parseSource(t, `console.warn("hello");`)
		defer tree.Close()

		lineStarts := buildLineIndex(source)
		diags := matchPatterns(context.Background(), []compiledPattern{cp}, tree, source, lineStarts)

		if len(diags) != 0 {
			t.Fatalf("expected 0 diagnostics, got %d", len(diags))
		}
	})

	t.Run("var pattern matches var not let", func(t *testing.T) {
		cp := compile(t, "no-var", "var $NAME = $VALUE", "No var")
		tree, source := parseSource(t, "var x = 1;\nlet y = 2;")
		defer tree.Close()

		lineStarts := buildLineIndex(source)
		diags := matchPatterns(context.Background(), []compiledPattern{cp}, tree, source, lineStarts)

		if len(diags) != 1 {
			t.Fatalf("expected 1 diagnostic, got %d", len(diags))
		}
		if diags[0].Line != 1 {
			t.Errorf("line = %d, want 1", diags[0].Line)
		}
	})

	t.Run("dedup by line", func(t *testing.T) {
		cp := compile(t, "no-console", "console.log($$$ARGS)", "No console.log")
		// Two console.log calls on the same line.
		tree, source := parseSource(t, "console.log(1); console.log(2);")
		defer tree.Close()

		lineStarts := buildLineIndex(source)
		diags := matchPatterns(context.Background(), []compiledPattern{cp}, tree, source, lineStarts)

		if len(diags) != 1 {
			t.Fatalf("expected 1 diagnostic (dedup by line), got %d", len(diags))
		}
	})

	t.Run("ERROR nodes skipped", func(t *testing.T) {
		cp := compile(t, "no-console", "console.log($$$ARGS)", "No console.log")
		// Malformed JS with an ERROR node, but console.log is still parseable.
		tree, source := parseSource(t, "console.log(1);\n{{{bad;;;")
		defer tree.Close()

		lineStarts := buildLineIndex(source)
		diags := matchPatterns(context.Background(), []compiledPattern{cp}, tree, source, lineStarts)

		// Should still find the console.log on line 1.
		if len(diags) != 1 {
			t.Fatalf("expected 1 diagnostic, got %d", len(diags))
		}
	})

	t.Run("empty source no matches", func(t *testing.T) {
		cp := compile(t, "no-console", "console.log($$$ARGS)", "No console.log")
		tree, source := parseSource(t, "")
		defer tree.Close()

		lineStarts := buildLineIndex(source)
		diags := matchPatterns(context.Background(), []compiledPattern{cp}, tree, source, lineStarts)

		if len(diags) != 0 {
			t.Fatalf("expected 0 diagnostics, got %d", len(diags))
		}
	})

	t.Run("multiple pattern rules in one walk", func(t *testing.T) {
		cp1 := compile(t, "no-console-log", "console.log($$$ARGS)", "No console.log")
		cp2 := compile(t, "no-console-warn", "console.warn($$$ARGS)", "No console.warn")

		tree, source := parseSource(t, "console.log(1);\nconsole.warn(2);")
		defer tree.Close()

		lineStarts := buildLineIndex(source)
		diags := matchPatterns(context.Background(), []compiledPattern{cp1, cp2}, tree, source, lineStarts)

		if len(diags) != 2 {
			t.Fatalf("expected 2 diagnostics, got %d", len(diags))
		}

		rules := map[string]bool{}
		for _, d := range diags {
			rules[d.Rule] = true
		}
		if !rules["no-console-log"] {
			t.Error("missing diagnostic for no-console-log")
		}
		if !rules["no-console-warn"] {
			t.Error("missing diagnostic for no-console-warn")
		}
	})
}
