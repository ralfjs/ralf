package engine

import (
	"fmt"

	"github.com/ralfjs/ralf/internal/config"
	"github.com/ralfjs/ralf/internal/parser"
)

// builtinNodeChecker inspects a single AST node and appends diagnostics.
// Called once per matching node kind during a single tree walk.
type builtinNodeChecker func(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic)

// builtinRuleDef defines a builtin rule: the node kinds it triggers on and its checker.
type builtinRuleDef struct {
	kinds   []string // tree-sitter node kinds that trigger this rule
	checker builtinNodeChecker
}

// compiledBuiltin is a pre-compiled built-in rule ready for matching.
type compiledBuiltin struct {
	name     string
	checker  builtinNodeChecker
	message  string
	severity config.Severity
}

// builtinDefs maps rule names to their definitions.
var builtinDefs = map[string]builtinRuleDef{
	"no-empty-pattern":            {kinds: []string{"object_pattern", "array_pattern"}, checker: checkNoEmptyPattern},
	"no-empty-static-block":       {kinds: []string{"class_static_block"}, checker: checkNoEmptyStaticBlock},
	"no-compare-neg-zero":         {kinds: []string{"binary_expression"}, checker: checkNoCompareNegZero},
	"no-delete-var":               {kinds: []string{"unary_expression"}, checker: checkNoDeleteVar},
	"no-unsafe-negation":          {kinds: []string{"binary_expression"}, checker: checkNoUnsafeNegation},
	"valid-typeof":                {kinds: []string{"binary_expression"}, checker: checkValidTypeof},
	"use-isnan":                   {kinds: []string{"binary_expression"}, checker: checkUseIsNaN},
	"no-useless-catch":            {kinds: []string{"catch_clause"}, checker: checkNoUselessCatch},
	"no-sparse-arrays":            {kinds: []string{"array"}, checker: checkNoSparseArrays},
	"no-dupe-keys":                {kinds: []string{"object"}, checker: checkNoDupeKeys},
	"no-duplicate-case":           {kinds: []string{"switch_statement"}, checker: checkNoDuplicateCase},
	"no-self-assign":              {kinds: []string{"assignment_expression"}, checker: checkNoSelfAssign},
	"no-octal":                    {kinds: []string{"number"}, checker: checkNoOctal},
	"no-shadow-restricted-names":  {kinds: []string{"variable_declarator"}, checker: checkNoShadowRestrictedNames},
	"no-empty":                    {kinds: []string{"statement_block"}, checker: checkNoEmpty},
	"no-unsafe-finally":           {kinds: []string{"return_statement", "throw_statement", "break_statement", "continue_statement"}, checker: checkNoUnsafeFinally},
	"for-direction":               {kinds: []string{"for_statement"}, checker: checkForDirection},
	"no-setter-return":            {kinds: []string{"method_definition"}, checker: checkNoSetterReturn},
	"no-extra-boolean-cast":       {kinds: []string{"unary_expression"}, checker: checkNoExtraBooleanCast},
	"require-yield":               {kinds: []string{"generator_function_declaration", "generator_function"}, checker: checkRequireYield},
	"no-cond-assign":              {kinds: []string{"if_statement", "while_statement", "do_statement", "for_statement", "ternary_expression"}, checker: checkNoCondAssign},
	"no-self-compare":             {kinds: []string{"binary_expression"}, checker: checkNoSelfCompare},
	"eqeqeq":                      {kinds: []string{"binary_expression"}, checker: checkEqeqeq},
	"no-empty-character-class":    {kinds: []string{"regex"}, checker: checkNoEmptyCharacterClass},
	"no-dupe-class-members":       {kinds: []string{"class_body"}, checker: checkNoDupeClassMembers},
	"no-dupe-args":                {kinds: []string{"formal_parameters"}, checker: checkNoDupeArgs},
	"no-constructor-return":       {kinds: []string{"return_statement"}, checker: checkNoConstructorReturn},
	"no-inner-declarations":       {kinds: []string{"function_declaration", "generator_function_declaration"}, checker: checkNoInnerDeclarations},
	"no-unsafe-optional-chaining": {kinds: []string{"member_expression", "call_expression"}, checker: checkNoUnsafeOptionalChaining},
	"no-constant-condition":       {kinds: []string{"if_statement", "while_statement", "do_statement", "for_statement", "ternary_expression"}, checker: checkNoConstantCondition},
	"no-loss-of-precision":        {kinds: []string{"number"}, checker: checkNoLossOfPrecision},
	"getter-return":               {kinds: []string{"method_definition"}, checker: checkGetterReturn},
	"no-fallthrough":              {kinds: []string{"switch_case"}, checker: checkNoFallthrough},
}

// isFunctionNode returns true for node kinds that define a new function scope.
// Used by builtin checkers that need to stop ancestor/descendant walks at
// function boundaries.
func isFunctionNode(kind string) bool {
	switch kind {
	case "function_declaration", "function_expression", "arrow_function",
		"method_definition", "generator_function_declaration", "generator_function":
		return true
	}
	return false
}

// compileBuiltinRules selects rules that have a registered Go checker function.
// Returns an error for any rule with Builtin=true that has no registered checker.
func compileBuiltinRules(rules map[string]config.RuleConfig) ([]compiledBuiltin, []error) {
	compiled := make([]compiledBuiltin, 0, len(builtinDefs))
	var errs []error
	for name := range rules {
		rule := rules[name]
		if rule.Severity == config.SeverityOff || !rule.Builtin {
			continue
		}
		def, ok := builtinDefs[name]
		if !ok {
			errs = append(errs, fmt.Errorf("rule %q: builtin checker not registered", name))
			continue
		}
		compiled = append(compiled, compiledBuiltin{
			name:     name,
			checker:  def.checker,
			message:  rule.Message,
			severity: rule.Severity,
		})
	}
	return compiled, errs
}

// builtinIndex maps kindID to the set of builtin rules triggered by that kind.
// Built once per LintFile call from the active rules.
type builtinIndex struct {
	byKindID map[uint16][]int // kindID → indices into the rules slice
}

func buildBuiltinIndex(rules []compiledBuiltin, root parser.Node) builtinIndex {
	idx := builtinIndex{byKindID: make(map[uint16][]int, len(rules)*2)}
	for i, r := range rules {
		def := builtinDefs[r.name]
		for _, kind := range def.kinds {
			kid := root.SymbolForKind(kind, true)
			if kid != 0 {
				idx.byKindID[kid] = append(idx.byKindID[kid], i)
			}
		}
	}
	return idx
}

// matchBuiltins runs all active builtin rules in a single tree walk, dispatching
// each node to matching checkers by kindID. Rule metadata (name, severity,
// message) is stamped onto diagnostics produced by each checker.
func matchBuiltins(rules []compiledBuiltin, tree *parser.Tree, source []byte, lineStarts []int) []Diagnostic {
	if len(rules) == 0 {
		return nil
	}
	root := tree.RootNode()
	idx := buildBuiltinIndex(rules, root)

	var diags []Diagnostic
	parser.WalkNamed(tree, func(node parser.Node, _ int) bool {
		ruleIndices := idx.byKindID[node.KindID()]
		for _, ri := range ruleIndices {
			r := &rules[ri]
			before := len(diags)
			r.checker(node, source, lineStarts, &diags)
			// Tag new diagnostics with rule metadata.
			for k := before; k < len(diags); k++ {
				diags[k].Rule = r.name
				diags[k].Severity = r.severity
				if diags[k].Message == "" {
					diags[k].Message = r.message
				}
			}
		}
		return true
	})
	return diags
}

// resolveBuiltinRules filters builtin rules that are active for the given file.
func resolveBuiltinRules(rules []compiledBuiltin, effective map[string]config.RuleConfig, filePath string) []compiledBuiltin {
	active := make([]compiledBuiltin, 0, len(rules))
	for i := range rules {
		cb := &rules[i]
		sev, msg, ok := resolveRule(effective, cb.name, cb.message, filePath)
		if !ok {
			continue
		}
		resolved := *cb
		resolved.severity = sev
		resolved.message = msg
		active = append(active, resolved)
	}
	return active
}
