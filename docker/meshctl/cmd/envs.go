package cmd

import (
	"fmt"
	"os"
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

// getEnvMap returns environment variables as a map from config file or defaults.
func getEnvMap(config string, def string) (map[string]string, error) {
	var (
		data []byte
		err  error
	)

	if config != "" {
		data, err = os.ReadFile(config)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
	} else {
		data = []byte(def)
	}

	return ParseToEnvMap(data)
}

// getEnvs returns environment variables as docker -e flags from config file or defaults.
func getEnvs(config string, def string) ([]string, error) {
	mp, err := getEnvMap(config, def)
	if err != nil {
		return nil, err
	}

	params := make([]string, 0, 2*len(mp))
	for k, v := range mp {
		params = append(params, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	return params, nil
}
