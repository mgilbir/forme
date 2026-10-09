package shape

import (
	"unicode/utf16"

	"github.com/mgilbir/forme/font"
)

// NamedInstance finds one of the points a variable font names in its design
// space — fvar's instance records, each a location and the subfamily name its
// publisher gave it, "Bold" or "Condensed Light" — and returns where it is, in
// the user coordinates LoadInstance takes, by axis tag. It is what CSS Fonts 4
// §4.7's font-named-instance descriptor asks for by name.
//
// The instance is the first whose subfamily name match accepts in any of the
// spellings the name table gives it: a name is localized, one record per
// language, and §5.1's localized name matching compares against all of them.
// How two names compare is the caller's to say — CSS folds case one way and a
// PDF writer might want another — so it is asked rather than decided here.
//
// ok is false for a static face, for one LoadInstance cut (which no longer has
// a design space), and where no instance's name matches. A record whose name
// the name table does not hold cannot be asked for by name and is passed over.
//
// Nothing is kept for a record that does not match: fvar may list 65,535 of
// them, and a font is untrusted input.
func (f *Face) NamedInstance(match func(name string) bool) (coords map[string]float64, ok bool) {
	if len(f.axes) == 0 || match == nil {
		return nil, false
	}
	tables := f.sfntTables()
	fvar := tables["fvar"]
	axes, err := parseFvar(fvar)
	if err != nil {
		return nil, false
	}
	at := font.Be16(fvar, 4) + font.Be16(fvar, 8)*font.Be16(fvar, 10)
	count := font.Be16(fvar, 12)
	size := font.Be16(fvar, 14)
	// A record is a subfamily name ID, flags, and a coordinate per axis, and
	// may carry a PostScript name ID after them.
	if size < 4+4*len(axes) || at+size*count > len(fvar) {
		return nil, false
	}
	names := nameStrings(tables["name"])
	// Each name ID is judged once, however many records name it, so the work
	// is the name table's size plus fvar's and not their product: a hostile
	// font can point sixty thousand records at one name with sixty thousand
	// spellings.
	judged := map[int]bool{}
	for i := 0; i < count; i++ {
		rec := at + size*i
		id := font.Be16(fvar, rec)
		found, done := judged[id]
		if !done {
			for _, name := range names[id] {
				if match(name) {
					found = true
					break
				}
			}
			judged[id] = found
		}
		if !found {
			continue
		}
		coords = make(map[string]float64, len(axes))
		for j, a := range axes {
			coords[a.tag] = fixed1616At(fvar, rec+4+4*j)
		}
		return coords, true
	}
	return nil, false
}

// nameStrings is every string a name table states, by name ID: each record
// decoded, and each distinct spelling of an ID once, in record order.
//
// The Windows and Unicode platforms' records are UTF-16, decoded whole — a
// localized name is exactly the one that is not ASCII. A Macintosh record is a
// single-byte script encoding this does not carry the tables for, and is taken
// only where it is ASCII, which is what every font that also has Windows
// records writes there.
func nameStrings(name []byte) map[int][]string {
	if len(name) < 6 {
		return nil
	}
	count := font.Be16(name, 2)
	storage := font.Be16(name, 4)
	out := map[int][]string{}
	seen := map[int]map[string]bool{}
	left := nameReadAllowance(name)
	for i := 0; i < count; i++ {
		rec := 6 + 12*i
		if rec+12 > len(name) {
			break
		}
		id := font.Be16(name, rec+6)
		length := font.Be16(name, rec+8)
		off := storage + font.Be16(name, rec+10)
		if off+length > len(name) {
			continue
		}
		if length > left {
			break
		}
		left -= length
		raw := name[off : off+length]
		var s string
		switch font.Be16(name, rec) {
		case 0, 3:
			units := make([]uint16, len(raw)/2)
			for j := range units {
				units[j] = uint16(raw[2*j])<<8 | uint16(raw[2*j+1])
			}
			s = string(utf16.Decode(units))
		case 1:
			ascii := true
			for _, c := range raw {
				ascii = ascii && c < 0x80
			}
			if !ascii {
				continue
			}
			s = string(raw)
		default:
			continue
		}
		if s == "" {
			continue
		}
		if seen[id] == nil {
			seen[id] = map[string]bool{}
		}
		if !seen[id][s] {
			seen[id][s] = true
			out[id] = append(out[id], s)
		}
	}
	return out
}
