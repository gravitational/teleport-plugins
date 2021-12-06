package lib

import "strings"

// MarkdownEscape wraps some text `t` in triple backticks (escaping any backtick
// inside the message), limiting the length of the message to `n` runes (inside
// the single preformatted block). Backticks are escaped and thus count as two
// runes for the purpose of the truncation.
func MarkdownEscape(t string, n int) string {
	var b strings.Builder
	b.WriteString("```")
	for i, r := range t {
		if i >= n {
			b.WriteString("``` (truncated)")
			return b.String()
		}
		b.WriteRune(r)
		if r == '`' {
			// byte order mark, as a zero width no-break space; seems to result
			// in escaped backticks with no spurious characters in the message
			b.WriteRune('\ufeff')
			n -= 1
		}
	}
	b.WriteString("```")
	return b.String()
}
