package shape

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/mgilbir/forme/font"
)

// 'MVAR': how a variable font's font-wide numbers move across its design space.
//
// The ascent a line is given, the x-height an "ex" is measured by, where an
// underline and a strikeout are drawn: each is one number in hhea, OS/2 or post,
// and each is the default instance's. MVAR states, per number, a delta-set in an
// item variation store (varStore), named by a four-letter tag.
//
// An instance used to drop the table and keep the default's numbers, so a heavy
// instance of a face whose MVAR raises its x-height and strikeout — Noto Sans
// states exactly those two — measured "ex" and struck through its text at the
// Regular's heights.
//
// # Which fields a tag moves
//
// HarfBuzz's, and not fontTools'. The two agree about every field but three:
// 'hasc', 'hdsc' and 'hlgp' name the horizontal ascender, descender and line
// gap, and fontTools' instancer applies them to OS/2's typographic values, and
// to hhea's only where hhea stated the same three numbers to begin with; while
// HarfBuzz — hb-ot-metrics.cc, which is what a browser lays a line out by —
// applies them to whichever of hhea's and OS/2's it reads, which is hhea's
// unless the font sets USE_TYPO_METRICS. So for a face whose hhea and OS/2
// differ, an instance cut by fontTools and read by HarfBuzz has a different
// ascent from the variable font read at the same location by the same
// HarfBuzz. Moving both fields always is what makes the two the same: a reader
// of the static instance, whichever field it takes, gets the number HarfBuzz
// reports at the location. VariedLayout.ttf states different numbers in the two
// so that the difference shows.
//
// The value written is HarfBuzz's too: the stated number plus the delta,
// rounded as em_scalef_y rounds it at a scale of one unit per font unit, which
// is half up — HarfBuzz's roundf is floor(x + 0.5), not the C library's — and so
// otRound.
//
// The 'gsp0'..'gsp9' tags vary gasp's size thresholds, which are hinting, and
// hinting is dropped from an instance (see instance.go); they move nothing here.

// mvarField is one number a tag varies: the table it is in, where, and whether
// the format states it unsigned.
type mvarField struct {
	table    string
	off      int
	unsigned bool
	// os2v2 marks a field OS/2 carries only from version 2, which a shorter or
	// older table does not have and must not be written into.
	os2v2 bool
}

// mvarFields are the fields each tag moves. See the note above for 'hasc',
// 'hdsc' and 'hlgp'.
var mvarFields = map[string][]mvarField{
	"hasc": {{table: "hhea", off: 4}, {table: "OS/2", off: 68}},
	"hdsc": {{table: "hhea", off: 6}, {table: "OS/2", off: 70}},
	"hlgp": {{table: "hhea", off: 8}, {table: "OS/2", off: 72}},
	"hcla": {{table: "OS/2", off: 74, unsigned: true}},
	"hcld": {{table: "OS/2", off: 76, unsigned: true}},
	"hcrs": {{table: "hhea", off: 18}},
	"hcrn": {{table: "hhea", off: 20}},
	"hcof": {{table: "hhea", off: 22}},
	"vasc": {{table: "vhea", off: 4}},
	"vdsc": {{table: "vhea", off: 6}},
	"vlgp": {{table: "vhea", off: 8}},
	"vcrs": {{table: "vhea", off: 18}},
	"vcrn": {{table: "vhea", off: 20}},
	"vcof": {{table: "vhea", off: 22}},
	"xhgt": {{table: "OS/2", off: 86, os2v2: true}},
	"cpht": {{table: "OS/2", off: 88, os2v2: true}},
	"sbxs": {{table: "OS/2", off: 10}},
	"sbys": {{table: "OS/2", off: 12}},
	"sbxo": {{table: "OS/2", off: 14}},
	"sbyo": {{table: "OS/2", off: 16}},
	"spxs": {{table: "OS/2", off: 18}},
	"spys": {{table: "OS/2", off: 20}},
	"spxo": {{table: "OS/2", off: 22}},
	"spyo": {{table: "OS/2", off: 24}},
	"strs": {{table: "OS/2", off: 26}},
	"stro": {{table: "OS/2", off: 28}},
	"unds": {{table: "post", off: 10}},
	"undo": {{table: "post", off: 8}},
}

// applyMVAR moves the font-wide numbers of an instance's tables to the
// location, writing into copies of the tables it changes. A font with no MVAR,
// or one whose MVAR has no store, is left as it is.
//
// A table that cannot be read is an error, as HVAR's is: the instance would
// otherwise go out with the default instance's metrics and nothing to say so.
func applyMVAR(out map[string][]byte, mvar []byte, coords []float64) error {
	if len(mvar) == 0 {
		return nil
	}
	if len(mvar) < 12 {
		return errors.New("fonts: the MVAR table is too short to hold its header")
	}
	if v := font.Be16(mvar, 0); v != 1 {
		return fmt.Errorf("fonts: MVAR is version %d, which this does not read", v)
	}
	size := font.Be16(mvar, 6)
	count := font.Be16(mvar, 8)
	storeOff := font.Be16(mvar, 10)
	if count == 0 || storeOff == 0 {
		return nil
	}
	if size < 8 || 12+size*count > len(mvar) {
		return errors.New("fonts: MVAR's value records lie outside it")
	}
	if storeOff >= len(mvar) {
		return errors.New("fonts: MVAR's item variation store lies outside it")
	}
	store, err := parseVarStore(mvar[storeOff:])
	if err != nil {
		return fmt.Errorf("fonts: MVAR: %w", err)
	}
	copied, seen := map[string]bool{}, map[string]bool{}
	for i := 0; i < count; i++ {
		rec := 12 + size*i
		tag := string(mvar[rec : rec+4])
		fields := mvarFields[tag]
		// A tag is one number's delta, stated once: the records are sorted by
		// tag and a reader finds one by searching them. A table stating a tag
		// twice moves its field once, by the first.
		if len(fields) == 0 || seen[tag] {
			continue
		}
		seen[tag] = true
		delta := store.delta(font.Be16(mvar, rec+4), font.Be16(mvar, rec+6), coords)
		if delta == 0 {
			continue
		}
		for _, fl := range fields {
			t := out[fl.table]
			if fl.off+2 > len(t) {
				continue
			}
			if fl.os2v2 && (len(t) < 90 || font.Be16(t, 0) < 2) {
				continue
			}
			if !copied[fl.table] {
				t = append([]byte(nil), t...)
				out[fl.table], copied[fl.table] = t, true
			}
			var v float64
			if fl.unsigned {
				v = float64(font.Be16(t, fl.off))
			} else {
				v = float64(signed16(font.Be16(t, fl.off)))
			}
			n := otRound(v + delta)
			if fl.unsigned {
				binary.BigEndian.PutUint16(t[fl.off:], uint16(clampU16(n)))
			} else {
				binary.BigEndian.PutUint16(t[fl.off:], uint16(int16(clampI16(n))))
			}
		}
	}
	return nil
}
