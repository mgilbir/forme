package css

import "strings"

// The parser of CSS Syntax Level 3 §5: the layer that turns a flat token stream
// into rules, declarations and the nested component values they are built from.
//
// Like the tokenizer, this follows the specification's algorithms step for step,
// and for the same reason: every place a reader disagrees with a browser about
// where a rule ends is a rendering bug that testing the output cannot localise.
//
// # What this layer does and does not decide
//
// It decides *structure* — that "a > b" is a prelude of five component values
// and that "color: red" is a declaration named "color". It decides nothing about
// *meaning*: it does not know that "color" is a property, that "a > b" is a
// selector, or that "@media" takes a query. That is deliberate and it is what
// the specification's own layering says. A stylesheet full of properties this
// engine has never heard of parses here exactly as well as one full of
// properties it implements, which is what lets the layer above report each
// unsupported declaration rather than failing the file.
//
// # Recovery
//
// Parsing never fails, for the same reason tokenizing never fails: one broken
// rule must not cost the author the rest of the stylesheet. Every malformed
// construct has a defined recovery, and each one is reported as an Error so a
// caller can say what was wrong without the reading of the file depending on it.

// maxNestingDepth bounds how deeply blocks and functions may nest.
//
// Each level of "(", "[", "{" or a function costs a stack frame, and a
// stylesheet is untrusted input: a few hundred kilobytes of "(((((((..." would
// otherwise exhaust the goroutine stack, which aborts the process in a way no
// recover can catch. Real CSS nests a handful of levels — calc(1px + var(--x))
// is three — so this is far above anything an author writes and far below
// anything that hurts.
//
// Hitting it is not an error the way a PDF's depth cap is. Parsing stays total:
// the offending block is consumed to its matching close so the token stream
// stays in step, its contents are dropped, and the trip is reported. See
// skipBlock.
const maxNestingDepth = 128

// A ComponentValue is one node of a parsed stylesheet: a preserved token, a
// function call, or a block delimited by (), [] or {}.
//
// Which of the three it is, is read off Token.Kind, and no separate tag is
// needed because the parser never leaves the ambiguous kinds preserved. A
// Function token always became a function, and an opening delimiter always
// became a block, so:
//
//   - Token.Kind == Function — a function. Token.Value is its name, Values its
//     arguments.
//   - Token.Kind == LeftParen, LeftSquare or LeftBrace — a block. Values is its
//     contents.
//   - anything else — a preserved token, and Values is nil.
//
// The methods below say the same thing without the caller having to remember it.
type ComponentValue struct {
	// Token is the preserved token, the function token, or the block's opening
	// delimiter, depending on which of the three this node is.
	Token Token

	// Values is the contents of a block or the arguments of a function, and nil
	// for a preserved token.
	//
	// A block's delimiters are not in it: they are Token and its mirror, and
	// the closing one may not have been present at all if the input ended
	// early. This is why the closing delimiter is not kept — after recovery
	// there may not be one to keep.
	Values []ComponentValue
}

// IsFunction reports whether this node is a function call.
func (c ComponentValue) IsFunction() bool { return c.Token.Kind == Function }

// IsBlock reports whether this node is a (), [] or {} block.
func (c ComponentValue) IsBlock() bool {
	switch c.Token.Kind {
	case LeftParen, LeftSquare, LeftBrace:
		return true
	}
	return false
}

// IsToken reports whether this node is a preserved token — neither a function
// nor a block.
func (c ComponentValue) IsToken() bool { return !c.IsFunction() && !c.IsBlock() }

// A Declaration is a property name and the value assigned to it.
type Declaration struct {
	// Name is the property name as written, with escapes resolved and the case
	// as the author typed it. Property names are matched case-insensitively, so
	// a caller comparing this must fold case rather than compare directly.
	Name string

	// Value is the component values between the colon and the end of the
	// declaration, with the "!important" removed if it was there and with
	// leading and trailing whitespace stripped. Whitespace *within* the value
	// is kept, because it separates the parts of a shorthand.
	Value []ComponentValue

	// Important reports that the declaration ended with "!important", which
	// changes where it sorts in the cascade.
	Important bool

	// Offset is the byte offset in the source at which the property name
	// begins.
	Offset int
}

// A Rule is either a qualified rule — a style rule, whose prelude is a selector
// list — or an at-rule such as @media or @page.
//
// The two are one type because the parser cannot tell what either means: at this
// layer a qualified rule is "some component values, then a {} block", and that
// is all that distinguishes it from an at-rule beginning with "@".
type Rule struct {
	// At reports whether this is an at-rule. It is a field rather than a test
	// on Name because it is the one thing that genuinely separates the two
	// kinds, and reading it should not depend on knowing that a qualified
	// rule's name is empty.
	At bool

	// Name is an at-rule's name without the "@" — "media", "page", "import".
	// It is empty for a qualified rule.
	Name string

	// Prelude is everything before the block: an at-rule's parameters, or a
	// qualified rule's selector list, still unparsed.
	Prelude []ComponentValue

	// Block is the contents of the {} block, and HasBlock says whether there
	// was one. The two are separate because "@import url(x);" has no block and
	// "@media print {}" has an empty one, and a caller has to tell them apart.
	Block    []ComponentValue
	HasBlock bool

	// Offset is the byte offset in the source at which the rule begins.
	Offset int
}

// ParseComponentValues parses a list of component values (§5.3.10).
//
// This is the entry point for anything that is a *value* rather than a
// stylesheet: the prelude of a rule, the value of a declaration, the arguments
// of a media query.
func ParseComponentValues(input string) ([]ComponentValue, []Error) {
	p := newParser(input)
	out := p.componentValues()
	return out, p.report()
}

// ParseStylesheet parses a stylesheet (§5.3.3): a list of rules, at the top
// level, where "<!--" and "-->" are ignored rather than being read as the start
// of a qualified rule.
func ParseStylesheet(input string) ([]Rule, []Error) {
	p := newParser(input)
	out := p.rules(true)
	return out, p.report()
}

// ParseRules parses a list of rules that is not a whole stylesheet (§5.3.4) —
// the contents of an @media block, say. The difference from ParseStylesheet is
// only the handling of "<!--" and "-->", which are historical and which a nested
// context has no reason to ignore.
func ParseRules(input string) ([]Rule, []Error) {
	p := newParser(input)
	out := p.rules(false)
	return out, p.report()
}

// ParseDeclarations parses a list of declarations (§5.3.6): the contents of a
// style rule's block.
//
// Declarations and rules are returned separately rather than interleaved. Both
// carry an Offset, so a caller that does need source order can recover it
// without this returning a sum type that every caller would then have to switch
// on — and the one caller that needs it, the cascade, needs something else
// instead: the *nesting*, which the two lists give it directly.
//
// The rules are CSS Nesting's. A style rule among the declarations is one, and
// startsANestedRule is how it is told from a declaration whose colon went
// missing; an at-rule among them is a conditional group the layer above still
// reports as unsupported, exactly as it does at the top of a stylesheet.
func ParseDeclarations(input string) ([]Declaration, []Rule, []Error) {
	p := newParser(input)
	decls, rules := p.declarations()
	return decls, rules, p.report()
}

// ParseDeclarationValues is ParseDeclarations over already-parsed input, which
// is what reading the block of a rule that has already been parsed needs.
//
// Re-tokenizing the block's source text instead would be wrong as well as
// wasteful: the nesting was worked out once already, and a second pass over text
// that was recovered from — an unclosed function, say — need not reach the same
// answer.
func ParseDeclarationValues(block []ComponentValue) ([]Declaration, []Rule, []Error) {
	p := &parser{vals: block}
	decls, rules := p.declarations()
	return decls, rules, p.report()
}

// ParseRulesFromValues is ParseRules over already-parsed component values, which
// is what reading the body of an @media block needs.
func ParseRulesFromValues(block []ComponentValue) ([]Rule, []Error) {
	p := &parser{vals: block}
	out := p.rules(false)
	return out, p.report()
}

// parser walks either a token stream or an already-parsed list of component
// values.
//
// Two inputs rather than one because the specification's algorithms are defined
// over both: a stylesheet is parsed from tokens, while the block of a rule that
// has already been parsed is a list of component values, and re-tokenizing it
// would lose the nesting that was already worked out. Which one is live is
// decided by fromValues.
//
// # The token stream is pulled, not materialised
//
// The tokens are read from the tokenizer as the parser reaches them, into a
// window that holds only those not yet consumed. They used to be tokenized whole
// first, into a slice held for the length of the parse beside the tree being
// built from it — and a token is seventy-two bytes, a stylesheet can be a token
// per byte, and every one of them is copied into that tree anyway. With the
// slice's growth on top, a megabyte of "a{b:,,,…}" peaked at over four hundred
// bytes held for each byte of the sheet.
//
// Nothing reads behind the position being consumed, and nothing reads far
// ahead of it into the window: the one look-ahead, startsANestedRule, reads
// with a copy of the tokenizer instead. So the window holds the token being
// looked at and not much else, whatever the length of the sheet.
type parser struct {
	// tz is the token stream, and nil for a parser over component values.
	tz *tokenizer
	// window is the tokens pulled from tz and not yet dropped: window[0] is the
	// token at position base. ended records that tz has produced its EOF,
	// which is not kept in the window — tokenAt makes one for any position at
	// or past the end, as reading past the end of the old slice did.
	window []Token
	base   int
	ended  bool

	vals []ComponentValue

	// stack is where the values of every list still being read are gathered,
	// innermost last. See take.
	stack gathered

	pos   int
	depth int
	// ahead is the grouping startsANestedRule keeps as it reads raw tokens
	// ahead. It is a field so that its room is reused from one declaration to
	// the next rather than allocated for each one that nests.
	ahead closers
	// errs is what the parser itself found, in the order it found it. It is
	// laid after the tokenizer's by report, under one bound for the two.
	errs []Error
}

func newParser(input string) *parser {
	return &parser{tz: newTokenizer(input)}
}

// fromValues reports whether this parser walks component values rather than
// tokens.
func (p *parser) fromValues() bool { return p.tz == nil }

// tokenAt is the token at an absolute position of the stream, pulling it from
// the tokenizer if it has not been read yet. Past the end it is an EOF token.
func (p *parser) tokenAt(i int) Token {
	for i-p.base >= len(p.window) {
		if p.ended {
			return Token{Kind: EOF, Offset: p.tz.end}
		}
		p.pull()
	}
	return p.window[i-p.base]
}

// pull reads one more token into the window.
//
// When the window is full, the tokens already consumed are dropped to make room
// if they are at least half of it, and otherwise it doubles. Either way the
// cost is paid at most once per token, so reading the stream is linear, and
// what the window holds is never more than twice what has been pulled and not
// yet consumed — which, with nothing but peek pulling, is a token.
func (p *parser) pull() {
	if len(p.window) == cap(p.window) {
		if dead := p.pos - p.base; dead > 0 && 2*dead >= len(p.window) {
			n := copy(p.window, p.window[dead:])
			clear(p.window[n:])
			p.window = p.window[:n]
			p.base = p.pos
		} else {
			grown := make([]Token, len(p.window), 2*cap(p.window)+1)
			copy(grown, p.window)
			p.window = grown
		}
	}
	tok := p.tz.token()
	if tok.Kind == EOF {
		p.ended = true
		return
	}
	p.window = append(p.window, tok)
}

// take is the values gathered on the stack since mark, in a slice of their own
// and of exactly their length, and pops them.
//
// Every list the parser builds — a block's contents, a function's arguments, a
// prelude, a declaration — is gathered on the stack and copied out once it is
// complete, rather than appended to where it will live. Appending grows a slice
// by a quarter at a time once it is large, so a block of a million values was
// copied a dozen times over on its way to its size and kept up to a quarter
// again in capacity nothing would use: the tree of "a{b:,,,…}" allocated five
// times what it ended up holding. A list inside a list is gathered above its
// parent's values and taken off before the parent adds it, which is why one
// stack serves the nesting. See gathered for why the stack itself never copies.
//
// A parser over component values gathers nothing. What it reads is a list
// already, one value to a position, so the values since mark are the stretch of
// that list between mark and where it stands, and that stretch is the list:
// the declarations of a block of a million values are read without a value of
// it being copied, where gathering them made the block twice over. The stretch
// is capped at its own length, so a caller that appends to it gets a slice of
// its own rather than writing over the values after it — and nothing here or
// above writes into one. A block met in a list of values was already handed
// back as the node's own list, for the same reason; see takeBlock.
//
// An empty list is nil, as the appends it replaced left it: a caller tells a
// block with nothing in it by its length, and a declaration value that is never
// an empty non-nil slice is pinned by a test of its own.
func (p *parser) take(mark int) []ComponentValue {
	if p.fromValues() {
		if p.pos == mark {
			return nil
		}
		return p.vals[mark:p.pos:p.pos]
	}
	return p.stack.take(mark)
}

// mark is where a list about to be gathered begins: a height of the stack, or a
// position in the values being read. Everything read between it and the take
// that ends the list has to be pushed, and nothing else may be read, which is
// why a closing delimiter is consumed after the take and not before.
func (p *parser) mark() int {
	if p.fromValues() {
		return p.pos
	}
	return p.stack.len()
}

// push adds a value to the list being gathered.
func (p *parser) push(c ComponentValue) {
	if !p.fromValues() {
		p.stack.push(c)
	}
}

// gathered is the parser's stack of values not yet in a list of their own.
//
// It is kept in chunks rather than in one slice, so that growing it copies
// nothing: a slice that doubles holds up to twice what it needs and copies what
// it holds each time, and for the one list of a million values that a sheet can
// be, that was more than the list. Chunks are allocated as the stack first grows
// into them and kept for reuse by every list after, doubling from one value —
// most lists are one to ten, and a parse of "12px" should not pay for a hundred
// — up to a size past which a bigger chunk would save nothing but a few
// pointers.
//
// So what the stack holds is never more than its deepest fill plus one chunk,
// and a list is copied exactly once: out of here, into the slice it keeps.
type gathered struct {
	// chunks are the chunks allocated so far. Those below top are full, top
	// is the one being filled, and any above it are empty and waiting.
	chunks [][]ComponentValue
	top    int
	// n is how many values are held.
	n int
}

// The chunk sizes: the first, and the one past which they stop doubling.
const (
	firstChunk = 1
	maxChunk   = 4096
)

// len is how many values are held, which is the mark a list begins at.
func (g *gathered) len() int { return g.n }

func (g *gathered) push(c ComponentValue) {
	switch {
	case len(g.chunks) == 0:
		g.chunks = append(g.chunks, make([]ComponentValue, 0, firstChunk))
		g.top = 0
	case len(g.chunks[g.top]) == cap(g.chunks[g.top]):
		g.top++
		if g.top == len(g.chunks) {
			g.chunks = append(g.chunks, make([]ComponentValue, 0, min(2*cap(g.chunks[g.top-1]), maxChunk)))
		}
	}
	g.chunks[g.top] = append(g.chunks[g.top], c)
	g.n++
}

func (g *gathered) take(mark int) []ComponentValue {
	count := g.n - mark
	if count == 0 {
		return nil
	}
	out := make([]ComponentValue, count)
	// From the top down, since the values since mark are the top of the stack.
	for w := count; w > 0; {
		ch := g.chunks[g.top]
		k := min(len(ch), w)
		copy(out[w-k:w], ch[len(ch)-k:])
		// Cleared so that a chunk does not keep the popped values' own lists
		// reachable after the tree that holds them has been dropped.
		clear(ch[len(ch)-k:])
		g.chunks[g.top] = ch[:len(ch)-k]
		w -= k
		if len(g.chunks[g.top]) == 0 && g.top > 0 {
			g.top--
		}
	}
	g.n = mark
	return out
}

// fail records a problem. It keeps one past the bound, and no more, because
// that is as many as report can use: the one past it is where the note that the
// list was cut goes.
func (p *parser) fail(off int, msg string) {
	if len(p.errs) <= maxErrors {
		p.errs = append(p.errs, Error{Offset: off, Message: msg})
	}
}

// report is the problems of the whole parse: the tokenizer's, then the
// parser's, under the one maxErrors bound.
//
// That is the order and the bound they had when the input was tokenized whole
// before it was parsed, and pulling the tokens as they are needed is not a
// reason for a caller to see a different list. So the rest of the stream is
// read first — every entry point reads to the end already, and this makes it
// true by construction rather than by each of them — and the parser's own
// problems are laid after the tokenizer's exactly as they would have been had
// the tokenizer's all been found first.
func (p *parser) report() []Error {
	var out []Error
	if p.tz != nil {
		for !p.ended {
			if p.tz.token().Kind == EOF {
				p.ended = true
			}
		}
		out = p.tz.errs
	}
	for _, e := range p.errs {
		out = addError(out, e)
	}
	return out
}

// peek returns the next node without consuming it. Past the end it returns an
// EOF token, so no caller has to bounds-check.
func (p *parser) peek() ComponentValue {
	if p.fromValues() {
		if p.pos < len(p.vals) {
			return p.vals[p.pos]
		}
		return ComponentValue{Token: Token{Kind: EOF, Offset: p.endOffset()}}
	}
	return ComponentValue{Token: p.tokenAt(p.pos)}
}

// endOffset is where the input ended, for a diagnostic that has run off the end.
func (p *parser) endOffset() int {
	if p.tz != nil {
		return p.tz.end
	}
	if n := len(p.vals); n > 0 {
		return p.vals[n-1].Token.Offset
	}
	return 0
}

func (p *parser) next() ComponentValue {
	c := p.peek()
	if c.Token.Kind != EOF {
		p.pos++
	}
	return c
}

func (p *parser) atEOF() bool { return p.peek().Token.Kind == EOF }

func (p *parser) skipWhitespace() {
	for p.peek().Token.Kind == Whitespace {
		p.pos++
	}
}

// componentValues consumes to the end of the input (§5.4.7 in the list form).
func (p *parser) componentValues() []ComponentValue {
	mark := p.mark()
	for !p.atEOF() {
		p.push(p.componentValue())
	}
	return p.take(mark)
}

// componentValue consumes one component value (§5.4.6): a block, a function, or
// a preserved token.
func (p *parser) componentValue() ComponentValue {
	c := p.next()

	// A parser walking component values has already done this work: what it
	// holds are blocks and functions, not the delimiters that open them.
	if p.fromValues() {
		return c
	}

	switch c.Token.Kind {
	case LeftBrace, LeftSquare, LeftParen:
		return p.block(c.Token)
	case Function:
		return p.function(c.Token)
	}
	return c
}

// mirror is the token that closes what open opens.
func mirror(open Kind) Kind {
	switch open {
	case LeftBrace:
		return RightBrace
	case LeftSquare:
		return RightSquare
	case LeftParen:
		return RightParen
	}
	return EOF
}

// block consumes a simple block (§5.4.7), the opening delimiter already
// consumed.
func (p *parser) block(open Token) ComponentValue {
	if p.depth >= maxNestingDepth {
		p.fail(open.Offset, "blocks are nested too deeply to read")
		p.skipBlock(mirror(open.Kind))
		return ComponentValue{Token: open}
	}
	p.depth++
	defer func() { p.depth-- }()

	end := mirror(open.Kind)
	mark := p.mark()
	for {
		switch c := p.peek(); {
		case c.Token.Kind == end:
			out := ComponentValue{Token: open, Values: p.take(mark)}
			p.pos++
			return out
		case c.Token.Kind == EOF:
			// An unclosed block still yields what it held. Discarding it would
			// throw away every rule in a stylesheet whose last brace is
			// missing, which is the commonest way a stylesheet is broken.
			p.fail(open.Offset, "a block that is never closed")
			return ComponentValue{Token: open, Values: p.take(mark)}
		default:
			p.push(p.componentValue())
		}
	}
}

// function consumes a function (§5.4.8), the function token already consumed.
func (p *parser) function(name Token) ComponentValue {
	if p.depth >= maxNestingDepth {
		p.fail(name.Offset, "functions are nested too deeply to read")
		p.skipBlock(RightParen)
		return ComponentValue{Token: name}
	}
	p.depth++
	defer func() { p.depth-- }()

	mark := p.mark()
	for {
		switch c := p.peek(); {
		case c.Token.Kind == RightParen:
			out := ComponentValue{Token: name, Values: p.take(mark)}
			p.pos++
			return out
		case c.Token.Kind == EOF:
			p.fail(name.Offset, "a function call that is never closed")
			return ComponentValue{Token: name, Values: p.take(mark)}
		default:
			p.push(p.componentValue())
		}
	}
}

// skipBlock discards input up to and including the delimiter that closes the
// block already open, without recursing.
//
// This is what keeps the depth cap from being a correctness bug. Stopping at the
// first close delimiter would leave the reader inside a nested block, and every
// rule after it would be misparsed; recursing to find the right one is the
// stack exhaustion the cap exists to prevent. So nesting is kept on the heap
// instead, and the cap costs only the contents of one absurdly nested block.
//
// What is kept is the whole grouping, in closers, and not a count of the one
// delimiter being waited for. A count finds the right ")" only when nothing
// of another kind is open inside: in "( [ ) ] )" the first ")" is inside the
// "[" block, a token like any other, and a count of parentheses stopped there
// and left "] )" to be read as though the author had closed the block. Past the
// cap, that is where the reader and a browser parted.
func (p *parser) skipBlock(end Kind) {
	if p.fromValues() {
		// Not reached: a parser over values opens no block, because what it
		// walks are blocks already, so it never has one to skip.
		return
	}
	open := closers{end}
	for !p.atEOF() {
		if open.take(p.next().Token.Kind); len(open) == 0 {
			return
		}
	}
}

// closers is the delimiters that close the blocks and functions a reading of
// raw tokens is inside, innermost last. It is how a scan that does not build
// the tree still groups the tokens exactly as the tree would (§5.4.7 and
// §5.4.8): an opening delimiter or a function token opens a level, the mirror
// of the innermost level closes it, and a closing delimiter of any other kind
// is a token like any other inside it — "( ] )" is one block holding a "]",
// and "f({})" one function holding a {} block.
//
// A scan that counted one kind of delimiter, or counted them all together,
// disagreed with the tree on exactly those inputs. Two scans read raw tokens
// this way: skipBlock, past the depth cap, and startsANestedRule's look-ahead
// in a declaration list read from text.
type closers []Kind

// take reads one token's kind into the grouping. A closing delimiter with no
// level open is the caller's to interpret; take leaves the grouping as it is.
func (o *closers) take(k Kind) {
	switch k {
	case LeftBrace, LeftSquare, LeftParen:
		*o = append(*o, mirror(k))
	case Function:
		*o = append(*o, RightParen)
	case RightBrace, RightSquare, RightParen:
		if n := len(*o); n > 0 && (*o)[n-1] == k {
			*o = (*o)[:n-1]
		}
	}
}

// rules consumes a list of rules (§5.4.1). At the top level of a stylesheet
// "<!--" and "-->" are skipped; anywhere else they begin a qualified rule.
func (p *parser) rules(topLevel bool) []Rule {
	var out []Rule
	for {
		switch c := p.peek(); {
		case c.Token.Kind == EOF:
			return out

		case c.Token.Kind == Whitespace:
			p.pos++

		case c.Token.Kind == CDO || c.Token.Kind == CDC:
			if topLevel {
				p.pos++
				continue
			}
			if r, ok := p.qualifiedRule(); ok {
				out = append(out, r)
			}

		case c.Token.Kind == AtKeyword:
			out = append(out, p.atRule())

		default:
			if r, ok := p.qualifiedRule(); ok {
				out = append(out, r)
			}
		}
	}
}

// atRule consumes an at-rule (§5.4.2), positioned at the at-keyword.
func (p *parser) atRule() Rule {
	at := p.next()
	out := Rule{At: true, Name: at.Token.Value, Offset: at.Token.Offset}
	mark := p.mark()
	for {
		switch c := p.peek(); {
		case c.Token.Kind == Semicolon:
			out.Prelude = p.take(mark)
			p.pos++
			return out

		case c.Token.Kind == EOF:
			// A statement at-rule needs no block, so running out of input is
			// only an error if the rule was still collecting its prelude. It
			// was, or we would have returned; but "@import 'x'" with no
			// semicolon is so common that reporting it would be noise.
			out.Prelude = p.take(mark)
			return out

		case c.Token.Kind == LeftBrace:
			out.Prelude = p.take(mark)
			out.Block, out.HasBlock = p.takeBlock(c), true
			return out

		default:
			p.push(p.componentValue())
		}
	}
}

// takeBlock consumes a {} block and returns its contents.
//
// It exists because the two inputs the specification defines these algorithms
// over reach a block differently: in a token stream the "{" is a delimiter and
// the contents are still ahead, while in a list of component values the block is
// a single node whose contents were worked out already.
func (p *parser) takeBlock(c ComponentValue) []ComponentValue {
	p.pos++
	if p.fromValues() {
		return c.Values
	}
	return p.block(c.Token).Values
}

// qualifiedRule consumes a qualified rule (§5.4.3). It returns ok=false when the
// input ended before the block, which the specification discards: a selector
// with no body is not a rule, and keeping it would let a truncated file apply
// styles its author never wrote.
func (p *parser) qualifiedRule() (Rule, bool) {
	out := Rule{Offset: p.peek().Token.Offset}
	mark := p.mark()
	for {
		switch c := p.peek(); {
		case c.Token.Kind == EOF:
			p.fail(out.Offset, "a rule with no block: the \"{\" is missing")
			// Popped though it is thrown away, or it would sit under whatever
			// list is gathered next.
			p.take(mark)
			return Rule{}, false

		case c.Token.Kind == LeftBrace:
			out.Prelude = p.take(mark)
			out.Block, out.HasBlock = p.takeBlock(c), true
			return out, true

		default:
			p.push(p.componentValue())
		}
	}
}

// lookAt is the component value at an absolute position of a parser over
// component values, for a look-ahead: the nesting has already been worked out,
// so a block is one value.
//
// There is no token-stream form of it, and that is deliberate. Looking ahead in
// the stream by position pulls every token passed over into the window and
// keeps it there; startsANestedRule reads ahead with a copy of the tokenizer
// instead.
func (p *parser) lookAt(i int) ComponentValue {
	if i < len(p.vals) {
		return p.vals[i]
	}
	return ComponentValue{Token: Token{Kind: EOF, Offset: p.endOffset()}}
}

// startsANestedRule reports whether what comes next in a declaration block is a
// style rule rather than a declaration.
//
// CSS Nesting lets the two stand side by side inside one block, and the rule for
// telling them apart is where the first "{" falls: a declaration ends at a
// semicolon, so a block that opens before one is not part of a declaration.
//
// It has to be a look-ahead rather than a try-and-see. A nested rule is not
// terminated by a semicolon at all, so consuming up to the next one and finding
// it was a rule would already have swallowed every rule after it — "a {} b {}"
// is two rules and one run.
func (p *parser) startsANestedRule() bool {
	if p.fromValues() {
		// Already grouped, so nothing inside a block or a function is at this
		// level, and each is one value to step over. A {} block among them is
		// the rule's own.
		for i := p.pos; ; i++ {
			switch c := p.lookAt(i); {
			case c.IsBlock() && c.Token.Kind == LeftBrace:
				return true
			case c.Token.Kind == EOF, c.Token.Kind == Semicolon, c.Token.Kind == RightBrace:
				return false
			}
		}
	}
	// A token stream is read ahead without being kept. The tokens already in
	// the window are looked at where they are, and the rest are read by a copy
	// of the tokenizer that is then thrown away, so the parser reads them again
	// from its own when it gets to them.
	//
	// Reading into the window instead would hold every token of the stretch
	// looked over, and a declaration is as long as its author makes it: the
	// one declaration of "b:,,,…" made the window the whole of the input, which
	// is the slice the stream exists not to have. Tokenizing twice is linear,
	// because the parser always consumes at least as far as this looked — it
	// stops only at a ";", "{" or "}" outside any block, and this stops at the
	// first of those outside any block too, grouping the tokens as the parser
	// will (see closers) — so no token is looked ahead over twice.
	//
	// The grouping is the point. The tokens here are raw, so a "(" or a
	// function token opens a level that the look-ahead has to keep: the "{" of
	// "--x: f({}); color: red" is inside the function, and taking it for a
	// rule's block lost the declaration after it. And keeping it exactly as
	// the parser does is what the linear bound above rests on, since a
	// look-ahead that thought a block still open where the parser had closed
	// it would read past where the parser stops, again for each declaration.
	p.ahead = p.ahead[:0]
	for i := p.pos - p.base; i < len(p.window); i++ {
		if done, nested := nestedRuleStep(p.window[i].Kind, &p.ahead); done {
			return nested
		}
	}
	if p.ended {
		return false
	}
	probe := *p.tz
	// Quiet, and with a list of its own: anything wrong ahead is reported when
	// the parser's own tokenizer reads it, which it will.
	probe.errs, probe.quiet = nil, true
	for {
		if done, nested := nestedRuleStep(probe.token().Kind, &p.ahead); done {
			return nested
		}
	}
}

// nestedRuleStep is startsANestedRule's reading of one raw token: whether it
// decides the question, and which way. open is the blocks and functions the
// look-ahead is inside.
func nestedRuleStep(k Kind, open *closers) (done, nested bool) {
	if k == EOF {
		return true, false
	}
	if len(*open) == 0 {
		switch k {
		case LeftBrace:
			return true, true
		case Semicolon:
			return true, false
		case RightBrace:
			// The end of the block this declaration list is in.
			return true, false
		}
	}
	open.take(k)
	return false, false
}

// declarations consumes a list of declarations (§5.4.4), and the style rules
// CSS Nesting allows among them.
func (p *parser) declarations() ([]Declaration, []Rule) {
	var decls []Declaration
	var rules []Rule
	for {
		switch c := p.peek(); {
		case c.Token.Kind == EOF:
			return decls, rules

		case c.Token.Kind == Whitespace || c.Token.Kind == Semicolon:
			p.pos++

		case c.Token.Kind == AtKeyword:
			rules = append(rules, p.atRule())

		case p.startsANestedRule():
			// Before the Ident case, because a nested rule usually begins with
			// one: "span { color: blue }" is a type selector and not a property
			// whose colon went missing, which is what it was read as until the
			// look-ahead above existed.
			if r, ok := p.qualifiedRule(); ok {
				rules = append(rules, r)
			}

		case c.Token.Kind == Ident:
			// The declaration is parsed from the run up to the next semicolon,
			// so that a malformed one cannot swallow the declarations after it.
			run := p.until(Semicolon)
			if d, ok := parseDeclarationFrom(run, p); ok {
				decls = append(decls, d)
			}

		default:
			p.fail(c.Token.Offset, "expected a property name")
			p.until(Semicolon)
		}
	}
}

// until collects component values up to, but not including, the next top-level
// token of the given kind.
func (p *parser) until(end Kind) []ComponentValue {
	mark := p.mark()
	for {
		c := p.peek()
		if c.Token.Kind == EOF || c.Token.Kind == end {
			return p.take(mark)
		}
		p.push(p.componentValue())
	}
}

// parseDeclarationFrom consumes a declaration from a run of component values
// (§5.4.5). Errors are reported through owner, which holds the shared budget.
func parseDeclarationFrom(run []ComponentValue, owner *parser) (Declaration, bool) {
	q := &parser{vals: run, errs: owner.errs}
	defer func() { owner.errs = q.errs }()

	name := q.next()
	if name.Token.Kind != Ident {
		q.fail(name.Token.Offset, "expected a property name")
		return Declaration{}, false
	}
	out := Declaration{Name: name.Token.Value, Offset: name.Token.Offset}

	q.skipWhitespace()
	if c := q.peek(); c.Token.Kind != Colon {
		q.fail(c.Token.Offset, "expected \":\" after the property name "+quoteName(out.Name))
		return Declaration{}, false
	}
	q.pos++
	q.skipWhitespace()

	// The rest of the run is the value, trimmed where it lies. The run is the
	// declaration's own — until gathered it into a slice of its own length and
	// nothing else holds it — so the value can stay where it is, and does when
	// it is most of the run. When it is not, it is copied out at exactly its
	// length instead, because keeping it where it lies keeps the name, the
	// colon and whatever was trimmed for as long as the declaration lives: for
	// "b:c", three values held to keep one. The line between the two is an
	// eighth, so a value is never kept at more than that over its own size and
	// the one long declaration a sheet can be is not copied a second time.
	value, important := takeImportant(run[q.pos:])
	if value = trimTrailingWhitespace(value); value != nil {
		if waste := len(run) - len(value); 8*waste <= len(value) {
			out.Value = value
		} else {
			out.Value = make([]ComponentValue, len(value))
			copy(out.Value, value)
		}
	}
	out.Important = important
	return out, true
}

// takeImportant removes a trailing "!important" and reports whether it was
// there.
//
// The two tokens need not be adjacent — "! important" is legal, and so is a
// comment between them, which the tokenizer already removed. So this looks at
// the last two values that are not whitespace rather than at the last two
// values, and removes exactly those two along with anything between them.
func takeImportant(vals []ComponentValue) ([]ComponentValue, bool) {
	// The order is "!" then "important", so the last of the two is the ident.
	word := lastNonWhitespace(vals, len(vals))
	if word < 0 {
		return vals, false
	}
	if v := vals[word]; !v.IsToken() || v.Token.Kind != Ident ||
		!strings.EqualFold(v.Token.Value, "important") {
		return vals, false
	}

	bang := lastNonWhitespace(vals, word)
	if bang < 0 {
		return vals, false
	}
	if v := vals[bang]; !v.IsToken() || !v.Token.IsDelim('!') {
		return vals, false
	}

	// Everything before the "!" is the value; whatever sat between the "!" and
	// the "important" was whitespace and goes with them.
	return vals[:bang], true
}

// lastNonWhitespace returns the index of the last value before limit that is not
// whitespace, or -1.
func lastNonWhitespace(vals []ComponentValue, limit int) int {
	for i := limit - 1; i >= 0; i-- {
		if !(vals[i].IsToken() && vals[i].Token.Kind == Whitespace) {
			return i
		}
	}
	return -1
}

func trimTrailingWhitespace(vals []ComponentValue) []ComponentValue {
	n := lastNonWhitespace(vals, len(vals)) + 1
	if n == 0 {
		return nil
	}
	return vals[:n]
}

// quoteName renders a property name for a diagnostic without letting a hostile
// stylesheet put control characters into a caller's log.
func quoteName(s string) string {
	const max = 40
	var b strings.Builder
	b.WriteByte('"')
	for i, r := range s {
		if i >= max {
			b.WriteString("...")
			break
		}
		if r < 0x20 || r == 0x7F {
			b.WriteByte('?')
			continue
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}
