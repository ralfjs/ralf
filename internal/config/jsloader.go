package config

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// loadJS evaluates a JavaScript config file via goja and returns the parsed Config.
// Supports both CommonJS (module.exports = {...}) and shimmed ES default exports
// (export default {...}).
func loadJS(path string, data []byte) (*Config, error) {
	source := shimExportDefault(string(data))

	vm := goja.New()

	// Set up CommonJS module/exports objects.
	module := vm.NewObject()
	exports := vm.NewObject()
	if err := module.Set("exports", exports); err != nil {
		return nil, fmt.Errorf("config: setup JS VM: %w", err)
	}
	if err := vm.Set("module", module); err != nil {
		return nil, fmt.Errorf("config: setup JS VM: %w", err)
	}
	if err := vm.Set("exports", exports); err != nil {
		return nil, fmt.Errorf("config: setup JS VM: %w", err)
	}

	// Interrupt after 5 seconds to prevent infinite loops.
	timer := time.AfterFunc(5*time.Second, func() {
		vm.Interrupt("config JS evaluation timed out after 5s")
	})
	defer timer.Stop()

	if _, err := vm.RunScript(path, source); err != nil {
		return nil, fmt.Errorf("config: eval JS %s: %w", path, err)
	}

	val := module.Get("exports")
	return exportToConfig(path, val)
}

// shimExportDefault rewrites "export default" to "module.exports =" so that
// goja (ES5.1 CommonJS) can evaluate configs written with ES module syntax.
func shimExportDefault(source string) string {
	return strings.Replace(source, "export default", "module.exports =", 1)
}

// exportToConfig converts a goja value to a *Config via JSON round-trip.
func exportToConfig(path string, val goja.Value) (*Config, error) {
	raw := val.Export()
	if raw == nil {
		return nil, fmt.Errorf("config: JS %s: module.exports is nil or undefined", path)
	}
	if _, ok := raw.(map[string]interface{}); !ok {
		return nil, fmt.Errorf("config: JS %s: module.exports must be an object, got %T", path, raw)
	}

	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("config: JS %s: marshal exports: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: JS %s: unmarshal config: %w", path, err)
	}

	return &cfg, nil
}
