package config

// noLabelsNotDefault matches any JS identifier except exactly "default".
// Rure lacks lookahead, so we exclude "default" via character-position alternation:
//   - $-prefixed identifiers ($label, $$)
//   - \w words of length 1-6 or 8+
//   - 7-char \w words that differ from "default" at any position
const noLabelsNotDefault = `(?:` +
	`\$[\w$]*` +
	`|\w{1,6}|\w{8,}` +
	`|[a-ce-zA-Z_]\w{6}` +
	`|d[a-df-zA-Z0-9_]\w{5}` +
	`|de[a-eg-zA-Z0-9_]\w{4}` +
	`|def[b-zA-Z0-9_]\w{3}` +
	`|defa[a-tv-zA-Z0-9_]\w{2}` +
	`|defau[a-km-zA-Z0-9_]\w` +
	`|defaul[a-su-zA-Z0-9_]` +
	`)`

// BuiltinRules returns the 50 built-in rules. A fresh map is returned
// on every call so callers may mutate it freely.
//
// Each rule is modeled after its ESLint equivalent where one exists.
// Rules use regex, pattern, AST structural, or custom Go built-in matchers
// depending on the analysis required.
func BuiltinRules() map[string]RuleConfig {
	return map[string]RuleConfig{
		// ESLint: no-var — flags all var declarations.
		"no-var": {
			Severity: SeverityError,
			Regex:    `\bvar\s`,
			Message:  "Use `let` or `const` instead of `var`",
		},
		// ESLint: no-console — flags all console method calls.
		// Covers the full set of standard Console API methods.
		"no-console": {
			Severity: SeverityWarn,
			Regex:    `\bconsole\.(log|warn|error|info|debug|trace|dir|dirxml|table|time|timeEnd|timeLog|timeStamp|assert|clear|count|countReset|group|groupCollapsed|groupEnd|profile|profileEnd)\s*\(`,
			Message:  "Unexpected console statement",
		},
		// ESLint: no-eval — flags eval() calls and indirect eval via (0, eval)().
		"no-eval": {
			Severity: SeverityError,
			Regex:    `\beval[ \t]*\(|\(\s*0\s*,\s*eval\s*\)[ \t]*\(`,
			Message:  "`eval` is dangerous and should not be used",
		},
		// ESLint: no-debugger — flags debugger statements.
		"no-debugger": {
			Severity: SeverityError,
			Regex:    `\bdebugger\b`,
			Message:  "Unexpected `debugger` statement",
		},
		// ESLint: no-alert — flags alert(), confirm(), and prompt().
		"no-alert": {
			Severity: SeverityWarn,
			Regex:    `\b(alert|confirm|prompt)\s*\(`,
			Message:  "Unexpected use of `alert`, `confirm`, or `prompt`",
		},
		// No ESLint core equivalent — RALF-original XSS prevention rule.
		"no-inner-html": {
			Severity: SeverityError,
			Regex:    `\.innerHTML\s*=`,
			Message:  "Direct `.innerHTML` assignment is an XSS risk",
		},
		// ESLint: no-with — flags with statements.
		"no-with": {
			Severity: SeverityError,
			Regex:    `\bwith\s*\(`,
			Message:  "`with` statement is not allowed",
		},
		// ESLint: no-caller — flags arguments.caller and arguments.callee.
		"no-caller": {
			Severity: SeverityError,
			Regex:    `\barguments\.(caller|callee)\b`,
			Message:  "`arguments.caller` and `arguments.callee` are deprecated",
		},
		// ESLint: no-implied-eval — flags setTimeout/setInterval/execScript
		// with string first argument.
		"no-implied-eval": {
			Severity: SeverityError,
			Regex:    `\b(setTimeout|setInterval|execScript)\s*\(\s*["'` + "`" + `]`,
			Message:  "Implied `eval` through string argument",
		},
		// ESLint: no-new-wrappers — flags new String/Number/Boolean
		// (with or without parentheses, matching ESLint behavior).
		"no-new-wrappers": {
			Severity: SeverityError,
			Regex:    `\bnew\s+(String|Number|Boolean)\b`,
			Message:  "Do not use primitive wrapper constructors",
		},
		// ESLint: no-proto — flags __proto__ via dot and bracket notation.
		"no-proto": {
			Severity: SeverityError,
			Regex:    `\.__proto__\b|\["__proto__"\]|\['__proto__'\]`,
			Message:  "Use `Object.getPrototypeOf` instead of `__proto__`",
		},
		// ESLint: no-iterator — flags __iterator__ via dot and bracket notation.
		"no-iterator": {
			Severity: SeverityError,
			Regex:    `\.__iterator__\b|\["__iterator__"\]|\['__iterator__'\]`,
			Message:  "`__iterator__` is obsolete; use `Symbol.iterator`",
		},
		// ESLint: no-new-func — flags new Function() and Function() calls.
		"no-new-func": {
			Severity: SeverityError,
			Regex:    `\bFunction\s*\(`,
			Message:  "`Function()` is a form of `eval`",
		},
		// ESLint: no-void — flags the void operator.
		// Matches void in expression context only, not TS type annotations.
		// Covers both void EXPR and void(EXPR) forms.
		"no-void": {
			Severity: SeverityWarn,
			Regex:    `(?m)(?:^|[=(!&|?+\-~,;\{\[]|return)\s*\bvoid[\s(]|=>\s*\bvoid\s+[a-zA-Z0-9_$"'(!~+\-]`,
			Message:  "Avoid the `void` operator",
		},
		// ESLint: no-script-url — flags javascript: protocol URLs (case-insensitive).
		"no-script-url": {
			Severity: SeverityError,
			Regex:    `(?i)["'` + "`" + `]javascript:`,
			Message:  "Script URLs are a form of `eval`",
		},
		// ESLint: no-extend-native — flags extending native prototypes.
		// Covers direct assignment and Object.defineProperty/defineProperties.
		"no-extend-native": {
			Severity: SeverityError,
			Regex:    `\b(Object|Array|String|Number|Boolean|Function|RegExp|Date|Error|Map|Set|WeakMap|WeakSet|Promise|Symbol|BigInt)\.prototype\.\w+\s*=|\bObject\.definePropert(y|ies)\s*\(\s*(Object|Array|String|Number|Boolean|Function|RegExp|Date|Error|Map|Set|WeakMap|WeakSet|Promise|Symbol|BigInt)\.prototype\b`,
			Message:  "Do not extend native objects",
		},
		// ESLint: no-multi-str — flags multiline strings via backslash continuation.
		// Handles both LF and CRLF line endings.
		"no-multi-str": {
			Severity: SeverityWarn,
			Regex:    `\\\r?\n`,
			Message:  "Unexpected multiline string (use template literals)",
		},
		// ESLint: no-octal-escape — flags octal escape sequences in strings.
		// Excludes \0 (null character) which is a valid escape.
		"no-octal-escape": {
			Severity: SeverityError,
			Regex:    `\\[1-7][0-7]{0,2}|\\0[0-7]{1,2}`,
			Message:  "Octal escape sequences are deprecated",
		},
		// ESLint: no-labels — flags labeled statements.
		// Excludes "default:" (switch case) via character-position alternation.
		"no-labels": {
			Severity: SeverityWarn,
			Regex:    `(?m)^\s*` + noLabelsNotDefault + `\s*:\s*(for|while|do|switch)\b`,
			Message:  "Labeled statements are not allowed",
		},
		// ESLint: no-return-await (deprecated in v8.46.0) — flags return await.
		"no-return-await": {
			Severity: SeverityWarn,
			Regex:    `\breturn\s+await\s`,
			Message:  "Redundant use of `return await`",
		},

		// ── Regex rules (3) ──────────────────────────────────────────────

		// ESLint: no-nonoctal-decimal-escape — flags \8 and \9 escapes.
		"no-nonoctal-decimal-escape": {
			Severity: SeverityError,
			Regex:    `\\[89]`,
			Message:  `Don't use '\8' and '\9' escape sequences in string literals`,
		},
		// ESLint: no-regex-spaces — flags multiple consecutive spaces in regex.
		// Simplified: matches 2+ consecutive spaces between regex delimiters.
		"no-regex-spaces": {
			Severity: SeverityError,
			Regex:    `/[^/]*[^ /]  +[^/]*/`,
			Message:  "Spaces are hard to count. Use {N}.",
		},
		// ESLint: no-control-regex — flags control characters in regex.
		"no-control-regex": {
			Severity: SeverityWarn,
			Regex:    `/[^/]*\\x[01][0-9a-fA-F][^/]*/|/[^/]*\\u00[01][0-9a-fA-F][^/]*/`,
			Message:  "Unexpected control character(s) in regular expression",
		},

		// ── Pattern rules (4) ────────────────────────────────────────────

		// ESLint: no-async-promise-executor — flags async Promise executors.
		"no-async-promise-executor": {
			Severity: SeverityError,
			Pattern:  "new Promise(async ($$$) => $$$)",
			Message:  "Promise executor functions should not be async.",
		},
		// ESLint: no-prototype-builtins — flags direct Object.prototype method calls.
		"no-prototype-builtins": {
			Severity: SeverityError,
			Pattern:  "$$.hasOwnProperty($$$)",
			Message:  "Do not access Object.prototype method 'hasOwnProperty' from target object.",
		},
		// ESLint: no-new-native-nonconstructor — flags new Symbol/BigInt.
		"no-new-native-nonconstructor": {
			Severity: SeverityError,
			Pattern:  "new Symbol($$$)",
			Message:  "'Symbol' cannot be invoked as a constructor.",
		},
		// ESLint: no-obj-calls — flags calling Math/JSON/Reflect/Atomics/Intl.
		"no-obj-calls": {
			Severity: SeverityError,
			Pattern:  "Math($$$)",
			Message:  "'Math' is not a function.",
		},

		// ── Structural AST rules (6) ────────────────────────────────────

		// ESLint: no-case-declarations — flags lexical decls in case clauses.
		"no-case-declarations": {
			Severity: SeverityError,
			AST:      &ASTMatcher{Kind: "lexical_declaration", Parent: &ASTMatcher{Kind: "switch_case"}},
			Message:  "Unexpected lexical declaration in case clause.",
		},
		// ESLint: no-octal — flags octal literals (custom Go checker).
		"no-octal": {
			Severity: SeverityError,
			Builtin:  true,
			Message:  "Octal literals should not be used.",
		},
		// ESLint: no-shadow-restricted-names — flags shadowing of globals.
		"no-shadow-restricted-names": {
			Severity: SeverityError,
			Builtin:  true,
			Message:  "Shadowing of global property.",
		},

		// ── Custom Go built-in rules (19) ────────────────────────────────
		// These are dispatched by the engine's builtin checker registry.
		// Builtin: true marks them as Go-implemented checkers.

		"no-empty-pattern": {
			Severity: SeverityError,
			Builtin:  true,
			Message:  "Unexpected empty object pattern.",
		},
		"no-empty-static-block": {
			Severity: SeverityError,
			Builtin:  true,
			Message:  "Unexpected empty static block.",
		},
		"no-compare-neg-zero": {
			Severity: SeverityError,
			Builtin:  true,
			Message:  "Do not use the operator to compare against -0.",
		},
		"no-delete-var": {
			Severity: SeverityError,
			Builtin:  true,
			Message:  "Variables should not be deleted.",
		},
		"no-unsafe-negation": {
			Severity: SeverityError,
			Builtin:  true,
			Message:  "Unexpected negating the left operand of operator.",
		},
		"valid-typeof": {
			Severity: SeverityError,
			Builtin:  true,
			Message:  "Invalid typeof comparison value.",
		},
		"use-isnan": {
			Severity: SeverityError,
			Builtin:  true,
			Message:  "Use the isNaN function to compare with NaN.",
		},
		"no-useless-catch": {
			Severity: SeverityWarn,
			Builtin:  true,
			Message:  "Unnecessary catch clause.",
		},
		"no-sparse-arrays": {
			Severity: SeverityWarn,
			Builtin:  true,
			Message:  "Unexpected comma in middle of array.",
		},
		"no-dupe-keys": {
			Severity: SeverityError,
			Builtin:  true,
			Message:  "Duplicate key.",
		},
		"no-duplicate-case": {
			Severity: SeverityError,
			Builtin:  true,
			Message:  "Duplicate case label.",
		},
		"no-self-assign": {
			Severity: SeverityError,
			Builtin:  true,
			Message:  "Self-assignment.",
		},
		"no-empty": {
			Severity: SeverityError,
			Builtin:  true,
			Message:  "Empty block statement.",
		},
		"no-unsafe-finally": {
			Severity: SeverityError,
			Builtin:  true,
			Message:  "Unsafe usage in finally block.",
		},
		"for-direction": {
			Severity: SeverityError,
			Builtin:  true,
			Message:  "The update clause in this loop moves the variable in the wrong direction.",
		},
		"no-setter-return": {
			Severity: SeverityError,
			Builtin:  true,
			Message:  "Setter cannot return a value.",
		},
		"no-extra-boolean-cast": {
			Severity: SeverityError,
			Builtin:  true,
			Message:  "Redundant double negation.",
		},
		"require-yield": {
			Severity: SeverityWarn,
			Builtin:  true,
			Message:  "This generator function does not have 'yield'.",
		},
		"no-cond-assign": {
			Severity: SeverityError,
			Builtin:  true,
			Message:  "Unexpected assignment within a condition.",
		},
	}
}

// RecommendedConfig returns a Config populated with all built-in rules.
// This is the zero-config fallback when no .ralfrc file is found.
func RecommendedConfig() *Config {
	return &Config{
		Rules: BuiltinRules(),
	}
}
