package brotli

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

// The three tables RFC 7932 states rather than derives: the static dictionary,
// the transforms that reach beyond it, and the context models.
//
// None of them can be computed, so all three are copied — the dictionary as
// bytes, the other two by cmd/genbrotli reading the reference implementation's
// own source. What follows is not the evidence that they are right; the
// evidence is that the streams in testdata decompress to what an independent
// decoder says they hold, which no wrong entry in any of these tables survives.
// These are the checks that say *which* tables those are, so that regenerating
// them from somewhere else, or editing one by hand, is a failure here rather
// than a wrong glyph somewhere much later.

// TestTheDictionaryIsRFC7932s. The digest is of the file RFC 7932 Appendix A
// describes, which is the same 122,784 bytes in every Brotli implementation
// there is.
func TestTheDictionaryIsRFC7932s(t *testing.T) {
	if len(dictionary) != 122784 {
		t.Fatalf("the dictionary is %d bytes; RFC 7932's is 122784", len(dictionary))
	}
	sum := sha256.Sum256(dictionary)
	const want = "20e42eb1b511c21806d4d227d07e5dd06877d8ce7b3a817f378f313653f35c70"
	if got := hex.EncodeToString(sum[:]); got != want {
		t.Errorf("the dictionary hashes to %s, want %s", got, want)
	}
	// Appendix A's words run four bytes to twenty-four, laid end to end by
	// length. The offsets are computed from the counts, so this is the check
	// that the two agree — and it is the same assertion dictionary.h makes.
	if wordOffsets[25] != len(dictionary) {
		t.Errorf("the words of every length come to %d bytes and the dictionary "+
			"is %d", wordOffsets[25], len(dictionary))
	}
	for n := 4; n <= 24; n++ {
		if wordOffsets[n+1]-wordOffsets[n] != n<<wordBits[n] {
			t.Errorf("the %d-byte words occupy %d bytes, and there are %d of them",
				n, wordOffsets[n+1]-wordOffsets[n], 1<<wordBits[n])
		}
	}
	// The first three words, which is where Appendix A starts.
	for i, want := range []string{"time", "down", "life"} {
		if got := string(dictionary[i*4 : i*4+4]); got != want {
			t.Errorf("the %d%s four-byte word is %q, want %q", i+1, "st", got, want)
		}
	}
}

// TestTheTransformsAreRFC7932s.
func TestTheTransformsAreRFC7932s(t *testing.T) {
	if len(transforms) != 121 {
		t.Fatalf("%d transforms; RFC 7932 §8 lists 121", len(transforms))
	}
	// The first three of Appendix B, which pin both the order and the meaning
	// of the three fields.
	for i, want := range []transform{
		{"", identity, ""},
		{"", identity, " "},
		{" ", identity, " "},
	} {
		if transforms[i] != want {
			t.Errorf("transform %d is %+v, want %+v", i, transforms[i], want)
		}
	}
	// The last, which is where the table ends.
	if got := transforms[120]; got != (transform{" ", upperCaseFirst, "='"}) {
		t.Errorf("the last transform is %+v", got)
	}
	for i, tr := range transforms {
		if tr.kind < identity || tr.kind > omitFirst9 {
			t.Errorf("transform %d does what RFC 7932 does not define: %d", i, tr.kind)
		}
	}
}

// TestATransformedWordIsWhatTheOperationSays walks the operations against a
// word whose letters are all distinguishable, so that a transform applied to
// the wrong end is visible rather than merely different.
func TestATransformedWordIsWhatTheOperationSays(t *testing.T) {
	for _, tc := range []struct {
		what string
		tr   transform
		want string
	}{
		{"identity", transform{"", identity, ""}, "abcdefgh"},
		{"a prefix and a suffix", transform{"<", identity, ">"}, "<abcdefgh>"},
		{"cutting the end off", transform{"", omitLast1, ""}, "abcdefg"},
		{"cutting three off the end", transform{"", omitLast1 + 2, ""}, "abcde"},
		{"cutting the front off", transform{"", omitFirst1, ""}, "bcdefgh"},
		{"cutting three off the front", transform{"", omitFirst1 + 2, ""}, "defgh"},
		{"upper-casing the first", transform{"", upperCaseFirst, ""}, "Abcdefgh"},
		{"upper-casing everything", transform{"", upperCaseAll, ""}, "ABCDEFGH"},
		// The order the parts go in: the prefix is not upper-cased and the
		// suffix is not either, because only the word is.
		{"upper-casing between a prefix and a suffix",
			transform{"a", upperCaseAll, "z"}, "aABCDEFGHz"},
		{"cutting and wrapping", transform{"[", omitFirst1 + 1, "]"}, "[cdefgh]"},
	} {
		got, err := applyTransform(nil, []byte("abcdefgh"), tc.tr)
		if err != nil {
			t.Errorf("%s: %v", tc.what, err)
			continue
		}
		if string(got) != tc.want {
			t.Errorf("%s: %q, want %q", tc.what, got, tc.want)
		}
	}
}

// TestEveryTransformIsReachable, and the one past the end is not.
//
// A dictionary reference names a word and a transform in one number, and the
// bound on the transform half is the only thing standing between a corrupt
// stream and a read past the end of the table. The last transform is checked
// alongside the first because a bound that is one too tight loses it, and
// nothing else here would notice: transform 120 is rare enough that no captured
// stream in testdata uses it.
func TestEveryTransformIsReachable(t *testing.T) {
	// Four-byte words, of which there are 1<<10, so the transform is the index
	// divided by that.
	const length, count = 4, 1 << 10
	for _, kind := range []int{0, 1, 60, len(transforms) - 1} {
		got, err := word(nil, length, kind*count+7)
		if err != nil {
			t.Errorf("transform %d: %v", kind, err)
			continue
		}
		tr := transforms[kind]
		if !strings.HasPrefix(string(got), tr.prefix) ||
			!strings.HasSuffix(string(got), tr.suffix) {
			t.Errorf("transform %d produced %q, which is not %q...%q",
				kind, got, tr.prefix, tr.suffix)
		}
	}
	// One past the end names nothing.
	if _, err := word(nil, length, len(transforms)*count); !errors.Is(err, errNoSuchTransform) {
		t.Errorf("a transform past the end gave %v, want %v", err, errNoSuchTransform)
	}
	// So does a word length the dictionary has none of.
	for _, length := range []int{0, 3, 25, 100} {
		if _, err := word(nil, length, 0); !errors.Is(err, errNoSuchWord) {
			t.Errorf("a %d-byte word gave %v, want %v", length, err, errNoSuchWord)
		}
	}
}

// TestUpperCasingFollowsRFC7932AndNotUnicode.
//
// §8's rule is a stand-in for case mapping and not the real thing: for a
// two-byte character it flips one bit of the second byte, and for a three-byte
// one it flips a bit that is not a case distinction at all. Doing it properly
// would produce different bytes from the ones the encoder assumed, so the
// wrongness is the specification's and has to be preserved exactly.
func TestUpperCasingFollowsRFC7932AndNotUnicode(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		n        int
	}{
		{"abc", "Abc", 1},
		{"Abc", "Abc", 1},
		{"1bc", "1bc", 1},
		// Two bytes: the second one's bit 5 flips, whatever that means.
		{"éx", "Éx", 2},
		// Three bytes: bit 2 of the third, which is not case at all.
		{"你x", "佥x", 3},
	} {
		b := []byte(tc.in)
		if n := upperCase(b); n != tc.n {
			t.Errorf("%q: stepped %d bytes, want %d", tc.in, n, tc.n)
		}
		if string(b) != tc.want {
			t.Errorf("%q became %q, want %q", tc.in, b, tc.want)
		}
	}
}

// TestTheContextModelsAreRFC7932s.
//
// Two of the four are stated as arithmetic and are checked as arithmetic here,
// which is a real comparison against §7.1 rather than against this table. The
// other two are tables in the specification, and what says they are right is
// that the streams in testdata decompress correctly — a literal read with the
// wrong context is read with the wrong prefix code and comes out as the wrong
// byte.
func TestTheContextModelsAreRFC7932s(t *testing.T) {
	if len(contextLookup) != 2048 {
		t.Fatalf("the context table is %d bytes, want 2048", len(contextLookup))
	}
	// LSB6: the low six bits of the byte before, and nothing from the one
	// before that.
	for i := 0; i < 256; i++ {
		if got := int(contextLookup[i]); got != i&0x3f {
			t.Fatalf("LSB6 of %d is %d, want %d", i, got, i&0x3f)
		}
		if contextLookup[256+i] != 0 {
			t.Fatalf("LSB6 reads the second byte back, and §7.1 says it does not")
		}
	}
	// MSB6: the high six bits.
	for i := 0; i < 256; i++ {
		if got := int(contextLookup[512+i]); got != i>>2 {
			t.Fatalf("MSB6 of %d is %d, want %d", i, got, i>>2)
		}
		if contextLookup[512+256+i] != 0 {
			t.Fatalf("MSB6 reads the second byte back, and §7.1 says it does not")
		}
	}
	// Every model combines its two halves by OR, which only works if no
	// context can escape the sixty-four a literal block has codes for.
	for mode := 0; mode < 4; mode++ {
		for p1 := 0; p1 < 256; p1++ {
			for p2 := 0; p2 < 256; p2++ {
				ctx := contextLookup[mode<<9+p1] | contextLookup[mode<<9+256+p2]
				if ctx > 63 {
					t.Fatalf("model %d turns (%d, %d) into context %d, and there "+
						"are 64", mode, p1, p2, ctx)
				}
			}
		}
	}
}

// TestTheCommandTableIsRFC7932s.
//
// This checks the whole of it rather than a few entries, and it does so against
// the specification's own three tables written out longhand: which quarter of
// each range the eleven blocks of sixty-four cover (§5), what each insert code
// means (§5's insert-length table) and what each copy code means (§5's
// copy-length table). command.go derives all three from a packed form, which is
// how the reference implementation does it and is unreadable on its own; this
// is the readable statement of what it is supposed to come to.
func TestTheCommandTableIsRFC7932s(t *testing.T) {
	if len(commandLut) != 704 {
		t.Fatalf("%d command symbols, want 704", len(commandLut))
	}
	// §5's insert lengths: where each code starts, and how many bits follow.
	insertFrom := [24]int{0, 1, 2, 3, 4, 5, 6, 8, 10, 14, 18, 26,
		34, 50, 66, 98, 130, 194, 322, 578, 1090, 2114, 6210, 22594}
	insertBits := [24]uint{0, 0, 0, 0, 0, 0, 1, 1, 2, 2, 3, 3,
		4, 4, 5, 5, 6, 7, 8, 9, 10, 12, 14, 24}
	// §5's copy lengths. Copying nothing is not a command, so they start at two.
	copyFrom := [24]int{2, 3, 4, 5, 6, 7, 8, 9, 10, 12, 14, 18,
		22, 30, 38, 54, 70, 102, 134, 198, 326, 582, 1094, 2118}
	copyBits := [24]uint{0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 2, 2,
		3, 3, 4, 4, 5, 5, 6, 7, 8, 9, 10, 24}
	// §5's eleven blocks: the first insert code and the first copy code each
	// covers, and whether its commands carry a distance of their own.
	blocks := []struct {
		insert, copy int
		spelled      bool
	}{
		{0, 0, false}, {0, 8, false},
		{0, 0, true}, {0, 8, true}, {8, 0, true}, {8, 8, true},
		{0, 16, true}, {16, 0, true}, {8, 16, true}, {16, 8, true}, {16, 16, true},
	}
	if len(blocks)*64 != len(commandLut) {
		t.Fatalf("%d blocks of 64 is not 704", len(blocks))
	}
	for sym, got := range commandLut {
		b := blocks[sym>>6]
		insertCode := b.insert + (sym>>3)&7
		copyCode := b.copy + sym&7
		want := command{
			insertBits:     insertBits[insertCode],
			copyBits:       copyBits[copyCode],
			insertOffset:   insertFrom[insertCode],
			copyOffset:     copyFrom[copyCode],
			context:        3,
			repeatDistance: !b.spelled,
		}
		// The distance code a copy uses is chosen by how long it is, and the
		// three shortest copies get one each.
		if want.copyOffset <= 4 {
			want.context = want.copyOffset - 2
		}
		if got != want {
			t.Errorf("command %d is %+v, want %+v", sym, got, want)
		}
	}
}
