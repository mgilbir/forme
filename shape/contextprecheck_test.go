package shape

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// firstInputMisses is an optimisation and may change no answer, nor the work a
// run is charged, which RunResult reports and RunLimits bounds. These shape the
// same text with it and without, on fresh faces, and require both to be the
// same.

// shapeBounded is each text's RunResult, and its error as text.
func shapeBounded(t *testing.T, f *Face, texts []string) []any {
	t.Helper()
	out := make([]any, 0, 2*len(texts))
	for _, s := range texts {
		r, err := f.ShapeGlyphsBounded(context.Background(), RunInput{Text: s, Kerns: true}, RunLimits{})
		msg := ""
		if err != nil {
			msg = err.Error()
		}
		out = append(out, r, msg)
	}
	return out
}

func requireSameWithAndWithoutPrecheck(t *testing.T, name string, load func() *Face, texts []string) {
	t.Helper()
	firstInputPrecheckOff = true
	defer func() { firstInputPrecheckOff = false }()
	want := shapeBounded(t, load(), texts)
	firstInputPrecheckOff = false
	got := shapeBounded(t, load(), texts)
	for i := range got {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Fatalf("%s: %q\n  with the precheck    %+v\n  without it          %+v",
				name, texts[i/2], got[i], want[i])
		}
	}
}

// The HarfBuzz corpora's faces are where format 3 rules are tried most: Noto
// Sans Arabic, Khmer, Javanese, Balinese and the bundled Noto Sans each carry
// chained format 3 subtables, and the corpora were written to reach them.
func TestTheFirstInputPrecheckIsTheMatchersAnswerOverTheHarfBuzzCorpora(t *testing.T) {
	for _, tc := range harfbuzzCases {
		t.Run(tc.name, func(t *testing.T) {
			corpus, _, _ := readHarfBuzzGolden(t, tc.corpus, tc.expected)
			data := notoSansBytes(t)
			if tc.font != "" {
				var err error
				if data, err = os.ReadFile(filepath.Join(harfbuzzDir, tc.font)); err != nil {
					t.Fatal(err)
				}
			}
			load := func() *Face {
				f, err := Load(data)
				if err != nil {
					t.Fatal(err)
				}
				return f
			}
			requireSameWithAndWithoutPrecheck(t, tc.name, load, corpus)
		})
	}
}

// formatThreeFace is a face whose one GSUB lookup, under calt, is a format 3
// subtable of a kind — 5, sequence context, or 6, chained — with an input of
// count glyphs, each coverage naming only a; and a lookahead of one, b, for
// the chained one. It has no records, so it changes no glyph whether it
// matches or not; what matching changes is the work charged, and where the
// walk resumes.
func formatThreeFace(t *testing.T, kind, count int) *Face {
	t.Helper()
	coverageA := u16(nil, 1, 1, 1)
	coverageB := u16(nil, 1, 1, 2)
	var sub []byte
	switch kind {
	case 5:
		// format, glyph count, record count, the input's coverages.
		covAt := 6 + 2*count
		sub = u16(nil, 3, count, 0)
		for i := 0; i < count; i++ {
			sub = u16(sub, covAt)
		}
		sub = append(sub, coverageA...)
	case 6:
		// format, no backtrack, the input's count and coverages, a lookahead
		// of one coverage, and the record count.
		covAt := 6 + 2*count + 6
		sub = u16(nil, 3, 0, count)
		for i := 0; i < count; i++ {
			sub = u16(sub, covAt)
		}
		sub = u16(sub, 1, covAt+len(coverageA), 0)
		sub = append(sub, coverageA...)
		sub = append(sub, coverageB...)
	}
	return costFace(t, map[string][]byte{"GSUB": fonttest.GSUBLookups(
		[]fonttest.Lookup{{Type: kind, Subtables: [][]byte{sub}}},
		map[string][]int{"calt": {0}})})
}

// The precheck answers only where the matcher would have looked at one glyph.
// A rule whose input is longer than the matcher takes is refused there without
// a charge, and an input of one glyph that does start at a, or one that the
// lookahead then refuses, is the matcher's to answer; each is asked of both
// format 3 forms, at a glyph the rule starts at and at one it does not.
func TestTheFirstInputPrecheckIsTheMatchersAnswerAtItsEdges(t *testing.T) {
	texts := []string{"a", "b", "c", "ab", "ba", "aab", "abcabc", strings.Repeat("cab", 30)}
	for _, kind := range []int{5, 6} {
		for _, count := range []int{1, 2, maxContextLength, maxContextLength + 1, 0xFFFF} {
			name := map[int]string{5: "sequence", 6: "chained"}[kind]
			requireSameWithAndWithoutPrecheck(t, name, func() *Face { return formatThreeFace(t, kind, count) }, texts)
		}
	}
}
