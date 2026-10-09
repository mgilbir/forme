package shape

import (
	"errors"
	"fmt"
	"sync"

	"github.com/mgilbir/forme/font"
)

// Fonts from a collection: a TrueType or OpenType collection, .ttc or .otc,
// several fonts in one file that share the tables they have in common. Many
// system fonts ship only as collections — on macOS Apple Color Emoji,
// Helvetica, PingFang and most CJK families; on Linux Noto Sans CJK — and Load
// takes a single font.
//
// A face of a collection is read from the collection's own bytes: its tables
// are slices of them (font.CollectionTables), so the tables two faces share,
// an emoji font's bitmaps say, are not copied for each face loaded. What does
// need a font of its own is embedding, and Program builds one, its tables
// copied out into a standalone sfnt, the first time it is asked for.

// collectionFace is what a face of a collection keeps in place of the program
// a face from Load keeps: its tables, and the standalone program built from
// them once it is wanted. It is shared by the face's clones.
type collectionFace struct {
	tables map[string][]byte

	once       sync.Once
	standalone []byte
}

// program is the face as a font of its own: its tables, copied out of the
// collection into an sfnt.
func (c *collectionFace) program() []byte {
	c.once.Do(func() { c.standalone = standaloneProgram(c.tables) })
	return c.standalone
}

// standaloneProgram is a font taken apart into its tables, written back out as
// an sfnt of its own: OpenType's 'OTTO' where its outlines are CFF, TrueType's
// otherwise.
func standaloneProgram(tables map[string][]byte) []byte {
	if _, cff := tables["CFF "]; cff {
		return assembleOTTO(tables)
	}
	return assembleSFNT(tables)
}

// sfntTables is the face's font taken apart into its tables: a face of a
// collection's own, and a face from Load's program's, as Load took it apart.
// It is nil for a standard face, which has no program. The map is the face's
// and its clones', and is not to be changed.
func (f *Face) sfntTables() map[string][]byte {
	if f.collection != nil {
		return f.collection.tables
	}
	if f.tables != nil {
		return f.tables
	}
	return font.SFNTTables(f.data)
}

// programSize is how large a face's font is, for the budgets sized by it: its
// program's length, or a face of a collection's tables'.
func programSize(data []byte, tables map[string][]byte) int {
	if data != nil {
		return len(data)
	}
	n := 0
	for _, t := range tables {
		n += len(t)
	}
	return n
}

// LoadCollection loads one face of a font collection, by its index from zero:
// a .ttc or .otc, or one wrapped as WOFF 2, which is unwrapped first.
// Its tables are read from the collection where they are and not copied — for
// a .ttc or .otc, from data, which must not be modified while the face is in
// use; Program copies them out into a font of their own for embedding.
//
// A single font, which is a collection of one, loads as Load loads it at index
// zero. A face of a collection loads at its default instance, as Load loads a
// variable font.
func LoadCollection(data []byte, index int) (*Face, error) {
	tables, err := collectionFaceTables(data, index)
	if err != nil {
		return nil, err
	}
	if tables == nil {
		return Load(data)
	}
	f, err := loadTables(nil, tables, nil)
	if err != nil {
		return nil, err
	}
	f.collection = &collectionFace{tables: tables}
	return f, nil
}

// LoadCollectionInstance is LoadInstance for one face of a font collection: the
// face, by its index from zero, at a point in its design space. An instance is
// a font program of its own, written for the location, so it shares nothing
// with the collection. A single font loads as LoadInstance loads it, at index
// zero.
func LoadCollectionInstance(data []byte, index int, coords map[string]float64) (*Face, error) {
	tables, err := collectionFaceTables(data, index)
	if err != nil {
		return nil, err
	}
	if tables == nil {
		return LoadInstance(data, coords)
	}
	return LoadInstance(standaloneProgram(tables), coords)
}

// collectionFaceTables is the tables of a face of a collection, and nil and no
// error for data that is a single font, whose only face is index zero.
func collectionFaceTables(data []byte, index int) (map[string][]byte, error) {
	data, err := unwrapWOFF(data)
	if err != nil {
		return nil, err
	}
	offsets := font.CollectionOffsets(data)
	if offsets == nil {
		if index != 0 {
			return nil, fmt.Errorf("fonts: a single font has one face, and face %d was asked for", index)
		}
		return nil, nil
	}
	if index < 0 || index >= len(offsets) {
		return nil, fmt.Errorf("fonts: the collection holds %d fonts, and font %d was asked for", len(offsets), index)
	}
	tables := font.CollectionTables(data, index)
	if tables == nil {
		return nil, fmt.Errorf("fonts: font %d of the collection has a table directory that cannot be read", index)
	}
	return tables, nil
}

// unwrapWOFF is a WOFF or WOFF 2 file's font, or font collection, unwrapped,
// and any other data as it is.
func unwrapWOFF(data []byte) ([]byte, error) {
	if font.IsWOFF(data) || font.IsWOFF2(data) {
		return font.DecodeWOFF(data)
	}
	return data, nil
}

// CollectionFace describes one face of a font file, as the face describes
// itself once loaded: see Face.Name, Family, Subfamily and Descriptor.
type CollectionFace struct {
	// Index is the face's index, which LoadCollection takes.
	Index int
	// Name is the PostScript name, as Face.Name states it.
	Name              string
	Family, Subfamily string
	// Weight and WidthClass are OS/2's, 100 to 900 and 1 to 9; Italic and
	// Oblique are its style bits. See Descriptor.
	Weight, WidthClass int
	Italic, Oblique    bool
}

// CollectionFaces describes each face of a font collection, without loading
// any: from its name, OS/2 and head tables alone. A single font is described
// as a collection of one. A face whose table directory cannot be read is
// described with its index only; LoadCollection says what is wrong with it.
func CollectionFaces(data []byte) ([]CollectionFace, error) {
	data, err := unwrapWOFF(data)
	if err != nil {
		return nil, err
	}
	offsets := font.CollectionOffsets(data)
	if offsets == nil {
		tables := font.SFNTTables(data)
		if tables == nil {
			return nil, errors.New("fonts: neither a font collection nor an sfnt font program")
		}
		return []CollectionFace{describeTables(0, tables)}, nil
	}
	faces := make([]CollectionFace, len(offsets))
	for i := range offsets {
		faces[i] = describeTables(i, font.CollectionTables(data, i))
	}
	return faces, nil
}

// describeTables reads what CollectionFaces says of a face from its tables,
// with the readers Load reads them with.
func describeTables(index int, tables map[string][]byte) CollectionFace {
	if tables == nil {
		return CollectionFace{Index: index}
	}
	f := &Face{}
	f.readOS2(tables["OS/2"])
	f.readStyle(tables["OS/2"], tables["head"])
	return CollectionFace{
		Index:      index,
		Name:       faceName(tables["name"]),
		Family:     nameWithFallback(tables["name"], 16, 1),
		Subfamily:  nameWithFallback(tables["name"], 17, 2),
		Weight:     f.weight,
		WidthClass: f.widthClass,
		Italic:     f.styleItalic,
		Oblique:    f.styleOblique,
	}
}
