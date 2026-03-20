package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Hideart/ralf/internal/config"
)

// biomeToRalf maps Biome "category/ruleName" to ralf rule names.
var biomeToRalf = map[string]string{
	// correctness
	"correctness/noConstructorReturn":           "no-constructor-return",
	"correctness/noEmptyCharacterClassInRegex":  "no-empty-character-class",
	"correctness/noEmptyPattern":                "no-empty-pattern",
	"correctness/noGlobalObjectCalls":           "no-obj-calls",
	"correctness/noInnerDeclarations":           "no-inner-declarations",
	"correctness/noInvalidBuiltinInstantiation": "no-new-native-nonconstructor",
	"correctness/noNonoctalDecimalEscape":       "no-nonoctal-decimal-escape",
	"correctness/noPrecisionLoss":               "no-loss-of-precision",
	"correctness/noSelfAssign":                  "no-self-assign",
	"correctness/noSetterReturn":                "no-setter-return",
	"correctness/noSwitchDeclarations":          "no-case-declarations",
	"correctness/noUnsafeFinally":               "no-unsafe-finally",
	"correctness/noUnsafeOptionalChaining":      "no-unsafe-optional-chaining",
	"correctness/useIsNan":                      "use-isnan",
	"correctness/useValidForDirection":          "for-direction",
	"correctness/useValidTypeof":                "valid-typeof",
	"correctness/useYield":                      "require-yield",
	"correctness/noConstantCondition":           "no-constant-condition",
	"correctness/useGetterReturn":               "getter-return",

	// suspicious
	"suspicious/noAsyncPromiseExecutor":     "no-async-promise-executor",
	"suspicious/noCompareNegZero":           "no-compare-neg-zero",
	"suspicious/noControlCharactersInRegex": "no-control-regex",
	"suspicious/noDebugger":                 "no-debugger",
	"suspicious/noDoubleEquals":             "eqeqeq",
	"suspicious/noDuplicateCase":            "no-duplicate-case",
	"suspicious/noDuplicateClassMembers":    "no-dupe-class-members",
	"suspicious/noDuplicateObjectKeys":      "no-dupe-keys",
	"suspicious/noDuplicateParameters":      "no-dupe-args",
	"suspicious/noEmptyBlockStatements":     "no-empty",
	"suspicious/noExtraBooleanCast":         "no-extra-boolean-cast",
	"suspicious/noFallthroughSwitchClause":  "no-fallthrough",
	"suspicious/noPrototypeBuiltins":        "no-prototype-builtins",
	"suspicious/noSelfCompare":              "no-self-compare",
	"suspicious/noShadowRestrictedNames":    "no-shadow-restricted-names",
	"suspicious/noSparseArray":              "no-sparse-arrays",
	"suspicious/noUnsafeNegation":           "no-unsafe-negation",
	"suspicious/noUselessCatch":             "no-useless-catch",

	// style
	"style/noVar":       "no-var",
	"style/noArguments": "no-caller",
	"style/useConsistentBuiltinInstantiation": "no-new-wrappers",

	// complexity
	"complexity/noExtraBooleanCast":      "no-extra-boolean-cast",
	"complexity/noUselessCatch":          "no-useless-catch",
	"complexity/noVoid":                  "no-void",
	"complexity/noAdjacentSpacesInRegex": "no-regex-spaces",

	// security
	"security/noDangerouslySetInnerHtml": "no-inner-html",
	"security/noGlobalEval":              "no-eval",
}

func migrateBiome(dir string) (*config.Config, *migrationReport, error) {
	path, err := findBiomeConfig(dir)
	if err != nil {
		return nil, nil, err
	}

	data, err := os.ReadFile(path) //nolint:gosec // user-supplied config path
	if err != nil {
		return nil, nil, fmt.Errorf("read Biome config %s: %w", path, err)
	}

	// Strip JSONC comments for .jsonc files.
	if filepath.Ext(path) == ".jsonc" {
		data = stripJSONC(data)
	}

	var raw struct {
		Linter struct {
			Rules map[string]map[string]json.RawMessage `json:"rules"`
		} `json:"linter"`
		Files struct {
			Ignore []string `json:"ignore"`
		} `json:"files"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, nil, fmt.Errorf("parse Biome config %s: %w", path, err)
	}

	rules := config.BuiltinRules()

	report := &migrationReport{
		source:     "biome",
		sourceFile: filepath.Base(path),
	}

	for category, ruleMap := range raw.Linter.Rules {
		for name, rawVal := range ruleMap {
			biomeKey := category + "/" + name
			sev, ok := parseBiomeSeverity(rawVal)
			if !ok {
				continue
			}

			ralfName, mapped := biomeToRalf[biomeKey]
			if !mapped || ralfName == "" {
				report.unsupportedRules = append(report.unsupportedRules, biomeKey)
				continue
			}

			if rule, exists := rules[ralfName]; exists {
				rule.Severity = sev
				rules[ralfName] = rule
				report.migratedCount++
			} else {
				// Mapping exists but builtin rule is missing; treat as unsupported.
				report.unsupportedRules = append(report.unsupportedRules, biomeKey)
			}
		}
	}

	cfg := &config.Config{Rules: rules}
	if len(raw.Files.Ignore) > 0 {
		cfg.Ignores = raw.Files.Ignore
		report.ignoreCount = len(raw.Files.Ignore)
	}

	return cfg, report, nil
}

func findBiomeConfig(dir string) (string, error) {
	for _, name := range []string{"biome.json", "biome.jsonc"} {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("no Biome config found (searched: biome.json, biome.jsonc)")
}

// parseBiomeSeverity extracts severity from a Biome rule value.
// Handles: "error"/"warn"/"off" and {"level": "error"}.
func parseBiomeSeverity(raw json.RawMessage) (config.Severity, bool) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return parseSeverityString(s)
	}

	var obj struct {
		Level string `json:"level"`
	}
	if json.Unmarshal(raw, &obj) == nil && obj.Level != "" {
		return parseSeverityString(obj.Level)
	}

	return "", false
}

// stripJSONC removes single-line (//) and multi-line (/* */) comments from
// JSONC input so it can be parsed by encoding/json.
func stripJSONC(data []byte) []byte {
	out := make([]byte, 0, len(data))
	i := 0
	for i < len(data) {
		// Inside a string — skip to closing quote.
		if data[i] == '"' {
			j := i + 1
			for j < len(data) {
				if data[j] == '\\' {
					j += 2
					continue
				}
				if data[j] == '"' {
					j++
					break
				}
				j++
			}
			out = append(out, data[i:j]...)
			i = j
			continue
		}

		// Single-line comment.
		if i+1 < len(data) && data[i] == '/' && data[i+1] == '/' {
			for i < len(data) && data[i] != '\n' {
				i++
			}
			continue
		}

		// Multi-line comment.
		if i+1 < len(data) && data[i] == '/' && data[i+1] == '*' {
			i += 2
			for i+1 < len(data) {
				if data[i] == '*' && data[i+1] == '/' {
					i += 2
					break
				}
				i++
			}
			continue
		}

		out = append(out, data[i])
		i++
	}
	return out
}
