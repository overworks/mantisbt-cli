package config

import (
	"fmt"
	"os"
	"strings"
)

// Config holds the resolved connection settings for a single CLI invocation.
type Config struct {
	URL        string
	Token      string
	JSONOutput bool
}

// Load resolves configuration from explicit flag values, falling back to the
// MANTISBT_URL / MANTISBT_TOKEN environment variables. flagURL and flagToken
// are empty strings when the corresponding flag was not provided.
func Load(flagURL, flagToken string, jsonOutput bool) (Config, error) {
	url := flagURL
	if url == "" {
		url = os.Getenv("MANTISBT_URL")
	}

	token := flagToken
	if token == "" {
		token = os.Getenv("MANTISBT_TOKEN")
	}

	var missing []string
	if url == "" {
		missing = append(missing, "MANTISBT_URL")
	}
	if token == "" {
		missing = append(missing, "MANTISBT_TOKEN")
	}

	if len(missing) > 0 {
		return Config{}, fmt.Errorf(
			"missing configuration: set %s or pass --url/--token",
			strings.Join(missing, ", "),
		)
	}
	if _, err := ParseURL(url); err != nil {
		return Config{}, err
	}

	return Config{URL: url, Token: token, JSONOutput: jsonOutput}, nil
}
