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

// ProductMeta holds internal metadata from a product's defaults.yml (keys starting with _).
type ProductMeta struct {
	GUIURL        string // _gui_url template
	LoginUser     string // _login_user template
	LoginPassword string // _login_password template
	DownloadFile  string // _download_file — remote path to fetch
	DownloadName  string // _download_name — filename for download
}

// LoadProductMeta reads the internal _-prefixed keys from a product's defaults.yml.
func LoadProductMeta(deployDir, productName string) ProductMeta {
	path := fmt.Sprintf("%s/products/%s/ansible/defaults.yml", deployDir, productName)
	data, err := os.ReadFile(path)
	if err != nil {
		return ProductMeta{}
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return ProductMeta{}
	}

	var meta ProductMeta
	if v, ok := raw["_gui_url"]; ok {
		meta.GUIURL = fmt.Sprintf("%v", v)
	}
	if v, ok := raw["_login_user"]; ok {
		meta.LoginUser = fmt.Sprintf("%v", v)
	}
	if v, ok := raw["_login_password"]; ok {
		meta.LoginPassword = fmt.Sprintf("%v", v)
	}
	if v, ok := raw["_download_file"]; ok {
		meta.DownloadFile = fmt.Sprintf("%v", v)
	}
	if v, ok := raw["_download_name"]; ok {
		meta.DownloadName = fmt.Sprintf("%v", v)
	}
	return meta
}

// ResolveTemplate replaces {host} and config variable placeholders in a template string.
func ResolveTemplate(tpl string, hostIP string, config map[string]string) string {
	if tpl == "" {
		return ""
	}
	result := strings.Replace(tpl, "{host}", hostIP, 1)
	for k, v := range config {
		result = strings.Replace(result, "{"+k+"}", v, -1)
	}
	return result
}
