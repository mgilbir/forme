package font

// Two leaf helpers the Type 1 parser needs.
//
// Both are frozen by specification — the white-space set is PostScript's, which
// ISO 32000-2 Table 1 restates for PDF, and hex-digit decoding is what it is.

// isWhitespace reports whether b is one of the six PostScript white-space
// characters (ISO 32000-2 Table 1 lists the same six for PDF), which is what
// separates the tokens of a Type 1 program.
func isWhitespace(b byte) bool {
	switch b {
	case 0, '\t', '\n', '\f', '\r', ' ':
		return true
	}
	return false
}

// decodeHexBytes decodes hex-string content, tolerating white space and other
// non-hex bytes and padding an odd digit count with a trailing zero, which is
// what a PDF hex string means. It is used to detect and decode a Type 1 font
// program whose eexec-encrypted portion arrived in hexadecimal.
func decodeHexBytes(b []byte) []byte {
	var digits []byte
	for _, c := range b {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
			digits = append(digits, c)
		}
	}
	if len(digits)%2 == 1 {
		digits = append(digits, '0')
	}
	out := make([]byte, len(digits)/2)
	hv := func(c byte) byte {
		switch {
		case c <= '9':
			return c - '0'
		case c >= 'a':
			return c - 'a' + 10
		default:
			return c - 'A' + 10
		}
	}
	for i := 0; i < len(out); i++ {
		out[i] = hv(digits[2*i])<<4 | hv(digits[2*i+1])
	}
	return out
}
