package service

import "testing"

func TestIdentifyClient(t *testing.T) {
	cases := map[string]string{
		"claude-code/1.0":          "claude-code",
		"Cursor/0.42 (darwin)":     "cursor",
		"openai-python/1.0":        "openai-python",
		"OpenAI/NodeJS/4.0":        "openai-node",
		"python-requests/2.31":     "python",
		"axios/1.6.0":              "node",
		"Go-http-client/2.0":       "go",
		"curl/8.4.0":               "curl",
		"CherryStudio/1.0":         "cherry-studio",
		"Mozilla/5.0 (compatible)": "browser",
		"SomethingWeird/1.0":       "other",
		"":                         "unknown",
	}
	for ua, want := range cases {
		if got := IdentifyClient(ua); got != want {
			t.Errorf("IdentifyClient(%q) = %q, want %q", ua, got, want)
		}
	}
}
