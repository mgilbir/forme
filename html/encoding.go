package html

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// What this engine reads, and what it does about a document that is not it.
//
// Parse takes a Go string, and a Go string is bytes: nothing in the type says
// they are UTF-8. HTML's own answer is a sniffing algorithm that reads a byte
// order mark, a Content-Type header, a <meta charset>, and failing all of those
// guesses from the bytes — and then *decodes* the document from whichever
// encoding it settled on. Decoding windows-1252, Shift-JIS, GB18030 and the
// thirty others the standard names is a table for each, and this engine has
// none of them.
//
// What it must not do is pretend. A document in Shift-JIS handed to a reader
// that assumes UTF-8 comes out as a page of replacement characters, and the
// failure that matters is not the mojibake — it is that nothing said so. Every
// byte was consumed, every element was found where it should be, and the parse
// reported success.
//
// So the two things that can be checked are checked, and both are reported:
//
//   - whether the bytes are UTF-8 at all, which is a fact about the input; and
//   - whether the document says it is something else, which is a fact about
//     what the author meant.
//
// The bytes are left alone. Replacing an invalid one with U+FFFD is what the
// standard's preprocessing does, and it would move every offset after it — so
// an author told "byte 4,117" would be sent to a byte four thousand one hundred
// and seventeen of a string they do not have. The reader already yields U+FFFD
// for one when it walks the text, which is the same page with the offsets kept.

// maxEncodingSniff is how far into a document <meta charset> is looked for.
//
// The standard's own sniffing gives up after 1024 bytes, and a declaration
// further in than that is one no browser would have honoured either. It also
// bounds the work: this walk is over the raw bytes and runs before the parse.
const maxEncodingSniff = 1024

// utf8Aliases are the labels that name UTF-8, from the Encoding Standard's own
// table. Anything else names an encoding this engine cannot decode.
var utf8Aliases = map[string]bool{
	"utf-8": true, "utf8": true, "unicode-1-1-utf-8": true,
	"unicode11utf8": true, "unicode20utf8": true, "x-unicode20utf8": true,
}

// checkEncoding reports what can be known about a document's encoding before it
// is read: bytes that are not UTF-8, and a declaration that says they are not
// meant to be.
func (t *tokenizer) checkEncoding() {
	t.checkDeclaredEncoding()
	t.checkUTF8()
}

// checkUTF8 reports the first byte that is not part of a UTF-8 sequence, and
// how many there are.
//
// One finding and not one per byte: a document in another encoding is wrong in
// every line of itself, and a reader given four thousand findings learns less
// than one given the first offset and the count. The count is the whole
// document's, because "how much of this is not text" is the question an author
// asks next.
func (t *tokenizer) checkUTF8() {
	src := t.src
	first, bad := -1, 0
	for i := 0; i < len(src); {
		r, size := utf8.DecodeRuneInString(src[i:])
		if r == utf8.RuneError && size <= 1 {
			if first < 0 {
				first = i
			}
			bad++
			i++
			continue
		}
		i += size
	}
	if first < 0 {
		return
	}
	msg := "this engine reads UTF-8, and byte " + strconv.Itoa(first) +
		" begins no UTF-8 character"
	if bad > 1 {
		msg += " (" + strconv.Itoa(bad) + " such bytes in the document)"
	}
	msg += "; every one of them is read as U+FFFD REPLACEMENT CHARACTER"
	t.add(Error{Offset: first, Message: msg})
}

// checkDeclaredEncoding reports a <meta> that names an encoding this engine
// cannot decode.
//
// Both spellings, because a document may use either and the older one is still
// what a great many pages carry: <meta charset=…> and <meta http-equiv=
// "content-type" content="text/html; charset=…">. A document that declares
// UTF-8 is saying what is already true and is passed over in silence.
//
// It is read off the raw bytes rather than off the parsed tree, because the
// tree is the thing that was built on the assumption this checks. A document in
// an encoding where "<" is not 0x3C has no tree to read a <meta> out of.
func (t *tokenizer) checkDeclaredEncoding() {
	head := t.src
	if len(head) > maxEncodingSniff {
		head = head[:maxEncodingSniff]
	}
	lower := strings.ToLower(head)
	for at := 0; ; {
		i := strings.Index(lower[at:], "<meta")
		if i < 0 {
			return
		}
		start := at + i
		end := strings.IndexByte(lower[start:], '>')
		if end < 0 {
			return
		}
		tag := lower[start : start+end]
		at = start + end
		label, ok := charsetOf(tag)
		if !ok || utf8Aliases[label] {
			continue
		}
		t.add(Error{
			Offset: start,
			Message: "the document declares the " + strconv.Quote(label) +
				" encoding; this engine reads UTF-8 and cannot decode any other, so " +
				"the text it read is not the text the document holds",
			Unsupported: true,
		})
		return
	}
}

// charsetOf reads the encoding label out of one lower-cased <meta> tag, in
// either of the two spellings.
func charsetOf(tag string) (string, bool) {
	i := strings.Index(tag, "charset")
	if i < 0 {
		return "", false
	}
	rest := strings.TrimSpace(tag[i+len("charset"):])
	if !strings.HasPrefix(rest, "=") {
		return "", false
	}
	rest = strings.TrimSpace(rest[1:])
	// The value may be quoted, in either quote, and in the http-equiv spelling
	// the quotes are around the whole of the content and not around the label.
	if len(rest) > 0 && (rest[0] == '"' || rest[0] == '\'') {
		rest = rest[1:]
	}
	label := strings.TrimFunc(rest, func(r rune) bool { return r == ' ' || r == '\t' })
	if j := strings.IndexAny(label, " \t\"';>/"); j >= 0 {
		label = label[:j]
	}
	if label == "" {
		return "", false
	}
	return label, true
}
