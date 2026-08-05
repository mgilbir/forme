package forme

import (
	"errors"
	"fmt"
)

// Embedding a font program as a simple font: one byte per character, through a
// standard encoding, rather than as a composite font keyed by glyph index.
//
// # Why both forms exist
//
// A composite font is the general answer — any character, any script, as many
// glyphs as the face has. A simple font is the narrow one: 256 codes, drawn
// from an encoding of Latin characters. Where the narrow one fits it is
// smaller in the file and simpler in the stream, because the codes *are* the
// text: a byte of a content stream set in a simple font is a character, which
// is why such a document is searchable by readers that do not consult a
// ToUnicode CMap at all.
//
// Where it does not fit, it does not fit at all. A document with a Greek word,
// a Chinese name or an em dash outside WinAnsiEncoding cannot be set this way,
// and Encode reports how many characters fell outside rather than quietly
// substituting something.
//
// The choice is the caller's and is made once, at load: LoadSimple for this
// form, Load for the composite one. It cannot be changed afterwards because
// Encode's output means different things in the two, and a face that had
// produced codes of one kind cannot honestly produce the other.

// LoadSimple parses an sfnt font program and prepares it to be embedded as a
// simple font with WinAnsiEncoding.
//
// It refuses a program that cannot serve as one: a face is only usable here if
// its character map covers the encoding, and one that covers almost none of it
// would produce a document of blanks.
func LoadSimple(data []byte) (*Face, error) {
	f, err := Load(data)
	if err != nil {
		return nil, err
	}
	if f.cff {
		// A CFF program embeds as FontFile3, and a simple font with CFF
		// outlines is a Type1C font whose encoding lives inside the program.
		// Refusing is better than writing a TrueType font dictionary around it.
		return nil, errors.New("fonts: CFF programs are not embedded as simple fonts; use Load")
	}
	covered := 0
	for r := range winAnsi {
		if _, ok := f.prog.Cmap[r]; ok {
			covered++
		}
	}
	if covered < 32 {
		return nil, fmt.Errorf("fonts: the font maps only %d of the %d characters WinAnsiEncoding names; it cannot be embedded as a simple font",
			covered, len(winAnsi))
	}
	f.simple = true
	return f, nil
}

// IsSimple reports whether the face will be embedded as a simple font — one
// byte per character — rather than as a composite one.
func (f *Face) IsSimple() bool { return f.simple }

// encodeSimple maps a string to single-byte WinAnsi codes, recording the glyphs
// those codes will draw so the subsetter keeps them.
func (f *Face) encodeSimple(s string) (codes []byte, missing int) {
	codes = make([]byte, 0, len(s))
	for _, r := range s {
		if hiddenBeforeShaping(r) {
			// WinAnsi gives the soft hyphen a code of its own, so without this
			// a word marked as breakable reaches the page with a hyphen through
			// it. See ignorable.go.
			continue
		}
		code, _, ok := stdCode(r)
		if !ok {
			missing++
			continue // outside the encoding: there is no byte that means it
		}
		gid, mapped := f.prog.Cmap[r]
		if !mapped || gid == 0 {
			missing++
			continue // the encoding has a code but this face has no glyph
		}
		f.used[gid] = true
		codes = append(codes, code)
	}
	return codes, missing
}
