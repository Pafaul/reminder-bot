package reminder

import (
	"errors"
	"os"
	"strings"
)

func init() {
	_, err := os.Stat(".env")
	if errors.Is(err, os.ErrNotExist) {
		panic("Missing .env file")
	}

	envContent, err := os.ReadFile(".env")
	if err != nil {
		panic("Error reading .env file")
	}

	if len(envContent) == 0 {
		panic("Empty .env file")
	}

	lines := strings.Split(string(envContent), "\n")

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if err := os.Setenv(key, value); err != nil {
				panic("Error setting environment variable: " + key)
			}
		}
	}
}
