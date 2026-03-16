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

// BuiltinRules returns the 20 built-in regex rules. A fresh map is returned
// on every call so callers may mutate it freely.
//
// Each rule is modeled after its ESLint equivalent where one exists. Regex-based
// matching is inherently less precise than AST analysis, so some ESLint
// edge cases (scope awareness, indirect references) are out of scope.
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
		// ESLint: no-eval — flags eval() calls and indirect eval via (0, eval).
		"no-eval": {
			Severity: SeverityError,
			Regex:    `\beval[ \t]*\(|\(\s*0\s*,\s*eval\s*\)`,
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
	}
}

// RecommendedConfig returns a Config populated with all built-in rules.
// This is the zero-config fallback when no .ralfrc file is found.
func RecommendedConfig() *Config {
	return &Config{
		Rules: BuiltinRules(),
	}
}
