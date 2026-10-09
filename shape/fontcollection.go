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
// program's length, or a face of a collection's tables'. Those are at most
// twice the collection's length, however its tables overlap, since
// font.SFNTTables refuses a directory whose tables are more: summed as they
// were stated, n tables on one range sized the outline allowance
// (newOutlineCache) by n times the range.
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
// So is a face past what describeAllowance lets the collection's description
// read, which no collection that is not built to cost reaches.
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
		return []CollectionFace{describeTables(0, tables, readFaceNames(tables["name"]))}, nil
	}
	// A face's directory and its name table are found by offsets, and nothing
	// stops every face from naming one directory, or every directory one name
	// table: each face read them afresh, so n faces on one directory of D
	// tables cost n×D, and on one name table n reads of it, quadratic in the
	// file. A directory is read once however many faces name it, and a name
	// table once however many directories do, and each face is described as
	// it was when it read them itself. What is left — directories and name
	// tables at distinct places, which may overlap — is charged against
	// describeAllowance, and a face past it is described by its index.
	faces := make([]CollectionFace, len(offsets))
	described := map[int]CollectionFace{}
	names := map[nameTable]faceNames{}
	left := describeAllowance(data)
	for i, at := range offsets {
		if d, ok := described[at]; ok {
			d.Index = i
			faces[i] = d
			continue
		}
		dir := directorySize(data, at)
		if dir > left {
			faces[i] = CollectionFace{Index: i}
			continue
		}
		left -= dir
		tables := font.CollectionTables(data, i)
		if tables == nil {
			faces[i] = CollectionFace{Index: i}
			described[at] = faces[i]
			continue
		}
		name := tables["name"]
		key := nameTable{len: len(name)}
		if len(name) > 0 {
			key.at = &name[0]
		}
		n, ok := names[key]
		if !ok {
			if len(name) > left {
				faces[i] = CollectionFace{Index: i}
				continue
			}
			left -= len(name)
			n = readFaceNames(name)
			names[key] = n
		}
		faces[i] = describeTables(i, tables, n)
		described[at] = faces[i]
	}
	return faces, nil
}

// describeAllowance is how many bytes of directories and name tables
// CollectionFaces may read from a collection, each one once.
//
// An honest collection's directories and name tables are distinct bytes of it,
// so between them they are less than the file; across the 153 collection
// files measured (619 faces: macOS's and Windows's, Noto Sans CJK, WenQuanYi,
// HarfBuzz's and this tree's own) they were at most 0.62 of it, in HarfBuzz's
// TTC.ttc, whose two faces share their name table. Twice the file leaves a
// collection whose tables are not where they should be room to be described.
func describeAllowance(data []byte) int { return 2 * len(data) }

// directorySize is the bytes of the table directory at at that reading it
// reads: its header and its records, or nothing much for one font.SFNTTables
// refuses before reading its records because they run past the end.
func directorySize(data []byte, at int) int {
	if at < 0 || at > len(data)-12 {
		return 0
	}
	if size := 12 + 16*font.Be16(data, at+4); size <= len(data)-at {
		return size
	}
	return 0
}

// nameTable is a name table by where it is, so that faces sharing one read it
// once: the address of its first byte, and its length.
type nameTable struct {
	at  *byte
	len int
}

// faceNames is what CollectionFaces reads from a name table.
type faceNames struct{ name, family, subfamily string }

func readFaceNames(name []byte) faceNames {
	return faceNames{
		name:      faceName(name),
		family:    nameWithFallback(name, 16, 1),
		subfamily: nameWithFallback(name, 17, 2),
	}
}

// describeTables reads what CollectionFaces says of a face from its tables,
// with the readers Load reads them with, and its names, read from its name
// table.
func describeTables(index int, tables map[string][]byte, names faceNames) CollectionFace {
	f := &Face{}
	f.readOS2(tables["OS/2"])
	f.readStyle(tables["OS/2"], tables["head"])
	return CollectionFace{
		Index:      index,
		Name:       names.name,
		Family:     names.family,
		Subfamily:  names.subfamily,
		Weight:     f.weight,
		WidthClass: f.widthClass,
		Italic:     f.styleItalic,
		Oblique:    f.styleOblique,
	}
}
