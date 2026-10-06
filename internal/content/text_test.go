package content

import "testing"

func TestCleanText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"plain", "Hello", 0, "Hello"},
		{"punctuation kept", "‘猎杀对决’ 社区", 0, "‘猎杀对决’ 社区"},
		{"bidi stripped", "a\u202eb\u2066c\u2069", 0, "abc"},
		{"zero width stripped", "x\u200by\u200dz\ufeff", 0, "xyz"},
		{"controls", "a\x00b\x07c", 0, "abc"},
		{"newlines to spaces", " a\nb\tc\r ", 0, "a b c"},
		{"truncate runes", "一二三四五", 3, "一二三"},
		{"truncate trims", "ab cd", 3, "ab"},
		{"invalid utf8", "a\xffb", 0, "a�b"},
		{"no limit", "abc", -1, "abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CleanText(tt.in, tt.max); got != tt.want {
				t.Errorf("CleanText(%q, %d) = %q, want %q", tt.in, tt.max, got, tt.want)
			}
		})
	}
}
