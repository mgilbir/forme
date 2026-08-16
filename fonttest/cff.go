package fonttest

// A CFF table built from nothing, for the paths that only exist because the
// outlines are CFF rather than glyf.
//
// OTTO wraps a CFF table in an OpenType container but had no table to wrap: a
// CFF means INDEX structures, DICTs of operands-before-operator and Type 2
// charstrings, so the tests that needed one reached for a fetched face and
// skipped when it was absent. What cannot be reached that way is a font that is
// deliberately wrong — one numbered in a collection it does not name — and that
// is the case a reader has to get right.
//
// The charstrings are bare endchars. Nothing here is for rasterising; it is for
// the questions asked of a program before a glyph is ever drawn.

// CFFOptions configures a synthetic CFF. The zero value is a one-glyph font
// that is not CID-keyed.
type CFFOptions struct {
	// Glyphs is the number of charstrings including .notdef; 1 if zero. The
	// INDEX offsets written here are one byte, so this is bounded — see CFF.
	Glyphs int
	// CIDKeyed emits a ROS operator, which is the only thing that makes a CFF
	// CID-keyed: the charset is then read as glyph index to CID rather than to
	// a glyph name.
	CIDKeyed bool
	// Charset is the charset operator's operand: 0 ISOAdobe, 1 Expert, 2 Expert
	// Subset, or an offset to a charset the font carries. Zero is both the
	// default and ISOAdobe, which is what a font omitting the operator means, so
	// this emits the operator only when it is not zero — a fixture asking for
	// ISOAdobe gets the same bytes it always did.
	Charset int
	// The collection to state. Registry and Ordering go into the font's own
	// string INDEX and the ROS names them by SID, the way a collection Adobe
	// never published is carried. "Adobe" and "Identity" if empty.
	Registry, Ordering string
	Supplement         int
	// UnnamedCollection makes the ROS name SIDs past the end of the string
	// INDEX: a font that is CID-keyed and has not said which numbering its CIDs
	// are in. It parses, which is what makes it worth building — half a ROS is
	// the shape that reaches a document unnoticed.
	UnnamedCollection bool
	// NegativeSupplement states a supplement below zero. A supplement is a
	// version of the collection and counts up, so this is malformed; it is here
	// because it is malformed in a way that parses.
	NegativeSupplement bool
}

// CFF builds a CFF table: the header, the four INDEXes, a Private DICT and one
// charstring per glyph.
//
// It panics rather than returning an error, because it is a fixture and the
// only way to reach either panic is to ask for a font this cannot express. The
// glyph bound is the INDEX offset size: this writes one-byte offsets, so the
// charstrings have to fit in 255 bytes, which at one byte each they do until
// there are 255 of them. A test needing more glyphs than that needs a real
// font, not a fixture.
func CFF(opts CFFOptions) []byte {
	if opts.Glyphs == 0 {
		opts.Glyphs = 1
	}
	if opts.Glyphs < 1 || opts.Glyphs > 250 {
		panic("fonttest: a synthetic CFF holds 1 to 250 glyphs")
	}
	if opts.Registry == "" {
		opts.Registry = "Adobe"
	}
	if opts.Ordering == "" {
		opts.Ordering = "Identity"
	}

	// The strings the font carries. The first SID past the 391 standard ones is
	// 391, so the two named here are 391 and 392.
	const firstCustomSID = 391
	var strs [][]byte
	regSID, ordSID := 0, 0
	if opts.CIDKeyed {
		strs = [][]byte{[]byte(opts.Registry), []byte(opts.Ordering)}
		regSID, ordSID = firstCustomSID, firstCustomSID+1
		if opts.UnnamedCollection {
			// Past the end of the INDEX this font carries, so both resolve to
			// nothing while remaining perfectly well-formed operands.
			regSID, ordSID = firstCustomSID+len(strs), firstCustomSID+len(strs)+1
		}
	}
	supplement := opts.Supplement
	if opts.NegativeSupplement {
		supplement = -91
	}

	// Every operand below is written in the 28 form, three bytes whatever its
	// value, so the Top DICT written against placeholder offsets is the same
	// length as the one written against the real ones and nothing shifts under
	// it. The fixture checks that rather than trusting it.
	priv := cffPrivateDict()
	top := func(csOff, privOff int) []byte {
		var d []byte
		if opts.CIDKeyed {
			d = append(d, cffOperand3(regSID)...)
			d = append(d, cffOperand3(ordSID)...)
			d = append(d, cffOperand3(supplement)...)
			d = append(d, 12, 30) // ROS
		}
		if opts.Charset != 0 {
			d = append(d, cffOperand3(opts.Charset)...)
			d = append(d, 15) // charset
		}
		d = append(d, cffOperand3(csOff)...)
		d = append(d, 17) // CharStrings
		d = append(d, cffOperand3(len(priv))...)
		d = append(d, cffOperand3(privOff)...)
		d = append(d, 18) // Private: size then offset
		return d
	}

	var data []byte
	data = append(data, 1, 0, 4, 1) // version 1.0, hdrSize 4, offSize 1
	data = append(data, cffINDEX([]byte("Fixture"))...)
	topAt := len(data)
	data = append(data, cffINDEX(top(0, 0))...)
	topLen := len(data) - topAt
	data = append(data, cffINDEX(strs...)...)
	data = append(data, cffINDEX()...) // Global Subr INDEX, empty
	privOff := len(data)
	data = append(data, priv...)
	csOff := len(data)

	const endchar = 14
	charstrings := make([][]byte, opts.Glyphs)
	for i := range charstrings {
		charstrings[i] = []byte{endchar}
	}
	data = append(data, cffINDEX(charstrings...)...)

	final := cffINDEX(top(csOff, privOff))
	if len(final) != topLen {
		panic("fonttest: the Top DICT changed length between passes, so every " +
			"offset in it now points somewhere else")
	}
	copy(data[topAt:], final)
	return data
}

// cffPrivateDict states defaultWidthX and nominalWidthX, which is what a
// charstring carrying no width of its own is read against.
func cffPrivateDict() []byte {
	var b []byte
	b = append(b, cffOperand3(500)...)
	b = append(b, 20) // defaultWidthX
	b = append(b, cffOperand3(0)...)
	b = append(b, 21) // nominalWidthX
	return b
}

// cffOperand3 encodes an integer in the 28 form: two bytes of payload whatever
// the value, so that operands keep their width when they are rewritten.
func cffOperand3(v int) []byte {
	return []byte{28, byte(v >> 8), byte(v)}
}

// cffINDEX writes a CFF INDEX with one-byte offsets. An empty one is the two
// count bytes alone, which is what the format says and not an omission.
func cffINDEX(items ...[]byte) []byte {
	if len(items) == 0 {
		return []byte{0, 0}
	}
	total := 0
	for _, it := range items {
		total += len(it)
	}
	if total+1 > 0xFF {
		panic("fonttest: a synthetic CFF INDEX holds 255 bytes")
	}
	out := []byte{byte(len(items) >> 8), byte(len(items)), 1}
	off := 1
	out = append(out, byte(off))
	for _, it := range items {
		off += len(it)
		out = append(out, byte(off))
	}
	for _, it := range items {
		out = append(out, it...)
	}
	return out
}
