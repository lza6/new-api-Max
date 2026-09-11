package common

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaskKey(t *testing.T) {
	testCases := []struct {
		name string
		key  string
		want string
	}{
		{name: "empty", key: "", want: ""},
		{name: "normal sk key", key: "sk-abcdefgh12345678", want: "sk-a****5678"},
		{name: "anthropic key", key: "sk-ant-api03-abcdefgh12345678", want: "sk-a****5678"},
		{name: "google key", key: "AIzaSyAbCdEfGh1234567890", want: "AIza****7890"},
		{name: "short key keeps tail 4", key: "sk-1234", want: "****1234"},
		{name: "exactly 12", key: "abcdefghijkl", want: "abcd****ijkl"},
		{name: "exactly 11 keeps tail", key: "abcdefghijk", want: "****hijk"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, MaskKey(tc.key))
		})
	}
}

func TestMaskMessage(t *testing.T) {
	key := "sk-abcdefgh1234567890"
	testCases := []struct {
		name    string
		msg     string
		want    string
		notWant []string
	}{
		{
			name: "empty",
			msg:  "",
			want: "",
		},
		{
			name: "no key text passes through",
			msg:  "channel 3 request failed with 500",
			want: "channel 3 request failed with 500",
		},
		{
			name:    "single key masked",
			msg:     "request failed with key " + key + " retrying",
			want:    "request failed with key sk-a****7890 retrying",
			notWant: []string{key},
		},
		{
			name:    "key in quotes masked",
			msg:     `{"api_key":"` + key + `","model":"gpt-4"}`,
			want:    `{"api_key":"sk-a****7890","model":"gpt-4"}`,
			notWant: []string{key},
		},
		{
			name:    "multiple keys masked",
			msg:     "old " + key + " new sk-ant-api03-zzzzzzzzzz99999999 done",
			want:    "old sk-a****7890 new sk-a****9999 done",
			notWant: []string{key, "sk-ant-api03-zzzzzzzzzz99999999"},
		},
		{
			name: "short token below threshold untouched",
			msg:  "prefix sk-1234 is too short",
			want: "prefix sk-1234 is too short",
		},
		{
			name:    "google key masked",
			msg:     "gemini key AIzaSyAbCdEfGh1234567890 expired",
			want:    "gemini key AIza****7890 expired",
			notWant: []string{"AIzaSyAbCdEfGh1234567890"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := MaskMessage(tc.msg)
			assert.Equal(t, tc.want, got)
			for _, secret := range tc.notWant {
				assert.NotContains(t, got, secret)
			}
		})
	}
}

func TestMaskMessageNoRegexBacktrack(t *testing.T) {
	// 长文本里大量前缀样片段，线性扫描应快速完成且不崩溃。
	msg := strings.Repeat("sk- ", 10000) + "end"
	got := MaskMessage(msg)
	assert.True(t, strings.HasSuffix(got, "end"))
}
