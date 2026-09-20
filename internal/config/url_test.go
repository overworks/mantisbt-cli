package config

import (
	"strings"
	"testing"
)

func TestParseURLRejectsAmbiguousBases(t *testing.T) {
	for _, raw := range []string{
		"", "mantis.example", "/mantis", "//mantis.example", "ftp://mantis.example", "https:///mantis",
		"https://", "http://[invalid", " https://mantis.example", "https://mantis.example ",
		"https://mantis.example/?secret=value", "https://mantis.example/?", "https://mantis.example/#", "https://mantis.example/#dashboard",
		"https://user:secret@mantis.example", "https://mantis.example:0", "https://mantis.example:65536", "https://mantis.example:",
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := ParseURL(raw); err == nil {
				t.Fatal("invalid URL accepted")
			} else if strings.Contains(err.Error(), "secret") {
				t.Fatal("URL error exposed credentials or query data")
			}
		})
	}
}

func TestParseURLAcceptsInstallationPathsAndLocalServers(t *testing.T) {
	for _, raw := range []string{
		"https://mantis.example", "http://localhost:8080/mantisbt/", "https://[::1]:443/api/rest/index.php",
		"https://mantis.example/api/rest/", "https://mantis.example/mantis%20bt/api/rest", "https://mantis.example/folder%2Fname/",
	} {
		if _, err := ParseURL(raw); err != nil {
			t.Errorf("%s: %v", raw, err)
		}
	}
}

func TestLoadValidatesFlagAndEnvironmentURLs(t *testing.T) {
	t.Setenv("MANTISBT_TOKEN", "test-token")
	t.Setenv("MANTISBT_URL", "https://mantis.example/#dashboard")
	if _, err := Load("", "", false); err == nil {
		t.Fatal("invalid environment URL accepted")
	}
	if _, err := Load("https://mantis.example", "", false); err != nil {
		t.Fatalf("valid flag must override invalid environment: %v", err)
	}
	if _, err := Load("https://mantis.example/?token=secret", "", false); err == nil {
		t.Fatal("invalid flag URL accepted")
	}
}
