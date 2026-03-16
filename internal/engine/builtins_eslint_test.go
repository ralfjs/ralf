package engine

import (
	"context"
	"testing"

	"github.com/Hideart/ralf/internal/config"
)

// TestBuiltinRules_ESLintParity verifies each built-in regex rule against
// cases derived from ESLint documentation. Each subtest exercises true
// positives (ESLint would flag), true negatives (ESLint would not flag),
// and known edge cases.
func TestBuiltinRules_ESLintParity(t *testing.T) {
	builtins := config.BuiltinRules()

	cases := []struct {
		rule      string
		shouldHit []string // each snippet MUST produce ≥1 diagnostic (may be multi-line)
		shouldOK  []string // each snippet MUST produce 0 diagnostics
	}{
		{
			rule: "no-var",
			shouldHit: []string{
				"var x = 1;",
				"var y;",
				"for (var i = 0; i < 10; i++) {}",
			},
			shouldOK: []string{
				"const x = 1;",
				"let y = 2;",
				// "variable" contains "var" but not as a keyword
				"const variable = 1;",
				// "variants" contains "var" prefix
				"const variants = [];",
			},
		},
		{
			rule: "no-console",
			shouldHit: []string{
				"console.log('hi');",
				"console.warn('w');",
				"console.error('e');",
				"console.info('i');",
				"console.debug('d');",
				"console.trace('t');",
				"console.dir(obj);",
				"console.dirxml(node);",
				"console.table(data);",
				"console.time('label');",
				"console.timeEnd('label');",
				"console.timeLog('label');",
				"console.timeStamp('label');",
				"console.assert(false);",
				"console.clear();",
				"console.count('label');",
				"console.countReset('label');",
				"console.group('g');",
				"console.groupCollapsed('g');",
				"console.groupEnd();",
				"console.profile('p');",
				"console.profileEnd('p');",
			},
			shouldOK: []string{
				// custom logger, not global console
				"logger.log('ok');",
				"myConsole.log('ok');",
				// reading console without calling
				"const c = console;",
				// non-standard method not in list
				"console.banana('x');",
			},
		},
		{
			rule: "no-eval",
			shouldHit: []string{
				`eval("code");`,
				`eval ("code");`,
				`window.eval("code");`,
				`global.eval("code");`,
				`globalThis.eval("code");`,
				// indirect eval via comma operator
				`(0, eval)("code");`,
				`(0,eval)("code");`,
			},
			shouldOK: []string{
				// "evaluate" starts with eval but is a different word
				"evaluate(x);",
				"const evaluation = 1;",
				// comma expression with other function
				"(0, myFunc)(x);",
				// indirect eval without call — ESLint flags this but
				// regex only flags when actually invoked
				"const f = (0, eval);",
			},
		},
		{
			rule: "no-debugger",
			shouldHit: []string{
				"debugger;",
				"  debugger;",
				"if (x) debugger;",
			},
			shouldOK: []string{
				`const debug = true;`,
				`const x = "the debug keyword";`,
				// "debuggerEnabled" is not "debugger"
				"const debuggerEnabled = false;",
			},
		},
		{
			rule: "no-alert",
			shouldHit: []string{
				// ESLint flags all three
				`alert("hi");`,
				`confirm("sure?");`,
				`prompt("name?");`,
				`window.alert("hi");`,
			},
			shouldOK: []string{
				"const alertMsg = 'hi';",
				"const confirmed = true;",
				"const prompted = 'val';",
				// method on object — regex will match, ESLint wouldn't
				// (scope-aware). This is an accepted regex limitation.
			},
		},
		{
			rule: "no-inner-html",
			shouldHit: []string{
				`el.innerHTML = "<b>x</b>";`,
				`document.body.innerHTML = "";`,
				`el.innerHTML  =  val;`,
			},
			shouldOK: []string{
				// reading innerHTML is fine
				"const html = el.innerHTML;",
				"el.textContent = 'safe';",
				// outerHTML assignment — not covered (RALF-original rule)
				"el.outerHTML = '<b>x</b>';",
			},
		},
		{
			rule: "no-with",
			shouldHit: []string{
				"with (obj) { x = 1; }",
				"with(Math) { y = cos(0); }",
			},
			shouldOK: []string{
				"const obj = { x: 1 };",
				// "withdraw" contains "with" prefix
				"withdraw(100);",
			},
		},
		{
			rule: "no-caller",
			shouldHit: []string{
				"arguments.caller;",
				"arguments.callee;",
				"const x = arguments.callee;",
			},
			shouldOK: []string{
				"arguments.length;",
				"arguments[0];",
				"Array.from(arguments);",
			},
		},
		{
			rule: "no-implied-eval",
			shouldHit: []string{
				`setTimeout("alert('hi')", 100);`,
				`setInterval("x++", 1000);`,
				`execScript("code");`,
				"setTimeout('code', 0);",
				"setInterval(`code`, 0);",
				`window.setTimeout("alert()", 0);`,
			},
			shouldOK: []string{
				"setTimeout(function() {}, 100);",
				"setTimeout(() => {}, 100);",
				"setTimeout(myFunc, 100);",
				"setInterval(tick, 1000);",
			},
		},
		{
			rule: "no-new-wrappers",
			shouldHit: []string{
				`new String("hi");`,
				"new Number(42);",
				"new Boolean(true);",
				"new  String('hi');",
				// ESLint also flags new without parens
				"new String;",
				"new Number;",
				"new Boolean;",
			},
			shouldOK: []string{
				// calling without new is fine (type coercion)
				"String(42);",
				"Number('42');",
				"Boolean(0);",
				// other constructors not covered
				"new Object();",
				"new Array();",
				// similar names that aren't exact matches
				"new StringBuilder();",
				"new BooleanFlag();",
			},
		},
		{
			rule: "no-proto",
			shouldHit: []string{
				"obj.__proto__;",
				"obj.__proto__ = null;",
				"foo.bar.__proto__;",
				// ESLint also catches bracket notation
				`obj["__proto__"];`,
				`obj['__proto__'];`,
			},
			shouldOK: []string{
				"Object.getPrototypeOf(obj);",
				"Object.setPrototypeOf(obj, null);",
				// __proto__ in object literal — ESLint allows this
				"const c = { __proto__: proto };",
			},
		},
		{
			rule: "no-iterator",
			shouldHit: []string{
				"obj.__iterator__ = fn;",
				"foo.__iterator__;",
				// ESLint also catches bracket notation
				`obj["__iterator__"];`,
				`obj['__iterator__'];`,
			},
			shouldOK: []string{
				"obj[Symbol.iterator] = fn;",
				// standalone identifier — no dot prefix
				"const __iterator__ = 1;",
			},
		},
		{
			rule: "no-new-func",
			shouldHit: []string{
				// ESLint flags both new Function() and Function()
				`new Function("return 1");`,
				`Function("return 1");`,
				`new Function("a", "return a");`,
			},
			shouldOK: []string{
				"function foo() {}",
				"const f = function() {};",
				"const g = () => 1;",
				// class named MyFunction — "Function" is preceded by "My"
				// so \bFunction won't match
				"class MyFunction {}",
			},
		},
		{
			rule: "no-void",
			shouldHit: []string{
				"void 0;",
				"void(0);",
				"const x = void fn();",
				"const x = void(0);",
				"return void 0;",
				"(void 0);",
				"!void 0;",
				"x || void 0;",
				"x ? void 0 : 1;",
				"  void 0;",
				"[void 0]",
				"x, void 0",
				"const f = () => void 0;",
				"const f = () => void doStuff();",
			},
			shouldOK: []string{
				"const x = undefined;",
				// "avoid" contains "void" but word boundary prevents match
				"avoid(x);",
				// TS type annotations — must NOT be flagged
				"function f(): void {}",
				"let x: void;",
				"async function f(): Promise<void> {}",
				"type F = () => void;",
				"const f: () => void = fn;",
			},
		},
		{
			rule: "no-script-url",
			shouldHit: []string{
				`const a = "javascript:alert(1)";`,
				`const b = 'javascript:void(0)';`,
				"const c = `javascript:doIt()`;",
				// case-insensitive — URL schemes are case-insensitive
				`const d = "JavaScript:alert(1)";`,
				`const e = "JAVASCRIPT:alert(1)";`,
			},
			shouldOK: []string{
				`const a = "https://example.com";`,
				`const b = 'http://example.com';`,
				// no quote before javascript:
				"// javascript: in a comment",
			},
		},
		{
			rule: "no-extend-native",
			shouldHit: []string{
				"Object.prototype.foo = fn;",
				"Array.prototype.last = fn;",
				"String.prototype.trim2 = fn;",
				"Number.prototype.toX = fn;",
				"Boolean.prototype.toggle = fn;",
				"Function.prototype.bind2 = fn;",
				"RegExp.prototype.test2 = fn;",
				"Date.prototype.format = fn;",
				"Error.prototype.toJSON = fn;",
				"Map.prototype.getOrDefault = fn;",
				"Set.prototype.addAll = fn;",
				"Promise.prototype.finally2 = fn;",
				"Symbol.prototype.desc = fn;",
				"BigInt.prototype.toX = fn;",
				// defineProperty form
				`Object.defineProperty(Array.prototype, "last", {});`,
				`Object.defineProperties(String.prototype, {});`,
			},
			shouldOK: []string{
				// reading prototype is fine
				"const x = Array.prototype.slice.call(args);",
				"const p = Object.prototype;",
				// defineProperty on custom object
				`Object.defineProperty(myObj, "name", {});`,
				// extending own class
				"class MyArray extends Array {}",
			},
		},
		{
			rule: "no-multi-str",
			shouldHit: []string{
				// backslash immediately before newline
				"const a = 'line1\\\nline2';",
			},
			shouldOK: []string{
				// template literal with actual newline (no backslash)
				"const a = `line1\nline2`;",
				// concatenation
				`const b = "a" + "b";`,
				// single line
				`const c = "hello";`,
			},
		},
		{
			rule: "no-octal-escape",
			shouldHit: []string{
				`const a = "\251";`, // \251 — octal
				`const b = "\1";`,   // \1 — octal
				`const c = "\77";`,  // \77 — octal
				`const d = "\00";`,  // \00 — octal (not bare \0)
				`const e = "\377";`, // \377 — octal
				`const f = "\01";`,  // \01 — octal
				`const g = "\123";`, // \123 — octal
				`const h = "\7";`,   // \7 — octal
				`const i = "\07";`,  // \07 — octal
				`const j = "\003";`, // \003 — octal
				`const k = "\100";`, // \100 — octal
			},
			shouldOK: []string{
				`const a = "\0";`,     // bare \0 is null char, not octal
				`const b = "\n";`,     // \n — not octal
				`const c = "\t";`,     // \t — not octal
				`const d = "\u00A9";`, // unicode — not octal
				`const e = "\xA9";`,   // hex — not octal
				`const f = "hello";`,  // no escapes
			},
		},
		{
			rule: "no-labels",
			shouldHit: []string{
				"outer: for (;;) { break outer; }",
				"loop: while (true) { break loop; }",
				"block: do { break block; } while (false);",
				"sw: switch (x) { case 1: break sw; }",
				// $ and _ prefixed labels
				"$loop: for (;;) {}",
				"_loop: for (;;) {}",
				// labels similar to "default" but not exactly it
				"defaults: for (;;) {}",
				"Default: for (;;) {}",
				"defaultValue: for (;;) {}",
			},
			shouldOK: []string{
				"for (;;) { break; }",
				"while (true) { break; }",
				// object property that looks like a label but isn't at line start
				`const obj = { loop: "value" };`,
				// switch default clause — must NOT be flagged
				"  default: for (;;) {}",
				"  default: while (true) {}",
				"  default: do {} while (false);",
				"  default: switch (x) {}",
			},
		},
		{
			rule: "no-return-await",
			shouldHit: []string{
				"return await foo();",
				"return  await  bar();",
				"  return await baz();",
			},
			shouldOK: []string{
				"return foo();",
				"const x = await foo();",
				"return x;",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.rule, func(t *testing.T) {
			rule, ok := builtins[tc.rule]
			if !ok {
				t.Fatalf("rule %q not found in builtins", tc.rule)
			}

			cfg := &config.Config{
				Rules: map[string]config.RuleConfig{
					tc.rule: rule,
				},
			}
			eng, errs := New(cfg)
			if len(errs) > 0 {
				t.Fatalf("compile errors: %v", errs)
			}

			for _, code := range tc.shouldHit {
				diags := eng.LintFile(context.Background(), "test.js", []byte(code))
				if len(diags) == 0 {
					t.Errorf("should flag: %s", code)
				}
			}

			for _, code := range tc.shouldOK {
				diags := eng.LintFile(context.Background(), "test.js", []byte(code))
				if len(diags) > 0 {
					t.Errorf("false positive: %s → %s", code, diags[0].Message)
				}
			}
		})
	}
}
