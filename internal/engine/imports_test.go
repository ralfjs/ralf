package engine

import (
	"context"
	"testing"

	"github.com/ralfjs/ralf/internal/config"
	"github.com/ralfjs/ralf/internal/parser"
)

func TestClassifyImport(t *testing.T) {
	t.Parallel()
	tests := []struct {
		source string
		isType bool
		want   importGroup
	}{
		// builtin
		{"fs", false, groupBuiltin},
		{"path", false, groupBuiltin},
		{"node:fs", false, groupBuiltin},
		{"node:path", false, groupBuiltin},
		{"http", false, groupBuiltin},
		{"stream/promises", false, groupBuiltin},
		// external
		{"react", false, groupExternal},
		{"@scope/pkg", false, groupExternal},
		{"lodash", false, groupExternal},
		{"lodash/merge", false, groupExternal},
		// internal
		{"@/utils", false, groupInternal},
		{"~/components", false, groupInternal},
		{"@/deep/path", false, groupInternal},
		// parent
		{"../helper", false, groupParent},
		{"../../lib", false, groupParent},
		// sibling
		{"./utils", false, groupSibling},
		{"./config", false, groupSibling},
		{"./deep/path", false, groupSibling},
		// index
		{".", false, groupIndex},
		{"./index", false, groupIndex},
		{"./index.js", false, groupIndex},
		{"./index.ts", false, groupIndex},
		// type
		{"react", true, groupType},
		{"./utils", true, groupType},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			t.Parallel()
			got := classifyImport(tt.source, tt.isType)
			if got != tt.want {
				t.Errorf("classifyImport(%q, %v) = %d, want %d", tt.source, tt.isType, got, tt.want)
			}
		})
	}
}

func TestIsNodeBuiltin(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path string
		want bool
	}{
		{"fs", true},
		{"path", true},
		{"http", true},
		{"node:fs", true},
		{"node:crypto", true},
		{"node:anything", true},
		{"react", false},
		{"lodash", false},
		{"./local", false},
		{"../parent", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			got := isNodeBuiltin(tt.path)
			if got != tt.want {
				t.Errorf("isNodeBuiltin(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestCompileImportRules(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		rules := map[string]config.RuleConfig{
			"import-order": {
				Severity: config.SeverityWarn,
				Imports: &config.ImportsMatcher{
					Groups:      []string{"builtin", "external", "sibling"},
					Alphabetize: true,
				},
				Message: "Imports not ordered",
			},
		}
		compiled, errs := compileImportRules(rules)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if len(compiled) != 1 {
			t.Fatalf("expected 1 compiled rule, got %d", len(compiled))
		}
		if len(compiled[0].groups) != 3 {
			t.Errorf("expected 3 groups, got %d", len(compiled[0].groups))
		}
		if !compiled[0].alpha {
			t.Error("expected alphabetize=true")
		}
	})

	t.Run("invalid group name", func(t *testing.T) {
		rules := map[string]config.RuleConfig{
			"import-order": {
				Severity: config.SeverityWarn,
				Imports: &config.ImportsMatcher{
					Groups: []string{"builtin", "bogus"},
				},
			},
		}
		compiled, errs := compileImportRules(rules)
		if len(errs) == 0 {
			t.Fatal("expected errors for invalid group name")
		}
		if len(compiled) != 0 {
			t.Errorf("expected 0 compiled rules, got %d", len(compiled))
		}
	})

	t.Run("severity off skipped", func(t *testing.T) {
		rules := map[string]config.RuleConfig{
			"import-order": {
				Severity: config.SeverityOff,
				Imports:  &config.ImportsMatcher{Groups: []string{"builtin"}},
			},
		}
		compiled, errs := compileImportRules(rules)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if len(compiled) != 0 {
			t.Errorf("expected 0 compiled rules, got %d", len(compiled))
		}
	})

	t.Run("non-import rules ignored", func(t *testing.T) {
		rules := map[string]config.RuleConfig{
			"no-var": {Severity: config.SeverityError, Regex: `\bvar\b`},
		}
		compiled, errs := compileImportRules(rules)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if len(compiled) != 0 {
			t.Errorf("expected 0 compiled rules, got %d", len(compiled))
		}
	})
}

func parseSource(t *testing.T, lang parser.Lang, source []byte) *parser.Tree {
	t.Helper()
	p := parser.NewParser(lang)
	tree, err := p.Parse(context.Background(), source, nil)
	p.Close()
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	return tree
}

func TestMatchImports_GroupOrder(t *testing.T) {
	source := []byte(`import React from "react";
import fs from "fs";
import { helper } from "./helper";
import { config } from "../config";
`)
	tree := parseSource(t, parser.LangJS, source)
	defer tree.Close()

	lineStarts := buildLineIndex(source)
	rules := []compiledImport{{
		name:     "import-order",
		groups:   []importGroup{groupBuiltin, groupExternal, groupParent, groupSibling},
		severity: config.SeverityWarn,
		message:  "wrong order",
	}}

	diags := matchImports(context.Background(), rules, tree, source, lineStarts)
	// fs (builtin) comes after React (external) → violation
	// ../config (parent) comes after ./helper (sibling) → violation
	if len(diags) < 2 {
		t.Fatalf("expected at least 2 diagnostics, got %d", len(diags))
	}
}

func TestMatchImports_Alphabetize(t *testing.T) {
	source := []byte(`import z from "react";
import a from "lodash";
`)
	tree := parseSource(t, parser.LangJS, source)
	defer tree.Close()

	lineStarts := buildLineIndex(source)
	rules := []compiledImport{{
		name:     "import-order",
		groups:   []importGroup{groupExternal},
		alpha:    true,
		severity: config.SeverityWarn,
	}}

	diags := matchImports(context.Background(), rules, tree, source, lineStarts)
	// "lodash" should appear before "react" alphabetically.
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
}

func TestMatchImports_NewlineBetween(t *testing.T) {
	t.Run("missing blank line between groups", func(t *testing.T) {
		source := []byte(`import fs from "fs";
import React from "react";
`)
		tree := parseSource(t, parser.LangJS, source)
		defer tree.Close()

		lineStarts := buildLineIndex(source)
		rules := []compiledImport{{
			name:     "import-order",
			groups:   []importGroup{groupBuiltin, groupExternal},
			newline:  true,
			severity: config.SeverityWarn,
		}}

		diags := matchImports(context.Background(), rules, tree, source, lineStarts)
		if len(diags) != 1 {
			t.Fatalf("expected 1 diagnostic (missing blank line), got %d", len(diags))
		}
	})

	t.Run("extra blank line within group", func(t *testing.T) {
		source := []byte(`import a from "a-pkg";

import b from "b-pkg";
`)
		tree := parseSource(t, parser.LangJS, source)
		defer tree.Close()

		lineStarts := buildLineIndex(source)
		rules := []compiledImport{{
			name:     "import-order",
			groups:   []importGroup{groupExternal},
			newline:  true,
			severity: config.SeverityWarn,
		}}

		diags := matchImports(context.Background(), rules, tree, source, lineStarts)
		if len(diags) != 1 {
			t.Fatalf("expected 1 diagnostic (extra blank line), got %d", len(diags))
		}
	})
}

func TestMatchImports_NoViolations(t *testing.T) {
	source := []byte(`import fs from "fs";
import path from "path";

import lodash from "lodash";
import React from "react";

import { helper } from "./helper";
`)
	tree := parseSource(t, parser.LangJS, source)
	defer tree.Close()

	lineStarts := buildLineIndex(source)
	rules := []compiledImport{{
		name:     "import-order",
		groups:   []importGroup{groupBuiltin, groupExternal, groupSibling},
		alpha:    true,
		newline:  true,
		severity: config.SeverityWarn,
	}}

	diags := matchImports(context.Background(), rules, tree, source, lineStarts)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestMatchImports_EmptyFile(t *testing.T) {
	source := []byte("const x = 1;\n")
	tree := parseSource(t, parser.LangJS, source)
	defer tree.Close()

	lineStarts := buildLineIndex(source)
	rules := []compiledImport{{
		name:     "import-order",
		groups:   []importGroup{groupBuiltin, groupExternal},
		severity: config.SeverityWarn,
	}}

	diags := matchImports(context.Background(), rules, tree, source, lineStarts)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestMatchImports_SingleImport(t *testing.T) {
	source := []byte(`import React from "react";
`)
	tree := parseSource(t, parser.LangJS, source)
	defer tree.Close()

	lineStarts := buildLineIndex(source)
	rules := []compiledImport{{
		name:     "import-order",
		groups:   []importGroup{groupExternal},
		alpha:    true,
		newline:  true,
		severity: config.SeverityWarn,
	}}

	diags := matchImports(context.Background(), rules, tree, source, lineStarts)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestMatchImports_TypeImports(t *testing.T) {
	source := []byte(`import type { FC } from "react";
import { useState } from "react";
`)
	tree := parseSource(t, parser.LangTS, source)
	defer tree.Close()

	lineStarts := buildLineIndex(source)
	rules := []compiledImport{{
		name:     "import-order",
		groups:   []importGroup{groupExternal, groupType},
		alpha:    true,
		severity: config.SeverityWarn,
	}}

	diags := matchImports(context.Background(), rules, tree, source, lineStarts)
	// "import type" from react → groupType
	// "import" from react → groupExternal
	// Type appears before External, but rule says external first → 1 violation
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic (type before external), got %d", len(diags))
	}
}

func TestStripQuotes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{`"react"`, "react"},
		{`'react'`, "react"},
		{"`react`", "react"},
		{"react", "react"},
		{`""`, ""},
		{`"a"`, "a"},
		{`"mismatched'`, `"mismatched'`},   // no strip: mismatched quotes
		{`'mismatched"`, `'mismatched"`},   // no strip: mismatched quotes
		{`"unterminated`, `"unterminated`}, // no strip: no closing quote
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := stripQuotes(tt.input)
			if got != tt.want {
				t.Errorf("stripQuotes(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
