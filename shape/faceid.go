package shape

import (
	"encoding/binary"
	"hash/maphash"
	"math"
	"slices"
	"sync"
)

// FaceID is a face's identity for a backend to key what it keeps by: two faces
// with the same FaceID draw and shape the same, and two that would not have
// different ones.
//
// It is needed because a face's pointer is not one. A face records the glyphs
// a document set in it (see Clone), so layout hands every document a face of
// its own — a clone of the caller's, an instance cut for it, a font its
// @font-face rule loaded — and the same font comes out as a new *Face in
// every document. A backend that caches per face by pointer misses on every
// document and keeps every face it ever saw alive. Keyed by ID, it hits.
//
// It is computed from what the face is made of: the font program (for a face
// of a collection, the face's own tables; for a standard face, its name), the
// point in the design space it was cut at, the font-feature-settings it was
// given (WithFeatureSettings), and whether it is embedded as a simple font
// (LoadSimple). The glyphs a document used are not part of it, so a clone has
// its face's ID, and so does the same file loaded again.
//
// It is a 128-bit hash, seeded per process: stable for as long as the process
// runs, and not to be stored or compared across processes. Two different fonts
// sharing one is as likely as any two random 128-bit numbers being equal.
type FaceID struct{ hi, lo uint64 }

// ID is the face's identity. See FaceID.
//
// The program is hashed the first time any of a face's clones is asked, and
// the answer shared by them all; for a font of ten megabytes that is under a
// millisecond, against the milliseconds it took to load. A face nothing asks
// for its ID never pays for it.
func (f *Face) ID() FaceID {
	id := f.ident
	if id == nil {
		// A face no constructor made: computed every time, which is still the
		// right answer, only not kept.
		id = &faceIdent{}
	}
	id.once.Do(func() { id.hi, id.lo = f.hashSource(identSeeds[0]), f.hashSource(identSeeds[1]) })
	return FaceID{hi: f.hashTraits(identSeeds[0], id.hi), lo: f.hashTraits(identSeeds[1], id.lo)}
}

// faceIdent is the hash of what a face and its clones draw from, made once.
type faceIdent struct {
	once   sync.Once
	hi, lo uint64
}

// identSeeds are the two seeds a FaceID's halves are hashed under: per
// process, which is the whole of the promise FaceID makes.
var identSeeds = [2]maphash.Seed{maphash.MakeSeed(), maphash.MakeSeed()}

// hashSource hashes what the face is read from, which its clones share: the
// program, or a collection face's tables, or a standard face's name. Each kind
// is written under its own prefix, so a name cannot collide with a program.
func (f *Face) hashSource(seed maphash.Seed) uint64 {
	var h maphash.Hash
	h.SetSeed(seed)
	switch {
	case f.collection != nil:
		h.WriteString("collection\x00")
		tags := make([]string, 0, len(f.collection.tables))
		for tag := range f.collection.tables {
			tags = append(tags, tag)
		}
		slices.Sort(tags)
		for _, tag := range tags {
			b := f.collection.tables[tag]
			h.WriteString(tag)
			writeUint64(&h, uint64(len(b)))
			h.Write(b)
		}
	case f.data != nil:
		h.WriteString("program\x00")
		h.Write(f.data)
	default:
		h.WriteString("standard\x00")
		h.WriteString(f.name)
	}
	return h.Sum64()
}

// hashTraits hashes the source's hash with what can differ between faces that
// share a source: the design-space location, the feature settings and the
// embedding. It is cheap, so it is not kept — WithFeatureSettings and
// LoadSimple change a face after it shares its source's ident.
func (f *Face) hashTraits(seed maphash.Seed, source uint64) uint64 {
	var h maphash.Hash
	h.SetSeed(seed)
	writeUint64(&h, source)
	writeUint64(&h, uint64(len(f.varCoords)))
	for _, c := range f.varCoords {
		var b [8]byte
		binary.LittleEndian.PutUint64(b[:], math.Float64bits(c))
		h.Write(b[:])
	}
	h.WriteString(f.settingsOn)
	h.WriteByte(0)
	h.WriteString(f.settingsOff)
	h.WriteByte(0)
	if f.simple {
		h.WriteByte(1)
	} else {
		h.WriteByte(0)
	}
	return h.Sum64()
}

// writeUint64 writes n as eight bytes.
func writeUint64(h *maphash.Hash, n uint64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], n)
	h.Write(b[:])
}
