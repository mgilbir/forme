package ascii

// White space in syntax is ASCII too.
//
// The Infra standard's "ASCII whitespace" is tab, line feed, form feed,
// carriage return and space, and it is what HTML splits and trims on wherever
// it names white space: the tokens of a class list, a rel, a srcset or a sizes
// list, the digits of a width or a colspan, the label of a <meta charset>. CSS
// Syntax 3 §4.2 calls white space a newline, a tab or a space, where a newline
// is the line feed that §3.3's preprocessing makes of every carriage return,
// form feed and CR LF pair. No other code point is white space to either.
//
// Go's strings.TrimSpace and strings.Fields split and trim on unicode.IsSpace,
// which is Unicode's White_Space property from the toolchain's release: the
// no-break space U+00A0, the em space U+2003, the ideographic space U+3000,
// U+0085 and the rest. Each of those is an ordinary character to CSS and
// HTML. In CSS it is a name code point, so "solid\u2003" is one identifier
// that is not the keyword solid; in HTML class="a\u00a0b" is one class. Trimmed
// or split by Unicode's set, either is read as something the author did not
// write. cmd/whitespace_test.go holds the engine to these.
//
// There are two sets of functions below, one per specification, and today the
// two sets are the same five bytes. CSS's is written with carriage return and
// form feed in it because not every string a CSS reader is handed has been
// through the tokenizer: a presentational attribute, a value built by the
// engine, a test. A carriage return or form feed in such a string is what
// preprocessing would have made a line feed, so it is white space as surely as
// the line feed is. Keeping the names apart says at every call which grammar
// is being read, which is what a reader needs to check it, and lets either
// change without the other if either specification does.
//
// Text content is not syntax and does not come here. Which characters of a
// text node are collapsible, and which are a segment break, is CSS Text's
// question, and package paragraph answers it.

// IsSpace reports whether c is Infra's ASCII whitespace: tab, line feed, form
// feed, carriage return or space.
func IsSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\f' || c == '\r'
}

// TrimSpace is s without its leading and trailing ASCII whitespace.
func TrimSpace(s string) string { return trim(s, IsSpace) }

// Fields splits s around each run of ASCII whitespace, as HTML's "split a
// string on ASCII whitespace" does, and returns no empty strings.
func Fields(s string) []string { return fields(s, IsSpace) }

// IsCSSSpace reports whether c is CSS white space: a space, a tab, or a
// newline, which preprocessing makes of a line feed, a carriage return and a
// form feed.
func IsCSSSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

// TrimCSSSpace is s without its leading and trailing CSS white space.
func TrimCSSSpace(s string) string { return trim(s, IsCSSSpace) }

// CSSFields splits s around each run of CSS white space and returns no empty
// strings. It is the reading of a value's space-separated words, and it takes
// the words as they were written: a quoted string with white space inside is
// not kept whole, which is what a caller that may meet one must see to itself.
func CSSFields(s string) []string { return fields(s, IsCSSSpace) }

func trim(s string, space func(byte) bool) string {
	i, j := 0, len(s)
	for i < j && space(s[i]) {
		i++
	}
	for j > i && space(s[j-1]) {
		j--
	}
	return s[i:j]
}

// fields scans bytes rather than runes. It can: every white space byte is
// ASCII, and no byte of a multi-byte UTF-8 sequence is.
func fields(s string, space func(byte) bool) []string {
	n := 0
	for i := 0; i < len(s); {
		for i < len(s) && space(s[i]) {
			i++
		}
		if i < len(s) {
			n++
		}
		for i < len(s) && !space(s[i]) {
			i++
		}
	}
	if n == 0 {
		return nil
	}
	out := make([]string, 0, n)
	for i := 0; i < len(s); {
		for i < len(s) && space(s[i]) {
			i++
		}
		start := i
		for i < len(s) && !space(s[i]) {
			i++
		}
		if i > start {
			out = append(out, s[start:i])
		}
	}
	return out
}
