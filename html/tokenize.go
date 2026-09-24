package html

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mgilbir/forme/internal/ascii"
)

// The tokenizer.
//
// HTML5's is an eighty-state machine, and most of those states exist to define
// what a malformed construct recovers to. This one refuses instead (see the
// package comment), so it is a fraction of the size — and every place it
// differs from a browser is a refusal with a message, never a quiet
// reinterpretation.

// maxErrors bounds how many problems are reported from one document, so a file
// of noise cannot turn into an unbounded diagnostic list. The last entry says
// the list was cut, so a truncated report never reads as a complete one.
const maxErrors = 100

// Error is a place the document was not what this engine reads.
type Error struct {
	// Offset is the byte offset in the input where the problem was noticed.
	Offset int
	// Message says what was wrong in terms of the source.
	Message string
	// Unsupported marks correct HTML that this engine does not implement, as
	// against input that is malformed. The two mean opposite things to an
	// author: malformed markup is theirs to fix, an unsupported element is a
	// limit of the renderer. See css.Error, which draws the same line.
	Unsupported bool
	// Limit marks a bound on the document that was reached — its size, its node
	// count, its nesting depth, or the length of this list — rather than
	// anything the document got wrong. It is the third thing an author can be
	// told and it is not either of the other two: the markup is correct, the
	// engine implements it, and some of the document was not read anyway. A
	// caller that reports this as malformed markup sends an author looking for
	// a mistake that is not there.
	Limit bool
}

func (e Error) Error() string { return fmt.Sprintf("byte %d: %s", e.Offset, e.Message) }

// Position converts a byte offset into a line and column, both counted from 1,
// with the column counted in code points — which is what an editor shows and so
// what an author can act on.
func Position(input string, offset int) (line, col int) {
	if offset > len(input) {
		offset = len(input)
	}
	line, col = 1, 1
	for i := 0; i < len(input); {
		_, size := utf8.DecodeRuneInString(input[i:])
		if i+size > offset {
			return line, col
		}
		if input[i] == '\n' {
			line, col = line+1, 1
		} else {
			col++
		}
		i += size
	}
	return line, col
}

type tokenKind uint8

const (
	tokEOF tokenKind = iota
	tokText
	tokStartTag
	tokEndTag
	tokDoctype
	// tokComment is a comment, and carries nothing: it is dropped, and the
	// reason it is a token at all is that HTML's rules are stated over the token
	// stream. "If the next token is a line feed, ignore it" is the <pre> rule,
	// and a comment written between the start tag and the newline makes the
	// newline not the next token — so a tokenizer that consumed the comment in
	// silence dropped a newline the author meant to write.
	tokComment
)

type token struct {
	kind        tokenKind
	name        string // lowercased, for tags
	attrs       []Attribute
	text        string
	selfClosing bool
	offset      int
}

type tokenizer struct {
	src  string
	pos  int
	errs []Error
	// raw is the element whose content is being read verbatim, empty when not
	// in one. It is set by the parser, because whether "<" starts a tag depends
	// on which element is open — the one place the two layers are not separable.
	raw    string
	rcdata bool
	// xml says the document is XHTML rather than HTML, which changes exactly one
	// thing here: <style> and <script> hold ordinary character data, so "&gt;"
	// in a stylesheet is a ">". See looksLikeXML.
	xml bool
	// foreign says the markup being read is inside an <svg> or a <math>, which
	// the parser sets while it skips one. It changes one thing: a CDATA section
	// is a CDATA section there in HTML as well as in XML.
	foreign bool
}

// commentLength is how many bytes a comment occupies, from its "<!--" to the
// end of whatever closed it.
//
// Three ways to close one, and only the first was read. The other two are
// errors in the markup and the standard names them, gives each a parse error,
// and says where the comment ends anyway — because it has to: a document does
// not stop being a document because somebody wrote a comment badly.
//
//   - "-->", the ordinary one.
//   - "--!>", which the standard calls an incorrectly-closed comment. It is
//     what an editor produces from "<!-- x --!>" and a person reads as closed.
//   - ">" straight after the "<!--" or after "<!---", which the standard calls
//     an abrupt closing of an empty comment. "<!-->" is a comment of nothing.
//
// Reading only "-->" meant the other two ran to the end of the document and
// took every element after them with it, reported as one comment that was never
// closed. A single "<!-->" in a template emptied the page.
func commentLength(src string) (int, bool) {
	body := src[len("<!--"):]
	// The abrupt close, which is only abrupt at the very start.
	if strings.HasPrefix(body, ">") {
		return len("<!--") + 1, true
	}
	if strings.HasPrefix(body, "->") {
		return len("<!--") + 2, true
	}
	// One pass, stopping at whichever terminator comes first. Searching for
	// each separately and taking the earlier is the obvious way and is
	// quadratic: the one that is not there scans to the end of the document
	// every time, so a page of six hundred thousand comments spent a hundred
	// seconds looking for a "--!>" that no comment had.
	for at := 0; ; at++ {
		i := strings.Index(body[at:], "--")
		if i < 0 {
			return 0, false
		}
		at += i
		switch rest := body[at+2:]; {
		case strings.HasPrefix(rest, ">"):
			return len("<!--") + at + len("-->"), true
		case strings.HasPrefix(rest, "!>"):
			return len("<!--") + at + len("--!>"), true
		}
	}
}

// bom is the byte order mark, U+FEFF, whose encoded form is the three bytes
// EF BB BF that a Windows editor writes at the front of a UTF-8 file.
const bom = "\ufeff"

// newTokenizer reads src as XHTML when the caller said it is one, and otherwise
// as whatever the document says it is. See ParseXHTML and looksLikeXML.
func newTokenizer(src string, xhtml bool) *tokenizer {
	t := &tokenizer{src: src, xml: xhtml || looksLikeXML(src)}
	if strings.HasPrefix(src, bom) {
		// A leading byte order mark is not content. HTML's encoding sniffing
		// takes those three bytes as the statement "this file is UTF-8" and
		// removes them, and its input preprocessing says the same thing again
		// for a document that arrived decoded: "one leading U+FEFF byte order
		// mark character must be ignored".
		//
		// Ignored where the reading starts rather than by cutting the string,
		// so that every offset in a finding is still an offset into the bytes
		// the author has in front of them.
		//
		// Only the first one, and only at the very front. Anywhere else U+FEFF
		// is ZERO WIDTH NO-BREAK SPACE, a character of the text that sets no
		// paper and holds two words together, and dropping one of those would
		// be dropping content.
		//
		// A mark that survives is not a subtle failure: it is a character in an
		// otherwise empty inline formatting context before the document's first
		// element, so the whole page moves down by a line.
		t.pos = len(bom)
	}
	// Before a byte is read as markup: whether the bytes are text at all, and
	// whether the document says they are meant to be something this engine
	// cannot read. See encoding.go.
	t.checkEncoding()
	return t
}

// looksLikeXML reports whether a document says it is XHTML.
//
// It is asked because HTML and XHTML disagree about what is inside a <style>:
// HTML makes it raw text, where "&" and "<" are literal characters, and XML
// makes it ordinary character data, where "&gt;" is a ">". A stylesheet written
// for XHTML therefore spells a child combinator "&gt;", and read as HTML that
// selector matches nothing at all — so every rule in the block is silently
// inert and the page comes back unstyled in a way that looks like a layout bug.
//
// The two signals are the ones a document can state about its own syntax, and
// either is enough:
//
//   - an XML declaration, which only an XML document may begin with;
//   - a doctype naming XHTML, in its public or its system identifier.
//
// A browser does not ask any of this — it is told by the MIME type, and a caller
// that knows it says so with ParseXHTML, which does not ask either. These are
// what is left when nobody says: they are what the file itself asserts, and a
// document carrying neither is read as HTML, which is the safe direction: HTML
// is what an unmarked document overwhelmingly is.
//
// The XHTML namespace on the root element is not a signal, and was one. HTML
// allows the attribute on <html> and gives it no meaning at all — it is there so
// that one document can be served as either — and Pandoc's HTML5 template and a
// great deal of HTML5 boilerplate write it under "<!DOCTYPE html>". A browser
// opening such a file reads it as HTML. Read as XHTML here, its <pre> kept the
// newline after the tag, "&#146;" became a control character instead of a
// curly apostrophe, and "<br></br>" lost a break, all without a finding.
func looksLikeXML(src string) bool {
	// Only the prologue is examined. The signals all belong to it, and scanning
	// a whole document for a string that may appear in its text would make the
	// answer depend on the content.
	head := src
	if len(head) > 2048 {
		head = head[:2048]
	}
	if strings.HasPrefix(strings.TrimLeft(head, " \t\r\n\uFEFF"), "<?xml") {
		return true
	}
	if i := ascii.IndexFold(head, "<!doctype"); i >= 0 {
		if end := strings.IndexByte(head[i:], '>'); end >= 0 {
			// "xhtml" anywhere in it: the public identifiers are spelled
			// "-//W3C//DTD XHTML 1.0 Strict//EN" and, in documents that got it
			// slightly wrong, "-//W3C//DTD//XHTML 1.0"; the system identifiers
			// all name an xhtml DTD. "<!DOCTYPE html>" names nothing.
			if ascii.IndexFold(head[i:i+end], "xhtml") >= 0 {
				return true
			}
		}
	}
	return false
}

func (t *tokenizer) fail(off int, msg string) { t.add(Error{Offset: off, Message: msg}) }
func (t *tokenizer) unsupported(off int, msg string) {
	t.add(Error{Offset: off, Message: msg, Unsupported: true})
}

// limit reports a bound on the document that was reached. See Error.Limit.
func (t *tokenizer) limit(off int, msg string) {
	t.add(Error{Offset: off, Message: msg, Limit: true})
}

func (t *tokenizer) add(e Error) {
	switch {
	case len(t.errs) > maxErrors:
		return
	case len(t.errs) == maxErrors:
		t.errs = append(t.errs, Error{
			Offset:  e.Offset,
			Message: "further problems in this document were not reported",
			Limit:   true,
		})
	default:
		t.errs = append(t.errs, e)
	}
}

// stopped reports that a bound stopped the document being read, and it is the
// one finding the cap above does not apply to.
//
// "Further problems were not reported" says the list is short. It does not say
// the *tree* is, and a document that reported a hundred problems before it ran
// into maxNodes was handed back as a short tree whose only word about it was the
// first — so a caller had no way to tell that the rest of the document had not
// been read. The parser stops reading when it calls this, so it is called once
// per document at most, and the list is still bounded.
func (t *tokenizer) stopped(off int, msg string) {
	t.errs = append(t.errs, Error{Offset: off, Message: msg, Limit: true})
}

// next produces one token.
//
// Half of what a document is made of produces no token at all — a comment, a
// processing instruction, a declaration, a stray "<" — and the reader has to
// go round again for each one. It goes round in a loop rather than by calling
// itself, because a document is allowed to be 64 MB of nothing but those: at
// one frame per skipped construct, fourteen megabytes of comments is a
// "fatal error: stack overflow", which no recover catches and which takes the
// whole process down with every other document in flight. A loop is the same
// work in a fixed frame.
func (t *tokenizer) next() token {
	for {
		before := t.pos
		if tok, ok := t.step(); ok {
			return tok
		}
		if t.pos == before {
			// Every branch that produces no token says so by consuming the
			// construct it skipped; one that consumed nothing would spin here
			// for ever. No input can reach this — it is a branch added later
			// that forgot to advance — so it stops rather than hangs.
			panic("html: the tokenizer skipped a construct without consuming it")
		}
	}
}

// step produces at most one token and reports whether it produced one. A false
// second result means a construct was consumed that has no token to show for
// it; next calls step again.
func (t *tokenizer) step() (token, bool) {
	if t.raw != "" {
		return t.rawText(), true
	}
	if t.pos >= len(t.src) {
		return token{kind: tokEOF, offset: len(t.src)}, true
	}

	if t.src[t.pos] == '<' {
		return t.markup()
	}
	return t.text(), true
}

// text reads character data up to the next "<".
func (t *tokenizer) text() token {
	start := t.pos
	for t.pos < len(t.src) && t.src[t.pos] != '<' {
		t.pos++
	}
	return token{kind: tokText, text: t.nuls(t.decodeRefs(t.src[start:t.pos], start, false),
		start, "text", nulDropped), offset: start}
}

// What becomes of a NUL byte, which is the one choice nuls makes.
const (
	// nulDropped is ordinary text's: the tokenizer emits the NUL and the tree
	// builder, in every mode that puts text in a document, ignores it.
	nulDropped = ""
	// nulReplaced is every other state's: raw text and RCDATA, an attribute's
	// name and value, a CDATA section. The tokenizer itself turns the NUL into
	// U+FFFD REPLACEMENT CHARACTER there.
	nulReplaced = "\uFFFD"
)

// nuls takes the NUL bytes out of a run of text and says so once. It is the one
// place a NUL is dealt with, and everything the tokenizer turns into text or an
// attribute goes through it.
//
// U+0000 is not a character a document can contain: the standard's tokenizer
// makes one a parse error in every state that can meet it. It was kept, so a
// text node held a byte that is not text — into the shaper, into a PDF, into
// whatever a caller does with Node.Text — and the parse reported success. The
// first fix reached ordinary text and nothing else, so a <textarea>, a <title>,
// a stylesheet and every attribute value still carried one through.
//
// What it becomes is the standard's answer for the state, which is not one
// answer: dropped from ordinary text, U+FFFD everywhere else. The difference
// is visible — a NUL in a <textarea> is a replacement character on the page in
// every browser — and so it is the standard's that is followed rather than one
// of the two applied to both.
//
// One finding for the run and not one per byte, and the offset is the run's:
// a file with NULs in it usually has a great many, and they are one fault
// (something wrote UTF-16, or a binary file was handed over as HTML) rather
// than a hundred.
func (t *tokenizer) nuls(s string, off int, what, into string) string {
	if !strings.ContainsRune(s, 0) {
		return s
	}
	n := strings.Count(s, "\x00")
	word := "byte"
	if n > 1 {
		word = "bytes"
	}
	fate := "they are dropped"
	if into == nulReplaced {
		fate = "each is read as U+FFFD REPLACEMENT CHARACTER"
	}
	t.fail(off, what+" holding "+strconv.Itoa(n)+" NUL "+word+", which are not "+
		"characters; "+fate)
	return strings.ReplaceAll(s, "\x00", into)
}

// rawText reads the content of a raw-text or RCDATA element, up to its end tag.
func (t *tokenizer) rawText() token {
	start := t.pos
	name := t.raw
	end := -1
	if name != "plaintext" || t.xml {
		end = t.findEndTag(name, t.pos)
	}
	if end < 0 {
		// <plaintext> has no end tag in HTML: the tokenizer switches to its
		// state and never leaves it, so "</plaintext>" is text and the rest of
		// the document is the element's. Its being open at the end is reported
		// once, by the tree builder, as every element open there is.
		if name != "plaintext" || t.xml {
			t.fail(start, "<"+name+"> is never closed")
		}
		t.pos = len(t.src)
		end = len(t.src)
	} else {
		t.pos = end
	}
	t.raw = ""
	return token{kind: tokText, text: t.nuls(t.rawValue(t.src[start:end], start), start,
		"the text of <"+name+">", nulReplaced), offset: start}
}

// rawValue resolves references in RCDATA and leaves raw text alone.
//
// In an XHTML document raw text is not raw: <style> and <script> hold ordinary
// character data there, so their references are resolved too. The one exception
// is a CDATA section, which is the XML syntax for "the characters between these
// markers are literal" and is exactly what an author writes round a stylesheet
// to keep "&" and "<" meaning themselves.
func (t *tokenizer) rawValue(s string, off int) string {
	if t.rcdata {
		return t.decodeRefs(s, off, false)
	}
	if !t.xml {
		return s
	}
	var b strings.Builder
	for rest, at := s, off; ; {
		i := strings.Index(rest, cdataOpen)
		if i < 0 {
			b.WriteString(t.decodeRefs(rest, at, false))
			break
		}
		b.WriteString(t.decodeRefs(rest[:i], at, false))
		body := rest[i+len(cdataOpen):]
		j := strings.Index(body, cdataClose)
		if j < 0 {
			// Unterminated: the rest of the element is literal, which is what
			// the markers asked for and is the reading that loses nothing.
			b.WriteString(body)
			break
		}
		b.WriteString(body[:j])
		rest = body[j+len(cdataClose):]
		at += i + len(cdataOpen) + j + len(cdataClose)
	}
	return b.String()
}

const (
	cdataOpen  = "<![CDATA["
	cdataClose = "]]>"
)

// The case-insensitive searches here are internal/ascii's, and they are not
// spelled "strings.HasPrefix(strings.ToLower(src[pos:]), ...)" for a reason
// beyond the folding. That spelling lowercases everything from the cursor to
// the end of the file, and it sat in the path taken at *every* "<". A document
// of forty thousand tags therefore lowercased its own length forty thousand
// times: forty gigabytes of work for a megabyte of HTML, which measured at
// sixty-six seconds while the same document's layout took a third of one.
// Anything past a few hundred kilobytes of small elements was effectively a
// hang, reachable by anyone who could hand this engine a file.
//
// Only ASCII is folded, which is what the HTML syntax needs: tag and doctype
// names are ASCII, and folding beyond it would make "İ" a match for "i".

// findEndTag locates "</name" followed by a tag terminator, from position i.
func (t *tokenizer) findEndTag(name string, i int) int {
	want := "</" + name
	for {
		j := ascii.IndexFold(t.src[i:], want)
		if j < 0 {
			return -1
		}
		at := i + j
		after := at + 2 + len(name)
		if after >= len(t.src) || t.src[after] == '>' || isSpace(t.src[after]) || t.src[after] == '/' {
			return at
		}
		i = at + 1
	}
}

// markup reads whatever begins with "<". Like step, whose contract it shares,
// a false second result means the construct was consumed and produced nothing.
func (t *tokenizer) markup() (token, bool) {
	start := t.pos

	// A comment. Its content is dropped — nothing downstream has any use for one
	// — but it is a token, because the rules that count tokens have to count it.
	// See tokComment.
	if strings.HasPrefix(t.src[t.pos:], "<!--") {
		if n, ok := commentLength(t.src[t.pos:]); ok {
			t.pos += n
			return token{kind: tokComment, offset: start}, true
		}
		t.fail(start, "a comment that is never closed")
		t.pos = len(t.src)
		return token{}, false
	}

	if ascii.HasPrefixFold(t.src[t.pos:], "<!doctype") {
		return t.doctype(), true
	}

	// A CDATA section, which is XML's way of saying "the characters between
	// these markers are literal". In an XHTML document it is text and is read
	// as text; skipping it to the first ">" — which is what a declaration gets
	// — dropped its content and, where the content held a ">", took the rest of
	// the document with it.
	//
	// HTML has the syntax too, in one place: inside SVG and MathML, which are
	// XML vocabularies and keep XML's CDATA sections — the tokenizer's
	// "adjusted current node is not an element in the HTML namespace" clause.
	// Illustrator and Inkscape write an SVG's <style> inside one. Anywhere else
	// in an HTML document "<![CDATA[" is a bogus comment ending at the first
	// ">", which is what the branch below does.
	if (t.xml || t.foreign) && strings.HasPrefix(t.src[t.pos:], cdataOpen) {
		body := t.src[t.pos+len(cdataOpen):]
		end := strings.Index(body, cdataClose)
		if end < 0 {
			t.fail(start, "a CDATA section that is never closed")
			t.pos = len(t.src)
			end = len(body)
		} else {
			t.pos += len(cdataOpen) + end + len(cdataClose)
		}
		return token{kind: tokText, text: t.nuls(body[:end], start+len(cdataOpen),
			"a CDATA section", nulReplaced), offset: start}, true
	}

	if strings.HasPrefix(t.src[t.pos:], "<![") || strings.HasPrefix(t.src[t.pos:], "<!") {
		t.fail(start, "a declaration this engine does not read")
		t.skipTo('>')
		return token{}, false
	}

	if strings.HasPrefix(t.src[t.pos:], "<?") {
		// A processing instruction. HTML has none — a browser reads "<?x?>" as a
		// bogus comment — but XML has them, and the first thing in half the
		// suite's XHTML is "<?xml version='1.0'?>". Reporting the document's own
		// declaration as malformed markup is a finding an author cannot act on,
		// and it taints every rendering of every document that carries one.
		//
		// Nothing is produced either way: a processing instruction is an
		// instruction to the application, and this application has none to
		// follow.
		if !t.xml {
			t.fail(start, "a processing instruction, which HTML has none of")
		}
		t.skipTo('>')
		return token{}, false
	}

	if strings.HasPrefix(t.src[t.pos:], "</") {
		return t.endTag()
	}

	// "<" that cannot begin a tag. HTML's tag open state emits it as character
	// data and reconsumes the byte after it, and that is what this does.
	//
	// It used to drop the character and report the document instead, on the
	// reasoning that an unescaped "<" in a template is a mistake and the
	// difference is invisible until it swallows a line. The report is right and
	// is kept. Dropping it was not: nothing is swallowed by emitting the
	// character — that is what makes this different from reading it as a tag —
	// and what the old rule did was delete a character the author wrote from
	// the page and from the text extracted out of it. "a<0b" rendered as
	// "a0b" here and as "a<0b" in every browser.
	if t.pos+1 >= len(t.src) || !isNameStart(t.src[t.pos+1]) {
		t.fail(start, "a \"<\" that does not begin a tag; write \"&lt;\" for a literal one")
		t.pos++
		return token{kind: tokText, text: "<", offset: start}, true
	}
	return t.startTag(), true
}

func (t *tokenizer) skipTo(c byte) {
	for t.pos < len(t.src) && t.src[t.pos] != c {
		t.pos++
	}
	if t.pos < len(t.src) {
		t.pos++
	}
}

func (t *tokenizer) doctype() token {
	start := t.pos
	t.skipTo('>')
	return token{kind: tokDoctype, offset: start}
}

func (t *tokenizer) endTag() (token, bool) {
	start := t.pos
	t.pos += 2
	// HTML's end tag open state: a name begins only with an ASCII letter, as a
	// start tag's does. Anything else — "</1x>", "</ſpan>", "</>" — is not an
	// end tag, and the standard reads it as a bogus comment to the next ">", or
	// as nothing at all for "</>". It used to be read as a tag whenever the
	// characters after "</" could continue a name, so "</1x>" was an end tag
	// "1x" that closed nothing.
	if t.pos >= len(t.src) || !isNameStart(t.src[t.pos]) {
		t.fail(start, "an end tag with no name")
		t.skipTo('>')
		return token{}, false
	}
	name := t.readName()
	t.skipSpace()
	if t.pos < len(t.src) && t.src[t.pos] == '>' {
		t.pos++
	} else {
		t.fail(start, "the end tag </"+name+" is not closed with \">\"")
		t.skipTo('>')
	}
	return token{kind: tokEndTag, name: name, offset: start}, true
}

func (t *tokenizer) startTag() token {
	start := t.pos
	t.pos++ // "<"
	name := t.readName()

	out := token{kind: tokStartTag, name: name, offset: start}
	seen := map[string]bool{}
	// cut records that maxAttributes was reached on this tag.
	cut := false

	for {
		t.skipSpace()
		if t.pos >= len(t.src) {
			t.fail(start, "the tag <"+name+" is never closed")
			return out
		}
		switch t.src[t.pos] {
		case '>':
			t.pos++
			return out
		case '/':
			if t.pos+1 < len(t.src) && t.src[t.pos+1] == '>' {
				t.pos += 2
				out.selfClosing = true
				return out
			}
			t.fail(t.pos, "a \"/\" in the middle of the tag <"+name+">")
			t.pos++
			continue
		}

		at := t.pos
		attr := t.attribute(name)
		if t.pos == at {
			// attribute always reads at least the byte it starts at, which is
			// what ends this loop; one that did not would spin here for ever.
			// No input reaches this, as with next's guard: it stops rather than
			// hangs.
			panic("html: an attribute was read without consuming anything")
		}
		if len(out.attrs) == maxAttributes {
			// Read and not kept, so the tag still ends where it ends, and not
			// checked for a repeat either: remembering the names of attributes
			// that are thrown away would be the unbounded list over again, as a
			// set. Reported once, at the first one dropped, which is where the
			// author has to look.
			if !cut {
				cut = true
				t.limit(at, "<"+name+"> has more attributes than this engine will read ("+
					strconv.Itoa(maxAttributes)+"); \""+attr.Name+"\" and those after it were dropped")
			}
			continue
		}
		if seen[attr.Name] {
			// A repeated attribute is refused rather than resolved. HTML keeps
			// the first and drops the rest, which means a template with two
			// class attributes silently loses one — and which one is lost is
			// not something an author can see in the output.
			t.fail(at, "the attribute \""+attr.Name+"\" appears twice on <"+name+">")
			continue
		}
		seen[attr.Name] = true
		out.attrs = append(out.attrs, attr)
	}
}

// attribute reads one attribute. It always consumes at least one byte and never
// the ">" that ends the tag, which is what lets startTag loop over it without a
// case for either.
//
// Where the markup is malformed the answer is the standard's, and it is
// reported: the characters HTML reads as part of a name or an unquoted value
// are kept in it. The alternative that was here — refuse the character and skip
// to the next ">" — consumed the tag's own ">", and the attribute loop went on
// reading attributes out of the text after the tag: "<a href=x\"y>hello</a>"
// lost the word "hello" into an attribute of <a> and reported a cascade of
// mistakes the author never made.
func (t *tokenizer) attribute(tag string) Attribute {
	at := t.pos
	name := t.nuls(t.readAttrName(tag), at, "an attribute name", nulReplaced)
	t.skipSpace()
	if t.pos >= len(t.src) || t.src[t.pos] != '=' {
		// A boolean attribute: present, with the empty string for a value.
		return Attribute{Name: name}
	}
	t.pos++ // "="
	t.skipSpace()
	if t.pos >= len(t.src) || t.src[t.pos] == '>' {
		// The standard's missing-attribute-value: an attribute with the empty
		// string for its value, and the tag ends where it ends.
		t.fail(at, "the attribute \""+name+"\" has no value")
		return Attribute{Name: name}
	}

	switch q := t.src[t.pos]; q {
	case '"', '\'':
		t.pos++
		start := t.pos
		for t.pos < len(t.src) && t.src[t.pos] != q {
			t.pos++
		}
		if t.pos >= len(t.src) {
			t.fail(at, "the value of \""+name+"\" is never closed")
			return Attribute{Name: name, Value: t.attrValue(t.src[start:], start)}
		}
		v := t.attrValue(t.src[start:t.pos], start)
		t.pos++
		return Attribute{Name: name, Value: v}
	}

	// Unquoted. HTML allows it, and ends it at white space or ">". The
	// characters it forbids inside one are the ones that make the end
	// ambiguous; each is a parse error, and each is kept in the value, which is
	// what every browser shows. Reported once for the value.
	start := t.pos
	reported := false
	for t.pos < len(t.src) && !isSpace(t.src[t.pos]) && t.src[t.pos] != '>' {
		switch c := t.src[t.pos]; c {
		case '"', '\'', '<', '=', '`':
			if !reported {
				reported = true
				t.fail(t.pos, fmt.Sprintf("%q in the unquoted value of \"%s\" is part of the "+
					"value, which is how HTML reads it; quote the value", c, name))
			}
		}
		t.pos++
	}
	return Attribute{Name: name, Value: t.attrValue(t.src[start:t.pos], start)}
}

// attrValue is an attribute value's text: its references resolved and its NULs
// replaced.
func (t *tokenizer) attrValue(s string, off int) string {
	return t.nuls(t.decodeRefs(s, off, true), off, "an attribute value", nulReplaced)
}

func (t *tokenizer) readName() string {
	start := t.pos
	// The name runs to white space, "/" or ">", and to nothing else. That is
	// HTML's tag name state (§13.2.5.8), which has exactly those three
	// terminators and appends every other character it meets: an ASCII capital
	// lowercased, a NUL as U+FFFD with a parse error, and anything else — a
	// digit, a ".", a quote, a letter outside ASCII — as it stands.
	//
	// It stopped at the first byte that was not an ASCII letter, digit, "-",
	// "_" or ":", and the rest of the name became an attribute. "<aſb>" was an
	// element "a" with an attribute "ſb", "<x.y>" an "x" with ".y", and the end
	// tag "</aſb>" was an end tag "a" followed by markup that did not close it.
	// A custom element's name may hold any of those — "<math-α>" and
	// "<emotion-😍>" are valid ones — and every browser reads them whole.
	//
	// A colon is part of a name, in HTML as well as in XML, which the rule
	// above now says without a special case: XML gives a name an optional
	// namespace prefix — the suite writes its inline SVG as "<svg:svg>" — and
	// HTML's tag name state has no reason to stop at one either. It was once
	// admitted in XML alone, which made "<o:p>" — the tag a Word document is
	// full of — an element "o" with an attribute ":p". What the prefix *means*
	// is the parser's question, not this one's: see parser.resolveName.
	//
	// The folding is ASCII's and only ASCII's (see internal/ascii). strings.ToLower
	// is Unicode's, and it would make a KELVIN SIGN the letter k: "<X\u212ABD>"
	// would open an element "xkbd" that nobody wrote.
	for t.pos < len(t.src) && !isSpace(t.src[t.pos]) && t.src[t.pos] != '/' && t.src[t.pos] != '>' {
		t.pos++
	}
	return t.nuls(ascii.Lower(t.src[start:t.pos]), start, "a tag name", nulReplaced)
}

// readAttrName reads an attribute name, which admits more characters than an
// element name does.
//
// It is called at a byte that is not white space, "/" or ">", and it reads at
// least that byte, which is the standard's rule: an "=" there begins the name
// rather than a value. A quote or a "<" inside a name is a parse error and part
// of the name, and is reported as the mistake it almost always is — a missing
// space or a missing quote.
func (t *tokenizer) readAttrName(tag string) string {
	start := t.pos
	reported := false
	for t.pos < len(t.src) {
		c := t.src[t.pos]
		if isSpace(c) || c == '>' || c == '/' || c == '=' && t.pos > start {
			break
		}
		if (c == '"' || c == '\'' || c == '<' || c == '=') && !reported {
			reported = true
			t.fail(t.pos, fmt.Sprintf("%q in an attribute name of <%s> is part of the name, "+
				"which is how HTML reads it; a space or a quote is missing", c, tag))
		}
		t.pos++
	}
	// ASCII's folding, as a tag name's: see readName.
	return ascii.Lower(t.src[start:t.pos])
}

func (t *tokenizer) skipSpace() {
	for t.pos < len(t.src) && isSpace(t.src[t.pos]) {
		t.pos++
	}
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

func isNameStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// decodeRefs resolves character references in a run of text.
//
// There is no expansion budget here, and the absence is deliberate rather than
// an oversight: HTML character references do not nest. "&amp;amp;" is the text
// "&amp;", not an ampersand, because resolution is a single pass over the
// source. The billion-laughs attack needs XML's recursive entity definitions,
// which HTML has none of, so the worst case is the 32:1 of the longest name.
//
// inAttr changes only the diagnostics, since the resolution itself is the same.
func (t *tokenizer) decodeRefs(s string, off int, inAttr bool) string {
	if !strings.ContainsRune(s, '&') && !strings.ContainsRune(s, '\r') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))

	for i := 0; i < len(s); {
		if s[i] == '\r' {
			// HTML's input preprocessing: "any LF character that immediately
			// follows a CR character must be ignored, and all CR characters
			// must then be converted to LF characters".
			//
			// Here rather than over the whole source, because it is a rule
			// about the *input stream* — the bytes the author wrote — and a
			// character reference is resolved afterwards. "&#x0D;" therefore
			// puts a real U+000D in the tree, which is what the suite's
			// control-chars-00D is made of, and CSS Text makes that one white
			// space rather than a segment break.
			//
			// Doing it over the whole source instead would be the same rule and
			// the wrong offsets: every finding after a CRLF would point one
			// byte earlier in the file than the markup it names.
			b.WriteByte('\n')
			i++
			if i < len(s) && s[i] == '\n' {
				i++
			}
			continue
		}
		if s[i] != '&' {
			b.WriteByte(s[i])
			i++
			continue
		}
		text, n, ok := t.reference(s[i:], off+i, inAttr)
		if !ok {
			b.WriteByte('&')
			i++
			continue
		}
		b.WriteString(text)
		i += n
	}
	return b.String()
}

// reference resolves one character reference at the start of s, returning the
// text it stands for and how many bytes it spanned.
func (t *tokenizer) reference(s string, off int, inAttr bool) (string, int, bool) {
	if len(s) < 2 {
		return "", 0, false
	}

	if s[1] == '#' {
		return t.numericReference(s, off)
	}

	// A named reference. The semicolon is required: without it the standard
	// matches one of 106 historical names, so "&notit;" is "¬it;" in a browser,
	// and a reader that guesses differently is a reader that silently changes
	// the text.
	end := -1
	limit := min(len(s), maxEntityNameLen+2)
	for j := 1; j < limit; j++ {
		if s[j] == ';' {
			end = j
			break
		}
		if !isEntityNamePart(s[j]) {
			break
		}
	}
	if end > 1 {
		name := s[1 : end+1] // including the ";", which is how the table is keyed
		if text, found := namedEntities[name]; found {
			return text, end + 1, true
		}
		t.fail(off, "\"&"+name+"\" is not a character reference; write \"&amp;\" for a literal ampersand")
		return "", 0, false
	}

	// No semicolon. This is a literal ampersand here and in a browser — unless
	// a historical name matches, which is the one case where the two differ, and
	// so the one case worth reporting.
	if legacy, n := longestLegacyName(s); legacy != "" {
		if inAttr {
			// HTML's named character reference state has one clause about an
			// attribute, and it is this: a name with no ";" followed by "=" or
			// by an alphanumeric is not a reference at all, and is not a parse
			// error either. "?q=1&copy=2" is a query string, in this engine and
			// in every browser, which is exactly the case the clause exists
			// for — and it was reported as something "a character reference in
			// some browsers", which no browser makes it.
			if next := 1 + n; next < len(s) && (s[next] == '=' || isEntityNamePart(s[next])) {
				return "", 0, false
			}
			t.fail(off, "\"&"+legacy+"\" without a \";\" is a literal ampersand here "+
				"and a character reference in some browsers; write \"&amp;\" or \"&"+legacy+";\"")
		} else {
			t.fail(off, "\"&"+legacy+"\" is missing its \";\"; write \"&"+legacy+";\", "+
				"or \"&amp;\" for a literal ampersand")
		}
	}
	return "", 0, false
}

// longestLegacyName finds the longest semicolon-less name in the table that
// matches at the start of s, which is what a browser would resolve here.
func longestLegacyName(s string) (string, int) {
	limit := min(len(s)-1, maxEntityNameLen)
	for n := limit; n >= 2; n-- {
		name := s[1 : 1+n]
		if _, ok := namedEntities[name]; ok {
			return name, n
		}
	}
	return "", 0
}

func (t *tokenizer) numericReference(s string, off int) (string, int, bool) {
	i := 2
	base, digits := 10, "0123456789"
	if i < len(s) && (s[i] == 'x' || s[i] == 'X') {
		i++
		base, digits = 16, "0123456789abcdefABCDEF"
	}
	start := i
	for i < len(s) && strings.IndexByte(digits, s[i]) >= 0 {
		i++
	}
	if i == start {
		t.fail(off, "\"&#\" with no number after it; write \"&amp;\" for a literal ampersand")
		return "", 0, false
	}
	if i >= len(s) || s[i] != ';' {
		t.fail(off, "a numeric character reference with no \";\"")
		return "", 0, false
	}

	// Leading zeros are not magnitude. "&#000000065;" is the letter A written
	// with nine digits, and counting them against the length refused it — while
	// "&#x00000041;", the same character written with eight, was accepted, so
	// the rule was not even the same in the two bases.
	digitsAt := start
	for digitsAt < i-1 && s[digitsAt] == '0' {
		digitsAt++
	}
	// A number too long to be a code point is rejected before it is parsed, so
	// a run of a million digits costs nothing.
	if i-digitsAt > 8 {
		t.fail(off, "a numeric character reference far outside Unicode")
		return "", 0, false
	}
	v, err := strconv.ParseInt(s[digitsAt:i], base, 64)
	if err != nil {
		t.fail(off, "a numeric character reference that is not a number")
		return "", 0, false
	}
	r, ok := t.codePoint(v, off)
	if !ok {
		return "", 0, false
	}
	return string(r), i + 1, true
}

// codePoint turns a numeric reference's value into a rune, refusing the ones
// that are not characters.
//
// The surrogates and the out-of-range values are refused rather than replaced,
// because a document asking for one is a document built wrong — most often by
// something that encoded UTF-16 as if it were code points — and silently
// substituting U+FFFD would put a replacement character in the page with nothing
// to say where it came from.
func (t *tokenizer) codePoint(v int64, off int) (rune, bool) {
	switch {
	case v == 0:
		t.fail(off, "a character reference to U+0000, which is not text")
		return 0, false
	case v >= 0xD800 && v <= 0xDFFF:
		t.fail(off, fmt.Sprintf("a character reference to U+%04X, half of a surrogate pair "+
			"and not a character", v))
		return 0, false
	case v > 0x10FFFF:
		t.fail(off, fmt.Sprintf("a character reference to %d, which is outside Unicode", v))
		return 0, false
	}
	if r, remapped := windows1252Reference[v]; remapped && !t.xml {
		// A reference into the C1 control range means what windows-1252 puts
		// there, which is what HTML says and what every browser does with an
		// HTML document. A page written by a Windows editor spells a curly
		// apostrophe "&#146;" and a euro "&#128;", and reading those as control
		// characters puts a character nothing draws where a letter belongs.
		//
		// In XHTML it does not. XML 1.0 §4.1 says a character reference is the
		// code point it names and nothing else, so "&#x80;" there is U+0080 —
		// which is what the suite's own control-characters-002.xht is written
		// to test, one box per control character.
		//
		// Not reported either way. In HTML it is not a mistake an author made,
		// it is the convention their editor writes in, and the standard's own
		// answer is the character rather than a complaint.
		return r, true
	}
	return rune(v), true
}

// windows1252Reference is what a numeric character reference in the C1 range
// stands for, from the standard's own table.
//
// The range is where windows-1252 puts its punctuation and Unicode puts control
// characters, and a numeric reference written by a Windows editor means the
// former. Seven of the thirty-two are unassigned in windows-1252 and are left
// as they are, which is what the table's own gaps say.
var windows1252Reference = map[int64]rune{
	0x80: 0x20AC, 0x82: 0x201A, 0x83: 0x0192, 0x84: 0x201E, 0x85: 0x2026,
	0x86: 0x2020, 0x87: 0x2021, 0x88: 0x02C6, 0x89: 0x2030, 0x8A: 0x0160,
	0x8B: 0x2039, 0x8C: 0x0152, 0x8E: 0x017D, 0x91: 0x2018, 0x92: 0x2019,
	0x93: 0x201C, 0x94: 0x201D, 0x95: 0x2022, 0x96: 0x2013, 0x97: 0x2014,
	0x98: 0x02DC, 0x99: 0x2122, 0x9A: 0x0161, 0x9B: 0x203A, 0x9C: 0x0153,
	0x9E: 0x017E, 0x9F: 0x0178,
}

func isEntityNamePart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}
