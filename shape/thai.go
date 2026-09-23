package shape

// Thai and Lao.
//
// Neither script reorders, and neither needs a syllable model: its vowel signs
// and tone marks are written in the order they are drawn, stacked above and
// below the consonant, and a font's mark positioning places them. What both need
// is one rearrangement of the text that every engine makes and no font can make
// for itself.
//
// # SARA AM
//
// U+0E33 THAI CHARACTER SARA AM is one character and two shapes: the NIKHAHIT
// circle drawn above the consonant, and SARA AA after it. Written after a tone
// mark — น้ำ, "water", is U+0E19 U+0E49 U+0E33 — the circle belongs *under* the
// tone mark, closest to the letter, and the tone mark is raised over it. So the
// character is taken apart into NIKHAHIT and SARA AA, and the NIKHAHIT is moved
// back in front of any above-base marks written before it. That puts the two
// marks in the order they are stacked, which is the order a font's mark-to-mark
// positioning is written against: Noto Sans Thai then draws the small tone mark
// on the circle. Set without it, the full-size tone mark sits low and the circle
// floats above it — the stack upside down, in words as common as น้ำ, ค่ำ and ต่ำ.
//
// This is not in the OpenType Thai specification; it is what Uniscribe does and
// what HarfBuzz does after it (Eric Muller's rule: "decompose it in NIKHAHIT +
// SARA AA, and move the NIKHAHIT backwards over any above-base marks"). Lao's
// U+0EB3 is the same character eighty code points on, and gets the same.
//
// # Fonts with no Thai rules
//
// A Thai font from before OpenType states no GSUB for Thai and draws the stacked
// forms as glyphs at code points in Unicode's Private Use Area, which Windows and
// the Mac assigned differently. For such a font the marks are moved to those
// code points where the font has them: a tone mark over an above-base vowel is
// the raised form, one on a letter with a tall ascender the form shifted left,
// and so on. It is HarfBuzz's fallback, with its tables, and it applies only to
// a Thai font that declares no Thai rules of its own.

// thaiPreprocess takes each SARA AM of a Thai or Lao run apart and moves its
// NIKHAHIT back over the above-base marks written before it. See the file's
// comment.
//
// The offsets of what moves are merged — the pieces and the marks they were
// moved across become one cluster, since there is no longer a place between
// them a caret could sit, and the offsets have to stay in order. That is the
// rule normalize.go applies to a mark that moves past another. Where nothing
// moves, the two pieces keep the offset of the character they came from and
// nothing else is merged: a cluster here is a character, not a grapheme — see
// Glyph.Cluster — which is where this parts from HarfBuzz, whose default cluster
// level merges the pieces into the consonant's cluster as well.
func thaiPreprocess(runes []rune, offsets []int) ([]rune, []int) {
	first := -1
	for i, r := range runes {
		if isSaraAm(r) {
			first = i
			break
		}
	}
	if first < 0 {
		return runes, offsets
	}
	outR := make([]rune, 0, len(runes)+4)
	outO := make([]int, 0, len(offsets)+4)
	outR = append(outR, runes[:first]...)
	outO = append(outO, offsets[:first]...)
	for i := first; i < len(runes); i++ {
		r := runes[i]
		if !isSaraAm(r) {
			outR = append(outR, r)
			outO = append(outO, offsets[i])
			continue
		}
		// NIKHAHIT and SARA AA, each at the offset of the character they came
		// out of.
		nikhahit, saraAa := r-0x0E33+0x0E4D, r-1
		outR = append(outR, nikhahit, saraAa)
		outO = append(outO, offsets[i], offsets[i])
		end := len(outR)
		start := end - 2
		for start > 0 && isThaiAboveBaseMark(outR[start-1]) {
			start--
		}
		if start+2 < end {
			// Move the NIKHAHIT (end-2) to the front of the marks it follows,
			// and make one cluster of what it crossed.
			mergeOffsets(outO, start, end)
			copy(outR[start+1:end-1], outR[start:end-2])
			outR[start] = nikhahit
		}
	}
	return outR, outO
}

// mergeOffsets makes one cluster of offsets[lo:hi]: each takes the earliest of
// them, which is where the indivisible stretch of text begins.
func mergeOffsets(offsets []int, lo, hi int) {
	least := offsets[lo]
	for _, o := range offsets[lo:hi] {
		least = min(least, o)
	}
	for i := lo; i < hi; i++ {
		offsets[i] = least
	}
}

// isSaraAm reports whether a character is Thai or Lao SARA AM.
func isSaraAm(r rune) bool { return r&^0x80 == 0x0E33 }

// isThaiAboveBaseMark reports whether a character is a Thai or Lao mark drawn
// above the consonant: the vowel signs above, the tone marks and the other
// signs stacked there. The Lao ones are the Thai ones eighty code points on.
func isThaiAboveBaseMark(r rune) bool {
	u := r &^ 0x80
	if u < 0x0E00 || u > 0x0E7F || r < 0x0E00 || r > 0x0EFF {
		return false
	}
	return u >= 0x0E34 && u <= 0x0E37 || u >= 0x0E47 && u <= 0x0E4E || u == 0x0E31 || u == 0x0E3B
}

// thaiPUAShape moves the marks of a Thai run to the Private Use Area forms an
// older font draws its stacked marks as, where the font has them. It is
// HarfBuzz's do_thai_pua_shaping, state machines and tables alike: the state
// is what is stacked above and below the current consonant so far, and each
// mark's move to a shifted form is decided by it.
func (f *Face) thaiPUAShape(runes []rune) []rune {
	above := thaiAboveStart[thaiNotConsonant]
	below := thaiBelowStart[thaiNotConsonant]
	base := 0
	var out []rune
	set := func(i int, r rune) {
		if r == runes[i] && out == nil {
			return
		}
		if out == nil {
			out = append([]rune(nil), runes...)
		}
		out[i] = r
	}
	for i, r := range runes {
		mt := thaiMarkType(r)
		if mt == thaiNotMark {
			ct := thaiConsonantType(r)
			above, below = thaiAboveStart[ct], thaiBelowStart[ct]
			base = i
			continue
		}
		ae := thaiAboveMachine[above][mt]
		be := thaiBelowMachine[below][mt]
		above, below = ae.next, be.next
		action := ae.action
		if action == thaiNOP {
			action = be.action
		}
		if action == thaiRD {
			set(base, f.thaiPUA(runes[base], action))
		} else {
			set(i, f.thaiPUA(r, action))
		}
	}
	if out == nil {
		return runes
	}
	return out
}

// thaiPUA is the Private Use Area form of a character for an action, where the
// face has one: the Windows code point first and the Mac one second, as
// HarfBuzz tries them.
func (f *Face) thaiPUA(r rune, action thaiAction) rune {
	var table []thaiPUAMapping
	switch action {
	case thaiSD:
		table = thaiSDMappings
	case thaiSDL:
		table = thaiSDLMappings
	case thaiSL:
		table = thaiSLMappings
	case thaiRD:
		table = thaiRDMappings
	default:
		return r
	}
	for _, m := range table {
		if m.u != r {
			continue
		}
		if f.hasGlyph(m.win) {
			return m.win
		}
		if f.hasGlyph(m.mac) {
			return m.mac
		}
		break
	}
	return r
}

type thaiConsonant uint8

const (
	thaiNC thaiConsonant = iota
	thaiAC
	thaiRC
	thaiDC
	thaiNotConsonant
)

func thaiConsonantType(r rune) thaiConsonant {
	switch {
	case r == 0x0E1B || r == 0x0E1D || r == 0x0E1F:
		return thaiAC
	case r == 0x0E0D || r == 0x0E10:
		return thaiRC
	case r == 0x0E0E || r == 0x0E0F:
		return thaiDC
	case r >= 0x0E01 && r <= 0x0E2E:
		return thaiNC
	}
	return thaiNotConsonant
}

type thaiMark uint8

const (
	thaiAV thaiMark = iota
	thaiBV
	thaiT
	thaiNotMark
)

func thaiMarkType(r rune) thaiMark {
	switch {
	case r == 0x0E31 || r >= 0x0E34 && r <= 0x0E37 || r == 0x0E47 || r >= 0x0E4D && r <= 0x0E4E:
		return thaiAV
	case r >= 0x0E38 && r <= 0x0E3A:
		return thaiBV
	case r >= 0x0E48 && r <= 0x0E4C:
		return thaiT
	}
	return thaiNotMark
}

type thaiAction uint8

const (
	thaiNOP thaiAction = iota
	thaiSD             // shift the mark down
	thaiSL             // shift it left
	thaiSDL            // shift it down and left
	thaiRD             // remove the base's descender
)

type thaiEdge struct {
	action thaiAction
	next   uint8
}

// The above-base states: T0 nothing stacked, T1 a consonant with a tall
// ascender, T2 that consonant with a mark shifted left, T3 full.
var thaiAboveStart = [...]uint8{thaiNC: 0, thaiAC: 1, thaiRC: 0, thaiDC: 0, thaiNotConsonant: 3}

var thaiAboveMachine = [4][3]thaiEdge{
	//         AV            BV            T
	/*T0*/ {{thaiNOP, 3}, {thaiNOP, 0}, {thaiSD, 3}},
	/*T1*/ {{thaiSL, 2}, {thaiNOP, 1}, {thaiSDL, 2}},
	/*T2*/ {{thaiNOP, 3}, {thaiNOP, 2}, {thaiSL, 3}},
	/*T3*/ {{thaiNOP, 3}, {thaiNOP, 3}, {thaiNOP, 3}},
}

// The below-base states: B0 no descender, B1 a descender that can be removed,
// B2 one that cannot.
var thaiBelowStart = [...]uint8{thaiNC: 0, thaiAC: 0, thaiRC: 1, thaiDC: 2, thaiNotConsonant: 2}

var thaiBelowMachine = [3][3]thaiEdge{
	//         AV            BV            T
	/*B0*/ {{thaiNOP, 0}, {thaiNOP, 2}, {thaiNOP, 0}},
	/*B1*/ {{thaiNOP, 1}, {thaiRD, 2}, {thaiNOP, 1}},
	/*B2*/ {{thaiNOP, 2}, {thaiSD, 2}, {thaiNOP, 2}},
}

type thaiPUAMapping struct{ u, win, mac rune }

var (
	thaiSDMappings = []thaiPUAMapping{
		{0x0E48, 0xF70A, 0xF88B}, // MAI EK
		{0x0E49, 0xF70B, 0xF88E}, // MAI THO
		{0x0E4A, 0xF70C, 0xF891}, // MAI TRI
		{0x0E4B, 0xF70D, 0xF894}, // MAI CHATTAWA
		{0x0E4C, 0xF70E, 0xF897}, // THANTHAKHAT
		{0x0E38, 0xF718, 0xF89B}, // SARA U
		{0x0E39, 0xF719, 0xF89C}, // SARA UU
		{0x0E3A, 0xF71A, 0xF89D}, // PHINTHU
	}
	thaiSDLMappings = []thaiPUAMapping{
		{0x0E48, 0xF705, 0xF88C}, // MAI EK
		{0x0E49, 0xF706, 0xF88F}, // MAI THO
		{0x0E4A, 0xF707, 0xF892}, // MAI TRI
		{0x0E4B, 0xF708, 0xF895}, // MAI CHATTAWA
		{0x0E4C, 0xF709, 0xF898}, // THANTHAKHAT
	}
	thaiSLMappings = []thaiPUAMapping{
		{0x0E48, 0xF713, 0xF88A}, // MAI EK
		{0x0E49, 0xF714, 0xF88D}, // MAI THO
		{0x0E4A, 0xF715, 0xF890}, // MAI TRI
		{0x0E4B, 0xF716, 0xF893}, // MAI CHATTAWA
		{0x0E4C, 0xF717, 0xF896}, // THANTHAKHAT
		{0x0E31, 0xF710, 0xF884}, // MAI HAN-AKAT
		{0x0E34, 0xF701, 0xF885}, // SARA I
		{0x0E35, 0xF702, 0xF886}, // SARA II
		{0x0E36, 0xF703, 0xF887}, // SARA UE
		{0x0E37, 0xF704, 0xF888}, // SARA UEE
		{0x0E47, 0xF712, 0xF889}, // MAITAIKHU
		{0x0E4D, 0xF711, 0xF899}, // NIKHAHIT
	}
	thaiRDMappings = []thaiPUAMapping{
		{0x0E0D, 0xF70F, 0xF89A}, // YO YING
		{0x0E10, 0xF700, 0xF89E}, // THO THAN
	}
)
