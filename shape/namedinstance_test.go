package shape

import (
	"encoding/binary"
	"fmt"
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// TestNamedInstanceIsFoundByAnyOfItsNames: VariedLayout.ttf names the
// instance at weight 700 "Bold", "Gras" and "太字" — the last two as Windows
// records in French and Japanese, and the Japanese one also as a Macintosh
// record in a Japanese encoding, which is not read. Each Windows spelling finds
// it, the caller's comparison decides what "the same name" means, and a name
// the font does not give finds nothing.
func TestNamedInstanceIsFoundByAnyOfItsNames(t *testing.T) {
	f, err := Load(harfbuzzFont(t, "VariedLayout.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		fold bool
		want float64
		ok   bool
	}{
		{"Bold", false, 700, true},
		{"Gras", false, 700, true},
		{"太字", false, 700, true},
		{"Book", false, 250, true},
		{"bold", false, 0, false},
		{"bold", true, 700, true},
		{"GRAS", true, 700, true},
		{"Regular", true, 0, false},
		{"", false, 0, false},
	} {
		match := func(s string) bool { return s == c.name }
		if c.fold {
			match = func(s string) bool { return strings.EqualFold(s, c.name) }
		}
		coords, ok := f.NamedInstance(match)
		if ok != c.ok || (ok && coords["wght"] != c.want) {
			t.Errorf("%q (folded %v): %v %v, want wght %v %v", c.name, c.fold, coords, ok, c.want, c.ok)
		}
		if ok && len(coords) != 1 {
			t.Errorf("%q: %d coordinates for a one-axis face", c.name, len(coords))
		}
	}
}

// TestNamedInstanceOfNotoSans is the bundled face's nine, by the names its
// publisher gave them, each a location LoadInstance accepts; and nothing for a
// face with no design space, which is what an instance is.
func TestNamedInstanceOfNotoSans(t *testing.T) {
	f, err := Load(notoSansBytes(t))
	if err != nil {
		t.Fatal(err)
	}
	coords, ok := f.NamedInstance(func(s string) bool { return s == "SemiBold" })
	if !ok || coords["wght"] != 600 || coords["wdth"] != 100 {
		t.Fatalf("SemiBold is %v %v, want wght 600 wdth 100", coords, ok)
	}
	inst, err := LoadInstance(notoSansBytes(t), coords)
	if err != nil {
		t.Fatal(err)
	}
	if got := inst.Name(); got != "NotoSans-SemiBold" {
		t.Errorf("the instance is named %q, want the publisher's NotoSans-SemiBold", got)
	}
	if _, ok := inst.NamedInstance(func(string) bool { return true }); ok {
		t.Error("an instance names instances of a design space it no longer has")
	}
}

// TestNamedInstanceJudgesEachNameOnce is the cost of the lookup on a hostile
// font: every one of n instance records names one name ID, which the name table
// spells n different ways. Judging the spellings per record asks the caller's
// comparison n² times, which at the format's 65,535 is four billion; judging
// them per name ID asks it n times, whatever the records do.
func TestNamedInstanceJudgesEachNameOnce(t *testing.T) {
	calls := func(n int) int {
		instances := make([]fonttest.VarInstance, n)
		for i := range instances {
			instances[i] = fonttest.VarInstance{SubfamilyNameID: 256, Coords: []float64{400, 100}}
		}
		// A name table of n Windows records for ID 256, each spelled apart.
		name := make([]byte, 6+12*n)
		binary.BigEndian.PutUint16(name[2:], uint16(n))
		binary.BigEndian.PutUint16(name[4:], uint16(6+12*n))
		var strs []byte
		for i := 0; i < n; i++ {
			rec := 6 + 12*i
			s := fmt.Sprintf("N%05d", i)
			binary.BigEndian.PutUint16(name[rec:], 3)
			binary.BigEndian.PutUint16(name[rec+2:], 1)
			binary.BigEndian.PutUint16(name[rec+4:], 0x409)
			binary.BigEndian.PutUint16(name[rec+6:], 256)
			binary.BigEndian.PutUint16(name[rec+8:], uint16(2*len(s)))
			binary.BigEndian.PutUint16(name[rec+10:], uint16(len(strs)))
			for _, c := range s {
				strs = append(strs, 0, byte(c))
			}
		}
		data := varFont{
			axes: wghtWdth, instances: instances,
			glyphs: [][]byte{nil, rectGlyph()}, advances: []int{0, 500},
			extra: map[string][]byte{"name": append(name, strs...)},
		}.build(t)
		f, err := Load(data)
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		if _, ok := f.NamedInstance(func(string) bool { count++; return false }); ok {
			t.Fatal("no instance's name matches, and one was found")
		}
		return count
	}
	for _, n := range []int{50, 200} {
		if got := calls(n); got != n {
			t.Errorf("%d instances naming one ID spelled %d ways: the comparison was asked %d times, want %d",
				n, n, got, n)
		}
	}
}
