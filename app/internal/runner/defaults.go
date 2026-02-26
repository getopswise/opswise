package runner

import (
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

// DefaultVar represents a single configurable variable from a product's defaults.yml.
type DefaultVar struct {
	Key   string
	Value string
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
		vars = append(vars, DefaultVar{
			Key:   k,
			Value: fmt.Sprintf("%v", v),
		})
	}

	sort.Slice(vars, func(i, j int) bool {
		return vars[i].Key < vars[j].Key
	})

	return vars
}
