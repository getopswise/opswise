package runner

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultVar represents a single configurable variable from a product's defaults.yml.
type DefaultVar struct {
	Key     string
	Value   string
	Options []string // non-empty when the YAML value is a list (first item is the default)
}

// LoadProductDefaults reads a product's defaults.yml and returns the variables
// as an ordered slice of key-value pairs.
func LoadProductDefaults(deployDir, productName string) []DefaultVar {
	path := fmt.Sprintf("%s/products/%s/ansible/defaults.yml", deployDir, productName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil
	}

	vars := make([]DefaultVar, 0, len(raw))
	for k, v := range raw {
		// Skip internal keys (prefixed with _)
		if strings.HasPrefix(k, "_") {
			continue
		}
		dv := DefaultVar{Key: k}
		if list, ok := v.([]interface{}); ok && len(list) > 0 {
			for _, item := range list {
				dv.Options = append(dv.Options, fmt.Sprintf("%v", item))
			}
			dv.Value = dv.Options[0]
		} else {
			dv.Value = fmt.Sprintf("%v", v)
		}
		vars = append(vars, dv)
	}

	sort.Slice(vars, func(i, j int) bool {
		return vars[i].Key < vars[j].Key
	})

	return vars
}

// LoadProductGUIURL reads the _gui_url template from a product's defaults.yml.
// Returns empty string if the product has no GUI URL defined.
func LoadProductGUIURL(deployDir, productName string) string {
	path := fmt.Sprintf("%s/products/%s/ansible/defaults.yml", deployDir, productName)
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return ""
	}

	if v, ok := raw["_gui_url"]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// ResolveGUIURL replaces {host} and config variable placeholders in a GUI URL template.
func ResolveGUIURL(template string, hostIP string, config map[string]string) string {
	if template == "" {
		return ""
	}
	result := strings.Replace(template, "{host}", hostIP, 1)
	for k, v := range config {
		result = strings.Replace(result, "{"+k+"}", v, -1)
	}
	return result
}
