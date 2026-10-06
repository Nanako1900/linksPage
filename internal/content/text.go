package content

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// isInvisible reports bidi controls and zero-width characters stripped
// from untrusted names (doc 5.1): U+200B–U+200F, U+202A–U+202E,
// U+2066–U+2069, plus U+FEFF.
func isInvisible(r rune) bool {
	switch {
	case r >= 0x200B && r <= 0x200F, r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069, r == 0xFEFF:
		return true
	}
	return false
}

// CleanText strips invisible/bidi characters and other control
// characters (newlines and tabs become spaces), trims surrounding space
// and truncates to maxRunes runes (maxRunes ≤ 0 means no limit). Invalid
// UTF-8 is replaced with U+FFFD. Ordinary punctuation is preserved.
func CleanText(s string, maxRunes int) string {
	s = strings.ToValidUTF8(s, "�")
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case isInvisible(r):
			continue
		case r == '\n' || r == '\r' || r == '\t':
			b.WriteRune(' ')
		case unicode.IsControl(r):
			continue
		default:
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	if maxRunes > 0 && utf8.RuneCountInString(out) > maxRunes {
		out = strings.TrimSpace(string([]rune(out)[:maxRunes]))
	}
	return out
}
