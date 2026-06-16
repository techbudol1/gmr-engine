package config

import (
	"bufio"
	"os"
	"strings"
)

func loadDotEnv(paths ...string) {
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			key = strings.TrimSpace(key)
			if key == "" || os.Getenv(key) != "" {
				continue
			}
			value = strings.Trim(strings.TrimSpace(value), `"'`)
			_ = os.Setenv(key, strings.ReplaceAll(value, `\n`, "\n"))
		}
		_ = file.Close()
	}
}
