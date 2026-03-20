package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Hideart/ralf/internal/config"
	"gopkg.in/yaml.v3"
)

// eslintToRalf maps ESLint core rule names to ralf equivalents.
// Nearly all of ralf's 61 rules have 1:1 ESLint name matches.
var eslintToRalf = map[string]string{
	"no-var":                       "no-var",
	"no-console":                   "no-console",
	"no-eval":                      "no-eval",
	"no-debugger":                  "no-debugger",
	"no-alert":                     "no-alert",
	"no-with":                      "no-with",
	"no-caller":                    "no-caller",
	"no-implied-eval":              "no-implied-eval",
	"no-new-wrappers":              "no-new-wrappers",
	"no-proto":                     "no-proto",
	"no-iterator":                  "no-iterator",
	"no-new-func":                  "no-new-func",
	"no-void":                      "no-void",
	"no-script-url":                "no-script-url",
	"no-extend-native":             "no-extend-native",
	"no-multi-str":                 "no-multi-str",
	"no-octal-escape":              "no-octal-escape",
	"no-labels":                    "no-labels",
	"no-return-await":              "no-return-await",
	"no-nonoctal-decimal-escape":   "no-nonoctal-decimal-escape",
	"no-regex-spaces":              "no-regex-spaces",
	"no-control-regex":             "no-control-regex",
	"no-async-promise-executor":    "no-async-promise-executor",
	"no-prototype-builtins":        "no-prototype-builtins",
	"no-new-native-nonconstructor": "no-new-native-nonconstructor",
	"no-obj-calls":                 "no-obj-calls",
	"no-case-declarations":         "no-case-declarations",
	"no-octal":                     "no-octal",
	"no-shadow-restricted-names":   "no-shadow-restricted-names",
	"no-empty-pattern":             "no-empty-pattern",
	"no-empty-static-block":        "no-empty-static-block",
	"no-compare-neg-zero":          "no-compare-neg-zero",
	"no-delete-var":                "no-delete-var",
	"no-unsafe-negation":           "no-unsafe-negation",
	"valid-typeof":                 "valid-typeof",
	"use-isnan":                    "use-isnan",
	"no-useless-catch":             "no-useless-catch",
	"no-sparse-arrays":             "no-sparse-arrays",
	"no-dupe-keys":                 "no-dupe-keys",
	"no-duplicate-case":            "no-duplicate-case",
	"no-self-assign":               "no-self-assign",
	"no-empty":                     "no-empty",
	"no-unsafe-finally":            "no-unsafe-finally",
	"for-direction":                "for-direction",
	"no-setter-return":             "no-setter-return",
	"no-extra-boolean-cast":        "no-extra-boolean-cast",
	"require-yield":                "require-yield",
	"no-cond-assign":               "no-cond-assign",
	"no-self-compare":              "no-self-compare",
	"eqeqeq":                       "eqeqeq",
	"no-empty-character-class":     "no-empty-character-class",
	"no-dupe-class-members":        "no-dupe-class-members",
	"no-dupe-args":                 "no-dupe-args",
	"no-constructor-return":        "no-constructor-return",
	"no-inner-declarations":        "no-inner-declarations",
	"no-unsafe-optional-chaining":  "no-unsafe-optional-chaining",
	"no-constant-condition":        "no-constant-condition",
	"no-loss-of-precision":         "no-loss-of-precision",
	"getter-return":                "getter-return",
	"no-fallthrough":               "no-fallthrough",
}

// eslintJSConfigs are JS-based ESLint config files that require evaluation.
var eslintJSConfigs = []string{
	".eslintrc.js", ".eslintrc.cjs",
	"eslint.config.js", "eslint.config.mjs", "eslint.config.cjs",
}

func migrateESLint(dir string) (*config.Config, *migrationReport, error) {
	path, err := findESLintConfig(dir)
	if err != nil {
		return nil, nil, err
	}

	data, err := os.ReadFile(path) //nolint:gosec // user-supplied config path
	if err != nil {
		return nil, nil, fmt.Errorf("read ESLint config %s: %w", path, err)
	}

	// Parse into interface{} for YAML compatibility, then extract rules.
	var parsed map[string]interface{}

	ext := filepath.Ext(path)
	switch ext {
	case ".json":
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, nil, fmt.Errorf("parse ESLint config %s: %w", path, err)
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &parsed); err != nil {
			return nil, nil, fmt.Errorf("parse ESLint config %s: %w", path, err)
		}
	default:
		return nil, nil, fmt.Errorf("unsupported ESLint config format: %s", ext)
	}

	rulesRaw, _ := parsed["rules"].(map[string]interface{})
	var ignorePatterns []string
	switch ips := parsed["ignorePatterns"].(type) {
	case []interface{}:
		for _, v := range ips {
			if s, ok := v.(string); ok {
				ignorePatterns = append(ignorePatterns, s)
			}
		}
	case string:
		if ips != "" {
			ignorePatterns = append(ignorePatterns, ips)
		}
	}

	// Start from all builtins at their default severities.
	rules := config.BuiltinRules()

	report := &migrationReport{
		source:     "eslint",
		sourceFile: filepath.Base(path),
	}

	for eslintName, rawVal := range rulesRaw {
		sev, ok := parseESLintSeverityAny(rawVal)
		if !ok {
			continue
		}

		ralfName, mapped := eslintToRalf[eslintName]
		if !mapped {
			report.unsupportedRules = append(report.unsupportedRules, eslintName)
			continue
		}

		if rule, exists := rules[ralfName]; exists {
			rule.Severity = sev
			rules[ralfName] = rule
			report.migratedCount++
		} else {
			// Mapping exists but builtin rule does not; treat as unsupported.
			report.unsupportedRules = append(report.unsupportedRules, eslintName)
		}
	}

	cfg := &config.Config{Rules: rules}
	if len(ignorePatterns) > 0 {
		cfg.Ignores = ignorePatterns
		report.ignoreCount = len(ignorePatterns)
	}

	return cfg, report, nil
}

func findESLintConfig(dir string) (string, error) {
	// Search for supported JSON/YAML configs first.
	names := []string{".eslintrc.json", ".eslintrc.yaml", ".eslintrc.yml"}
	for _, name := range names {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	// No JSON/YAML found — check if a JS config exists and give helpful error.
	for _, name := range eslintJSConfigs {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return "", fmt.Errorf(
				"JavaScript ESLint config found (%s). Export to JSON first:\n  npx eslint --print-config . > .eslintrc.json",
				name,
			)
		}
	}

	return "", fmt.Errorf("no ESLint config found (searched: .eslintrc.json, .eslintrc.yaml, .eslintrc.yml)")
}

// parseESLintSeverityAny extracts severity from a generic interface{} value
// (works with both JSON and YAML parsed data).
func parseESLintSeverityAny(v interface{}) (config.Severity, bool) {
	switch val := v.(type) {
	case string:
		return parseSeverityString(val)
	case float64:
		return eslintSevNumber(val)
	case int:
		return eslintSevNumber(float64(val))
	case []interface{}:
		if len(val) > 0 {
			return parseESLintSeverityAny(val[0])
		}
	}
	return "", false
}

// parseESLintSeverity extracts the severity from an ESLint rule value.
// Handles: "off"/"warn"/"error", 0/1/2, ["error", {...}], [2, {...}].
func parseESLintSeverity(raw json.RawMessage) (config.Severity, bool) {
	// Try string first: "off", "warn", "error".
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return parseSeverityString(s)
	}

	// Try number: 0, 1, 2.
	var n float64
	if json.Unmarshal(raw, &n) == nil {
		return eslintSevNumber(n)
	}

	// Try array: ["error", {...}] or [2, {...}].
	var arr []json.RawMessage
	if json.Unmarshal(raw, &arr) == nil && len(arr) > 0 {
		return parseESLintSeverity(arr[0])
	}

	return "", false
}

func eslintSevNumber(n float64) (config.Severity, bool) {
	switch int(n) {
	case 0:
		return config.SeverityOff, true
	case 1:
		return config.SeverityWarn, true
	case 2:
		return config.SeverityError, true
	}
	return "", false
}
