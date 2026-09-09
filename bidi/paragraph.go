package bidi

// The paragraph-level entry points, for a caller laying text out rather than
// shaping it.
//
// The rest of the package answers the question shaping asks — "which stretches
// of this string run which way" — and answers it for a whole string at once. A
// layout engine asks a longer question: it resolves a paragraph once, then
// breaks it into lines it did not know about beforehand, and rule L1 has to be
// applied to each of those lines separately because it is stated in terms of
// where a line ends. So the paragraph is kept, and asked about a range at a
// time.

// Direction is the base direction a paragraph is laid out in.
type Direction int8

const (
	// Auto is rules P2 and P3: the direction of the first strong character,
	// left to right if there is none. It is what "unicode-bidi: plaintext"
	// asks for, and what a paragraph whose direction nobody stated gets.
	Auto Direction = iota
	// LeftToRight and RightToLeft are what CSS's direction property sets:
	// paragraph embedding level 0 and 1 respectively.
	LeftToRight
	RightToLeft
)

// Resolve runs the algorithm over one paragraph.
//
// The text is one paragraph and not a document: rule P1 splits text into
// paragraphs at a paragraph separator, and that split belongs to the caller —
// in a layout engine a forced line break ends a bidi paragraph, and the engine
// knows where those are long before this is called.
func Resolve(text []rune, dir Direction) *Paragraph {
	classes := make([]Class, len(text))
	for i, r := range text {
		classes[i] = ClassOf(r)
	}
	level := 0
	switch dir {
	case RightToLeft:
		level = 1
	case Auto:
		pdi, _ := matchPDI(classes)
		level = paraLevelOf(classes, pdi, 0, len(classes))
	}
	p := resolveClasses(classes, text, level)
	p.fillRemoved()
	return &p
}

// Level is the paragraph embedding level, 0 or 1.
func (p *Paragraph) Level() int { return p.para }

// Len is the number of characters resolved.
func (p *Paragraph) Len() int { return len(p.levels) }

// Levels is the resolved embedding level of each character, in logical order.
//
// A character rule X9 removed — an embedding control, an override, a boundary
// neutral — has no level of its own, and carries the level of what precedes it:
// Resolve fills those in before it returns, so that a caller placing one has an
// answer rather than a sentinel to interpret. The first characters of a
// paragraph, where there is nothing before them, take the paragraph's own
// level. See fillRemoved.
//
// This said the slice carries -1 there, which it has not since the filling was
// added — so a caller reading the documentation wrote a branch for a value that
// never arrives, and one testing for it would conclude the paragraph had no
// controls in it. The slice is the paragraph's own and must not be modified.
//
// L1 has already been applied to these, with the whole paragraph taken as one
// line — which is the only line there is until a caller says otherwise. That is
// always a subset of what LineLevels will do, since a shorter line has its end
// in more places and L1 only ever resets *to* the paragraph level, so asking
// LineLevels afterwards neither double-counts nor contradicts it. A caller
// breaking the paragraph into lines should use LineLevels for each and not
// these.
func (p *Paragraph) Levels() []int { return p.levels }

// LineLevels is the part of rule L1 that only a line boundary can settle.
//
// L1 has four clauses and Resolve has already applied all of them, with the
// whole paragraph taken as one line. Three of the four are finished by that:
// segment and paragraph separators reset wherever they are, and so does white
// space before one. Their positions do not move when the text is broken into
// lines, so there is nothing here to do about them — and repeating the clauses
// would be code no input could reach.
//
// What is left is the fourth: white space at the end of a *line*, which the
// paragraph could not know about because the lines did not exist yet. It takes
// the paragraph's own direction so that it hangs on the correct side —
// otherwise a line of Hebrew ending in a space puts that space at the left of
// the line, where nothing follows it.
//
// The range is in characters of the paragraph. The returned slice is fresh and
// indexed from the start of the line, which is what VisualOrder expects.
func (p *Paragraph) LineLevels(start, end int) []int {
	if start < 0 {
		start = 0
	}
	if end > len(p.levels) {
		end = len(p.levels)
	}
	if start >= end {
		return nil
	}
	out := make([]int, end-start)
	copy(out, p.levels[start:end])

	// Backwards from the end of the line, which is the only place clause 4
	// starts from. It stops at the first character that marks the page.
	//
	// A character rule X9 removed is neither reset nor a stop *by itself*. §5.2
	// says two things about one and they are not in conflict: it takes the level
	// of the character before it, "so that the character does not interrupt a
	// run", and it joins a sequence of white space that L1 resets. So a PDF in
	// the middle of a run of spaces is part of that run, and a PDF with a letter
	// in front of it is part of the letter — it is held until the scan learns
	// which, and dropped if the scan stops before it finds white space.
	//
	// Resetting one on sight was what this did, and it contradicted the levels
	// Resolve had already settled: "a<LRO>b<RLO>c<PDF>" gave the trailing PDF
	// the paragraph's level here and c's level there, so LineLevels over the
	// whole paragraph disagreed with Levels — which the note on Levels says
	// cannot happen. A caller splitting runs where the level changes got one
	// extra run of a character that marks no paper.
	var held []int
	for i := end - 1; i >= start; i-- {
		c := p.classes[i]
		switch {
		case isRemoved(c):
			held = append(held, i-start)
			continue
		case c == WS || c == LRI || c == RLI || c == FSI || c == PDI:
			for _, at := range held {
				out[at] = p.para
			}
			held = held[:0]
			out[i-start] = p.para
		default:
			return out
		}
	}
	return out
}

// FirstStrong is rules P2 and P3 on their own: the direction the first strong
// character of a paragraph asks for, and whether there was one.
//
// Resolve answers P3's "otherwise, set it to zero" itself, which is the right
// answer for a paragraph nobody has any other information about. A caller that
// *does* have other information needs the question without the answer, and
// "unicode-bidi: plaintext" is the case: css-writing-modes hands the direction
// of a paragraph with no strong character to the paragraph before it, and to the
// containing block where there is none — which is what a plain text editor does,
// and is what the value is named after.
//
// Isolated runs are skipped exactly as P2 says: the contents of an isolate say
// nothing about the text around it.
func FirstStrong(text []rune) (Direction, bool) {
	classes := make([]Class, len(text))
	for i, r := range text {
		classes[i] = ClassOf(r)
	}
	pdi, _ := matchPDI(classes)
	for i := 0; i < len(classes); i++ {
		switch classes[i] {
		case L:
			return LeftToRight, true
		case R, AL:
			return RightToLeft, true
		case LRI, RLI, FSI:
			i = pdi[i]
		}
	}
	return LeftToRight, false
}
