package brotli

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// What this package is checked against.
//
// A decompressor is right or it is not, and "looks plausible" is not a state it
// can be in: one wrong byte in a font's glyf table is a wrong glyph, and one
// wrong byte in its loca table is a font that will not load at all. So the tests
// here compare against another implementation rather than against themselves.
//
// testdata/expected.txt records what google/brotli's own C decoder makes of
// every stream beside it. That decoder is not this one and shares no code with
// it, so agreeing with it byte for byte is evidence; a fixture this package
// generated for itself would not be.
//
// The streams were chosen to reach the corners of RFC 7932 rather than to be
// representative: an empty stream, one byte, one repeated byte (which is the
// degenerate prefix code that reads no bits at all), all 256 byte values (which
// forces the long form of a code description), text (which leans on the static
// dictionary), UTF-8 (which selects a different context model), records (which
// lean on the recent-distance cache), incompressible noise (which is stored
// rather than compressed), and an output well past the 128KB an encoder
// splits meta-blocks at. Windows from 1KB to 4MB, qualities from 0 to 11.

// vector is one line of the manifest.
type vector struct {
	name   string
	length int
	digest string
}

func readManifest(t *testing.T) []vector {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "expected.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []vector
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			t.Fatalf("testdata/expected.txt: %q is not <file> <length> <digest>", line)
		}
		n, err := strconv.Atoi(fields[1])
		if err != nil {
			t.Fatalf("testdata/expected.txt: %q has no length", line)
		}
		out = append(out, vector{fields[0], n, fields[2]})
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func digestOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])[:16]
}

// TestEveryStreamDecompressesToWhatTheReferenceSays.
func TestEveryStreamDecompressesToWhatTheReferenceSays(t *testing.T) {
	vectors := readManifest(t)
	for _, v := range vectors {
		src, err := os.ReadFile(filepath.Join("testdata", v.name))
		if err != nil {
			t.Errorf("%s: %v", v.name, err)
			continue
		}
		got, err := Decode(src, 1<<24)
		if err != nil {
			t.Errorf("%s: %v", v.name, err)
			continue
		}
		if len(got) != v.length {
			t.Errorf("%s: %d bytes, and the reference decoder gets %d",
				v.name, len(got), v.length)
			continue
		}
		if d := digestOf(got); d != v.digest {
			t.Errorf("%s: the output hashes to %s, and the reference decoder's "+
				"hashes to %s", v.name, d, v.digest)
		}
	}

	// Every stream in the directory must be in the manifest, so that adding one
	// and forgetting to record what it holds is a failure rather than a file
	// nothing reads.
	files, err := filepath.Glob(filepath.Join("testdata", "*.br"))
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	for _, v := range vectors {
		listed[v.name] = true
	}
	for _, f := range files {
		if !listed[filepath.Base(f)] {
			t.Errorf("%s is in testdata and not in expected.txt", filepath.Base(f))
		}
	}
	if len(files) != len(vectors) {
		t.Errorf("%d streams and %d manifest lines", len(files), len(vectors))
	}
	// And a floor, so the comparison cannot quietly fall to nothing.
	if len(vectors) < 15 {
		t.Errorf("%d vectors; this is meant to cover the format's corners", len(vectors))
	}
}

// aStream is one real stream to take apart, with what it holds.
func aStream(t *testing.T) (src []byte, length int, digest string) {
	t.Helper()
	for _, v := range readManifest(t) {
		// One large enough to have structure worth truncating in the middle of.
		if v.length > 5000 && v.length < 50000 {
			b, err := os.ReadFile(filepath.Join("testdata", v.name))
			if err != nil {
				t.Fatal(err)
			}
			return b, v.length, v.digest
		}
	}
	t.Fatal("no stream in the manifest is the right size")
	return nil, 0, ""
}

// TestATruncatedStreamIsRefused.
//
// This is the property the bit reader's bit-counting exists for, and it is
// worth stating why it is not obvious. Reading past the end yields zeroes
// rather than failing, because checking at every read would put a branch in the
// decoder's innermost loop. What makes that safe is the count afterwards — and
// a count that were wrong would show up here as a truncated stream that
// decodes, quietly, to a prefix of the truth with invented bytes on the end.
func TestATruncatedStreamIsRefused(t *testing.T) {
	src, _, _ := aStream(t)
	for n := 0; n < len(src); n++ {
		if _, err := Decode(src[:n], 1<<24); err == nil {
			t.Fatalf("the first %d bytes of a %d-byte stream decoded without "+
				"complaint", n, len(src))
		}
	}
	// The control: the whole thing does decode, so the loop above is not
	// passing because nothing decodes.
	if _, err := Decode(src, 1<<24); err != nil {
		t.Fatalf("the whole stream: %v", err)
	}
}

// TestTheOutputLimitIsHonoured. A Brotli stream states its window size and not
// its output size, so a few hundred bytes can decompress to gigabytes. The
// limit is the caller's only defence and has to hold exactly.
//
// It runs over every stream rather than one, and the reason is worth recording:
// the first version used a single stream, and that stream leaned on the static
// dictionary, where the length of what is appended is not known before it is
// appended and so the limit is checked in a second place. Removing the check
// that guards every *other* path left the test passing.
func TestTheOutputLimitIsHonoured(t *testing.T) {
	for _, v := range readManifest(t) {
		src, err := os.ReadFile(filepath.Join("testdata", v.name))
		if err != nil {
			t.Fatal(err)
		}
		if v.length == 0 {
			continue
		}
		for _, limit := range []int{0, v.length / 2, v.length - 1} {
			got, err := Decode(src, limit)
			if err == nil {
				t.Errorf("%s: a limit of %d let %d bytes through", v.name, limit, v.length)
			}
			if len(got) > limit {
				t.Errorf("%s: a limit of %d produced %d bytes", v.name, limit, len(got))
			}
		}
		// Exactly enough is enough: the limit is a ceiling, not a margin.
		got, err := Decode(src, v.length)
		if err != nil {
			t.Errorf("%s: a limit of exactly its size: %v", v.name, err)
			continue
		}
		if digestOf(got) != v.digest {
			t.Errorf("%s: decompressing up to an exact limit changed the output", v.name)
		}
	}
}

// TestAMalformedStreamIsRefusedRatherThanCrashing.
//
// Fonts arrive from the network and this decoder is the first thing that reads
// them, so a stream that is not a stream must produce an error and nothing
// else: not a panic, not a read past the end of a slice, and not a loop that
// does not finish.
func TestAMalformedStreamIsRefusedRatherThanCrashing(t *testing.T) {
	src, _, _ := aStream(t)
	// A cheap deterministic spread of corruptions rather than a random one, so
	// a failure here can be reproduced from the test alone.
	for i := 0; i < 2000; i++ {
		bad := append([]byte(nil), src...)
		at := (i * 7919) % len(bad)
		bad[at] ^= 1 << uint(i%8)
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("flipping bit %d of byte %d panicked: %v", i%8, at, r)
				}
			}()
			out, _ := Decode(bad, 1<<20)
			if len(out) > 1<<20 {
				t.Errorf("flipping bit %d of byte %d produced %d bytes past the limit",
					i%8, at, len(out))
			}
		}()
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			t.Fatalf("flipping bit %d of byte %d did not finish", i%8, at)
		}
	}
	// Bytes that were never a Brotli stream at all.
	for i := 0; i < 500; i++ {
		junk := make([]byte, i)
		for k := range junk {
			junk[k] = byte(i*31 + k*17)
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%d bytes of junk panicked: %v", i, r)
				}
			}()
			Decode(junk, 1<<20)
		}()
	}
}

// TestALargeWindowStreamIsRefused. Large-window Brotli is an extension to
// RFC 7932 with windows up to a gigabyte, written with a bit pattern RFC 7932
// leaves undefined. Reading one as though the window were ordinary would give a
// plausible wrong answer, so it is refused by name.
func TestALargeWindowStreamIsRefused(t *testing.T) {
	// The window field, bit by bit: 1, then three zeroes, then 001 — the
	// pattern §9.1 does not assign.
	_, err := Decode([]byte{0b0_0010001, 0, 0, 0}, 1<<20)
	if err != errLargeWindow {
		t.Errorf("a large-window stream gave %v, want %v", err, errLargeWindow)
	}
	// And the ordinary windows around it are still read, so this is not
	// refusing everything.
	for _, b := range []byte{0b0_0000000, 0b0_0010000, 0b0_0000001} {
		if _, err := Decode([]byte{b, 0, 0, 0}, 1<<20); err == errLargeWindow {
			t.Errorf("the window byte %08b was refused as a large window", b)
		}
	}
}
