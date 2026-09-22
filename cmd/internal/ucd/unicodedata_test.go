package ucd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeData(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "UnicodeData.txt")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestUnicodeDataReadsWhatTheFileSays: the three simple mappings, the
// titlecase field's empty-means-uppercase rule, and a First/Last pair read as
// every code point between them.
func TestUnicodeDataReadsWhatTheFileSays(t *testing.T) {
	path := writeData(t,
		"0041;LATIN CAPITAL LETTER A;Lu;0;L;;;;;N;;;;0061;",
		"0061;LATIN SMALL LETTER A;Ll;0;L;;;;;N;;;0041;;0041",
		"01C5;LATIN CAPITAL LETTER D WITH SMALL LETTER Z WITH CARON;Lt;0;L;<compat> 0044 017E;;;;N;;;01C4;01C6;01C5",
		"00E9;LATIN SMALL LETTER E WITH ACUTE;Ll;0;L;0065 0301;;;;N;;;00C9;;",
		"0897;ARABIC PEPET;Mn;230;NSM;;;;;N;;;;;",
		"3400;<CJK Ideograph Extension A, First>;Lo;0;L;;;;;N;;;;;",
		"4DBF;<CJK Ideograph Extension A, Last>;Lo;0;L;;;;;N;;;;;",
	)
	got, err := UnicodeData(path)
	if err != nil {
		t.Fatal(err)
	}
	for r, want := range map[rune]Char{
		'A':    {Category: "Lu", Lower: 'a'},
		'a':    {Category: "Ll", Upper: 'A', Title: 'A'},
		0x01C5: {Category: "Lt", Upper: 0x01C4, Lower: 0x01C6, Title: 0x01C5},
		// No titlecase field, so it is the uppercase mapping.
		0x00E9: {Category: "Ll", Upper: 0x00C9, Title: 0x00C9},
		0x0897: {Category: "Mn"},
		0x3400: {Category: "Lo"},
		0x3A00: {Category: "Lo"},
		0x4DBF: {Category: "Lo"},
	} {
		if got[r] != want {
			t.Errorf("U+%04X is %+v, want %+v", r, got[r], want)
		}
	}
	if _, ok := got[0x4DC0]; ok {
		t.Error("the range ran past its Last line")
	}
	if len(got) != 5+0x4DBF-0x3400+1 {
		t.Errorf("%d characters read", len(got))
	}
}

// TestUnicodeDataRefusesWhatItCannotRead: a file that is not the database, or
// is only part of it, is an error rather than a smaller table.
func TestUnicodeDataRefusesWhatItCannotRead(t *testing.T) {
	for name, lines := range map[string][]string{
		"too few fields":   {"0041;LATIN CAPITAL LETTER A;Lu;0;L"},
		"not hexadecimal":  {"00G1;X;Lu;0;L;;;;;N;;;;;"},
		"past the space":   {"110000;X;Lu;0;L;;;;;N;;;;;"},
		"an open range":    {"3400;<CJK, First>;Lo;0;L;;;;;N;;;;;"},
		"a range unopened": {"4DBF;<CJK, Last>;Lo;0;L;;;;;N;;;;;"},
		"a range that changes category": {
			"3400;<CJK, First>;Lo;0;L;;;;;N;;;;;",
			"4DBF;<CJK, Last>;Lu;0;L;;;;;N;;;;;",
		},
		"empty": {""},
	} {
		if _, err := UnicodeData(writeData(t, lines...)); err == nil {
			t.Errorf("%s: read without an error", name)
		}
	}
}
