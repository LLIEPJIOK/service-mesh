package cmd

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

func ParseToEnvMap(data []byte) (map[string]string, error) {
	var content map[string]any

	if err := yaml.Unmarshal(data, &content); err != nil {
		return nil, fmt.Errorf("failed to parse yaml: %w", err)
	}

	envMap := make(map[string]string)
	flatten(content, "", envMap)

	return envMap, nil
}

func flatten(m map[string]any, prefix string, envMap map[string]string) {
	for key, value := range m {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "_" + key
		}

		switch v := value.(type) {
		case map[string]any:
			flatten(v, fullKey, envMap)

		case []any:
			s := ""

			for i, elem := range v {
				if i > 0 {
					s += ","
				}

				s += fmt.Sprint(elem)
			}

			envMap[toEnvKey(fullKey)] = s

		default:
			envMap[toEnvKey(fullKey)] = fmt.Sprint(v)
		}
	}
}

func toEnvKey(key string) string {
	return strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
}
