package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestLoadPrecedenceAndOutputMode(t *testing.T) {
	cases := []struct {
		name, flagURL, flagToken, envURL, envToken string
		json                                       bool
		want                                       Config
	}{
		{"environment", "", "", "https://env.example", "env-token", false, Config{"https://env.example", "env-token", false}},
		{"flags", "https://flag.example", "flag-token", "", "", false, Config{"https://flag.example", "flag-token", false}},
		{"flags override environment", "https://flag.example", "flag-token", "https://env.example", "env-token", true, Config{"https://flag.example", "flag-token", true}},
		{"URL flag only", "https://flag.example", "", "https://env.example", "env-token", false, Config{"https://flag.example", "env-token", false}},
		{"token flag only", "", "flag-token", "https://env.example", "env-token", true, Config{"https://env.example", "flag-token", true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MANTISBT_URL", tc.envURL)
			t.Setenv("MANTISBT_TOKEN", tc.envToken)
			got, err := Load(tc.flagURL, tc.flagToken, tc.json)
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Load()=%+v, err=%v; want %+v", got, err, tc.want)
			}
		})
	}
}

func TestLoadReportsMissingSettings(t *testing.T) {
	cases := []struct {
		name, url, token string
		missing          []string
	}{
		{"both", "", "", []string{"MANTISBT_URL", "MANTISBT_TOKEN"}},
		{"URL", "", "private-test-token", []string{"MANTISBT_URL"}},
		{"token", "https://mantis.example", "", []string{"MANTISBT_TOKEN"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MANTISBT_URL", tc.url)
			t.Setenv("MANTISBT_TOKEN", tc.token)
			got, err := Load("", "", true)
			if err == nil || got != (Config{}) {
				t.Fatalf("expected no usable configuration, got %+v, err=%v", got, err)
			}
			for _, name := range tc.missing {
				if !strings.Contains(err.Error(), name) {
					t.Errorf("missing setting %s not reported: %v", name, err)
				}
			}
			if tc.token != "" && strings.Contains(err.Error(), tc.token) {
				t.Fatal("configuration error exposed the token")
			}
		})
	}
}
