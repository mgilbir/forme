package shape

import (
	"strings"
	"unicode/utf16"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/internal/ascii"
)

// Family is the name of the family the face belongs to, as the font states it:
// the typographic family (name ID 16) where there is one, and the legacy family
// (ID 1) where there is not, or where the typographic one cannot be read. It
// is the string a CSS font-family is matched against, "Noto Sans" rather than
// the "Noto Sans Bold" a four-style legacy family would be.
//
// It is the empty string for a face with no name table or no readable record
// for either ID, and for the fourteen standard faces, which have no program to
// read it from; an empty family is the answer and not an error. A face from
// LoadInstance reports what the name table it was cut with says, which
// rewrites the style names and leaves the family alone.
//
// Which record is read is the question readName answers.
func (f *Face) Family() string { return f.family }

// Subfamily is the style the face names itself: the typographic subfamily (name
// ID 17), falling back to the legacy one (ID 2), "Bold Italic" or "Condensed
// Light". It is empty in every case Family is.
//
// It is a label the publisher chose and not a measurement: Descriptor().Weight,
// WidthClass and Italic are what the font states as numbers, and are what to
// match against.
func (f *Face) Subfamily() string { return f.subfamily }

// readName reads a name ID as text, taking the record a reader wanting the
// English name would, and returns "" where none can be read.
//
// PostScript names (see nameByID) are ASCII by construction and are cut down
// to the characters a PDF name may hold. A family is not: it is shown and
// matched as written, so it is decoded whole, surrogate pairs included, and
// only stripped of the control characters a font file has no business putting
// in one.
//
// Of the records that state the ID, in order of preference:
//
//  1. Windows, Unicode encoding (platform 3, encoding 1 or 10), English.
//  2. Macintosh, Roman (platform 1, encoding 0), English.
//  3. The Unicode platform (0), in any language.
//  4. Windows, Unicode encoding, in any language.
//  5. Macintosh Roman in any language.
//
// The Macintosh records are read only where they are ASCII: decoding the rest
// takes MacRoman's upper half, which is worth a table only for a name no
// Windows record also states, and nearly every font that has a Mac record has
// the Windows one as well. A record that is not, is passed over rather than
// misread.
//
// The first record of the best rank wins, so the answer does not depend on how
// many others there are, and a name table is walked once. A record is ranked
// before it is decoded, and one that cannot win is not decoded at all; those
// that are decoded are charged against nameReadAllowance, since a record whose
// string reads as nothing leaves the rank open and every record can be one.
func readName(name []byte, id int) string {
	if len(name) < 6 {
		return ""
	}
	count := font.Be16(name, 2)
	storage := font.Be16(name, 4)
	best, bestRank := "", 6
	left := nameReadAllowance(name)
	for i := 0; i < count; i++ {
		rec := 6 + 12*i
		if rec+12 > len(name) {
			break
		}
		if font.Be16(name, rec+6) != id {
			continue
		}
		platform, encoding, language := font.Be16(name, rec), font.Be16(name, rec+2), font.Be16(name, rec+4)
		length := font.Be16(name, rec+8)
		off := storage + font.Be16(name, rec+10)
		if off+length > len(name) {
			continue
		}
		english := language&0x3ff == 0x09 // Windows language IDs; the primary language is the low ten bits
		rank := 0
		decode := decodeUTF16
		switch {
		case platform == 3 && (encoding == 1 || encoding == 10):
			if english {
				rank = 1
			} else {
				rank = 4
			}
		case platform == 1 && encoding == 0:
			if language == 0 { // Macintosh language 0 is English
				rank = 2
			} else {
				rank = 5
			}
			decode = decodeASCII
		case platform == 0:
			rank = 3
		default:
			continue
		}
		if rank >= bestRank {
			continue
		}
		if length > left {
			break
		}
		left -= length
		text := decode(name[off : off+length])
		if text == "" {
			continue
		}
		best, bestRank = text, rank
	}
	return best
}

// nameWithFallback reads id, and the older id where the font states nothing
// readable for it.
func nameWithFallback(name []byte, id, legacy int) string {
	if s := readName(name, id); s != "" {
		return s
	}
	return readName(name, legacy)
}

func decodeUTF16(raw []byte) string {
	units := make([]uint16, len(raw)/2)
	for i := range units {
		units[i] = uint16(raw[2*i])<<8 | uint16(raw[2*i+1])
	}
	return cleanName(string(utf16.Decode(units)))
}

func decodeASCII(raw []byte) string {
	for _, c := range raw {
		if c >= 0x80 {
			return ""
		}
	}
	return cleanName(string(raw))
}

// cleanName drops the control characters, C0 and C1, which include the NULs
// some fonts pad a record with, and the ASCII space around what is left. The
// ranges are written out because they are fixed by ISO 6429 and not by the
// toolchain's Unicode release, and the trim is ASCII's for the same reason: a
// name is shown as the font wrote it, and only padding is removed.
func cleanName(s string) string {
	return ascii.TrimSpace(strings.Map(func(r rune) rune {
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
			return -1
		}
		return r
	}, s))
}

// readStyle takes what OS/2 and head say about the face's style: the width
// class, and whether it is italic or oblique.
//
// fsSelection (OS/2, offset 62) has a bit for each — 0 is italic and 9 is
// oblique — and is present in every version of the table. head's macStyle
// (offset 44) has a bit 1 for italic and none for oblique. macStyle is what a
// font with no OS/2 table has to say, so it is read only then: where OS/2
// exists it is the newer statement and the one that wins, even when it says
// no. usWidthClass (offset 6) is read on the same terms as usWeightClass.
//
// Both statements also have a bold bit — 5 in fsSelection, 0 in macStyle —
// and the one the style came from gives the weight where OS/2 did not state
// usWeightClass: 700 for bold, 400 for anything else, which is what the two
// bits distinguish and all they do (issue #871). Without it a bold face with
// no OS/2 table read as having no weight at all, while its italic bit came
// through from the same field.
func (f *Face) readStyle(os2, head []byte) {
	if len(os2) >= 78 {
		f.widthClass = font.Be16(os2, 6)
		f.declared |= MetricWidth
	}
	var bold, stated bool
	switch {
	case len(os2) >= 64:
		sel := font.Be16(os2, 62)
		f.styleItalic, f.styleOblique = sel&(1<<0) != 0, sel&(1<<9) != 0
		bold, stated = sel&(1<<5) != 0, true
		f.declared |= MetricStyle
	case len(head) >= 46:
		mac := font.Be16(head, 44)
		f.styleItalic = mac&(1<<1) != 0
		bold, stated = mac&(1<<0) != 0, true
		f.declared |= MetricStyle
	}
	if stated && f.declared&MetricWeight == 0 {
		f.weight = 400
		if bold {
			f.weight = 700
		}
		f.declared |= MetricWeight
	}
}
