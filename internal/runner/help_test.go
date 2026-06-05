package runner

import (
	"strings"
	"testing"
)

func TestHelpTextIsDiscoverableForHumansAndAgents(t *testing.T) {
	global := HelpText("")
	for _, want := range []string{
		"Usage:",
		"api request <profile> <METHOD> <path>",
		"api profile",
		"api secret",
		"~/.api/profiles.json",
		"Response envelope",
		"Forbidden headers",
	} {
		if !strings.Contains(global, want) {
			t.Fatalf("global help missing %q:\n%s", want, global)
		}
	}

	for _, topic := range []string{"request", "profile", "secret"} {
		text := HelpText(topic)
		if !strings.Contains(text, "Usage:") {
			t.Fatalf("%s help missing Usage:\n%s", topic, text)
		}
		if strings.Contains(text, "SECRET") || strings.Contains(text, "token-value") {
			t.Fatalf("%s help appears to contain secret-shaped example:\n%s", topic, text)
		}
	}
}

func TestProfileHelpIncludesCreatePresets(t *testing.T) {
	text := HelpText("profile")
	for _, want := range []string{
		"api profile create <profile>",
		"--preset dolibarr",
		"mailerlite",
		"pipedrive",
		"pennylane",
		"crisp",
		"does not store credentials",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("profile help missing %q:\n%s", want, text)
		}
	}
}
