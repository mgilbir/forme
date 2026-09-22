package style

import (
	"hash/maphash"
	"iter"
	"sort"
)

// The computed style, and why it is not a map.
//
// It was one: a map from every registered property name to its value, built
// afresh for every element. That is the obvious shape and it is proportional to
// the wrong thing. A hundred and forty-eight entries is about nine kilobytes, and
// it is spent on an element that declares nothing exactly as on one that
// declares everything, so "<i></i>" repeated a quarter of a million times — under
// two megabytes of markup, and no stylesheet at all — held two and a half
// gigabytes once styled. Nothing bounded it: the cap on boxes acts after the
// cascade, and the cap on the tree allows four times that document.
//
// What an element's computed style actually consists of is two things, and
// neither needs a copy per element:
//
//   - The properties that inherit. Undeclared, each is the parent's value, so an
//     element that sets none of them has exactly its parent's — the whole block
//     of them, shared by pointer. An element that does set one gets a copy of the
//     block with that one changed, and its children share *that*. The block is
//     never written after the element it belongs to is finished, which is what
//     makes sharing it safe; see styleBuilder.
//   - The properties that do not inherit. Undeclared, each is its initial value,
//     whatever the parent says — so what is stored is only the values that
//     differ from it, sorted by property, and an element that declares none of
//     them stores nothing.
//
// So an element that declares nothing costs a pointer and an empty slice, and one
// that declares something costs what it declared. What a caller sees is
// unchanged: every registered property has a value, the inherited ones inherit,
// the others are initial where nothing set them, and a name that is not a
// property reads as the empty string, as a missing map key did.
//
// # What this is not
//
// It is still text. The audit that found the nine kilobytes (tension 4.3) also
// asks for a typed value per property, parsed once, so that every reader stops
// re-tokenising the string it is handed; that is a change to what a value *is*,
// and this is a change to where values are kept. It is the part of that
// proposal the memory needed, and the accessor below is the seam the rest can
// be done behind.

// propID is a registered property's index in propNames.
type propID uint16

// propSlot is what the representation needs to know about one property.
type propSlot struct {
	inherits bool
	initial  string
	// slot is the property's index in an inherited block, and is meaningful
	// only where inherits is true.
	slot int
}

// property is the registry entry the slot was made from.
func (p *propSlot) property() property {
	return property{inherits: p.inherits, initial: p.initial}
}

// registry is the property table indexed once: by name, by ID, and the order
// the IDs are handed out in, which is the names sorted so that iteration is the
// same on every run.
type registryIndex struct {
	names []string
	ids   map[string]propID
	slots []propSlot
	// inherited is the number of properties that inherit, which is the length
	// of every inherited block.
	inherited int
	// initial is the inherited block of an element with no parent: every
	// inheriting property at its initial value.
	initial *inheritedBlock
}

var registry = indexRegistry()

func indexRegistry() registryIndex {
	var r registryIndex
	for name := range properties {
		r.names = append(r.names, name)
	}
	sort.Strings(r.names)
	if len(r.names) > 1<<16 {
		panic("style: more registered properties than a propID holds")
	}
	r.ids = make(map[string]propID, len(r.names))
	r.slots = make([]propSlot, len(r.names))
	var initial []string
	for i, name := range r.names {
		p := properties[name]
		r.ids[name] = propID(i)
		r.slots[i] = propSlot{inherits: p.inherits, initial: p.initial, slot: -1}
		if p.inherits {
			r.slots[i].slot = r.inherited
			r.inherited++
			initial = append(initial, p.initial)
		}
	}
	r.initial = &inheritedBlock{v: initial}
	return r
}

// inheritedBlock is the values of every inheriting property, in slot order.
//
// It is shared between an element and every descendant that changes none of
// them, so it is immutable once the element it was made for is finished.
type inheritedBlock struct {
	v []string
}

// ownValue is one non-inheriting property whose value differs from its initial
// value.
type ownValue struct {
	id    propID
	value string
}

// ComputedStyle is one element's resolved property values.
//
// Every registered property has one: Get never has to tell "unset" from
// "absent", because the inherited properties inherit and the rest are at their
// initial value unless something set them. The zero ComputedStyle is the one
// exception, and is what a missing entry in Styled.Styles reads as: no style at
// all, every property the empty string, as a nil map was.
//
// It is a small value — a pointer and a slice — and copying it copies neither
// block. Both are immutable, and every method that changes something returns a
// new ComputedStyle, so a style handed to a caller cannot be changed under
// anyone else who holds it.
type ComputedStyle struct {
	inh *inheritedBlock
	// own is sorted by id and holds no initial values.
	own []ownValue
}

// Get is a property's computed value, or "" for a name that is not a registered
// property, or for the zero ComputedStyle.
func (cs ComputedStyle) Get(name string) string {
	if cs.inh == nil {
		return ""
	}
	id, ok := registry.ids[name]
	if !ok {
		return ""
	}
	return cs.get(id)
}

// Lookup is Get, and whether the style holds the property at all: false for a
// name that is not registered and for the zero ComputedStyle, true otherwise.
func (cs ComputedStyle) Lookup(name string) (string, bool) {
	if cs.inh == nil {
		return "", false
	}
	id, ok := registry.ids[name]
	if !ok {
		return "", false
	}
	return cs.get(id), true
}

func (cs ComputedStyle) get(id propID) string {
	p := &registry.slots[id]
	if p.inherits {
		return cs.inh.v[p.slot]
	}
	if i, found := findOwn(cs.own, id); found {
		return cs.own[i].value
	}
	return p.initial
}

// findOwn is where id is, or would go, in a sorted own list.
//
// (The standard library's slices package is not imported in this package:
// match.go has a function of that name, for the "~=" attribute selector.)
func findOwn(own []ownValue, id propID) (int, bool) {
	i := sort.Search(len(own), func(i int) bool { return own[i].id >= id })
	return i, i < len(own) && own[i].id == id
}

// IsZero reports whether this is the zero ComputedStyle, which holds nothing.
func (cs ComputedStyle) IsZero() bool { return cs.inh == nil }

// Len is the number of properties the style holds: every registered one, or
// none for the zero ComputedStyle.
func (cs ComputedStyle) Len() int {
	if cs.inh == nil {
		return 0
	}
	return len(registry.names)
}

// All is every property and its value, in the order of their names. The zero
// ComputedStyle yields nothing.
func (cs ComputedStyle) All() iter.Seq2[string, string] {
	return func(yield func(string, string) bool) {
		if cs.inh == nil {
			return
		}
		for id, name := range registry.names {
			if !yield(name, cs.get(propID(id))) {
				return
			}
		}
	}
}

// With is the style with one property set to a value, leaving the receiver as
// it was: copy-on-write, so only the block the property lives in is copied, and
// only when the value is different.
//
// The zero ComputedStyle is taken to be an element with no parent that declared
// nothing, which is every property at its initial value — the style a map with
// one key written into it would have been read as by anything that fell back to
// the initial value, and the only full style there is to start from.
//
// It panics on a name that is not a registered property. The map stored such a
// key and nothing read it back; a caller writing one is a mistake in this
// module, and saying so is better than a value that is kept and never seen.
func (cs ComputedStyle) With(name, value string) ComputedStyle {
	id, ok := registry.ids[name]
	if !ok {
		panic("style: With of " + name + ", which is not a registered property")
	}
	b := newStyleBuilder(cs)
	b.set(id, value)
	return b.cs
}

// Initial is the style of an element with no parent and no declarations: every
// property at its initial value.
func Initial() ComputedStyle {
	return ComputedStyle{inh: registry.initial}
}

// styleBuilder makes a ComputedStyle by changing another one, copying each of
// its two parts the first time something in it changes and never after.
//
// It is how the cascade builds an element's style out of its parent's: it
// starts from the parent's inherited block — shared, not copied — and no own
// values, which is exactly what an element that declares nothing has, and then
// sets what the element declared. The block is copied only if one of those
// declarations changes an inherited value, which is what keeps the block
// shared all the way down a subtree that says nothing.
//
// Nothing a builder was started from is written to: the copies are its own, and
// the ComputedStyle it hands out is not changed again once handed out.
type styleBuilder struct {
	cs ComputedStyle
	// ownInh and ownOwn say whether the two parts are this builder's own
	// copies yet, and so may be written in place.
	ownInh, ownOwn bool
}

// newStyleBuilder starts from a whole style, both parts shared.
func newStyleBuilder(from ComputedStyle) *styleBuilder {
	if from.inh == nil {
		from = Initial()
	}
	return &styleBuilder{cs: from}
}

// childStyleBuilder starts an element whose parent's style is parent: the
// parent's inherited values, shared, and every non-inheriting property at its
// initial value. The zero parent is no parent, which inherits the initial
// values.
func childStyleBuilder(parent ComputedStyle) *styleBuilder {
	inh := parent.inh
	if inh == nil {
		inh = registry.initial
	}
	return &styleBuilder{cs: ComputedStyle{inh: inh}}
}

// set gives a property a value, copying the part it lives in the first time
// that part changes.
func (b *styleBuilder) set(id propID, value string) {
	p := &registry.slots[id]
	if p.inherits {
		if b.cs.inh.v[p.slot] == value {
			return
		}
		if !b.ownInh {
			b.cs.inh = &inheritedBlock{v: append([]string(nil), b.cs.inh.v...)}
			b.ownInh = true
		}
		b.cs.inh.v[p.slot] = value
		return
	}
	i, found := findOwn(b.cs.own, id)
	if found && b.cs.own[i].value == value {
		return
	}
	if !found && value == p.initial {
		return
	}
	if !b.ownOwn {
		b.cs.own = append(make([]ownValue, 0, len(b.cs.own)+1), b.cs.own...)
		b.ownOwn = true
	}
	switch {
	case value == p.initial:
		// Back to the initial value, which is stored by not being stored.
		b.cs.own = append(b.cs.own[:i], b.cs.own[i+1:]...)
	case found:
		b.cs.own[i].value = value
	default:
		b.cs.own = append(b.cs.own, ownValue{})
		copy(b.cs.own[i+1:], b.cs.own[i:])
		b.cs.own[i] = ownValue{id: id, value: value}
	}
}

// styleInterner shares, within one document, the parts of computed styles that
// came out the same.
//
// An element that changes an inherited value gets its own copy of the block, and
// most elements that do so change it in the same way as their siblings: every
// paragraph of an article indents its first line by the same amount, every
// <em> italicises, and each of them would otherwise hold a block of its own
// that is identical to the one beside it. The same goes for the values
// themselves, which the cascade makes as text per element — "16px" is written
// out once for each paragraph that has a 1em margin.
//
// Only what a builder made is looked up: a part it shares with its parent is
// already shared, and hashing it again would be the cost this exists to avoid.
type styleInterner struct {
	seed   maphash.Seed
	blocks map[uint64][]*inheritedBlock
	owns   map[uint64][][]ownValue
	values map[string]string
}

func newStyleInterner() *styleInterner {
	return &styleInterner{seed: maphash.MakeSeed(),
		blocks: map[uint64][]*inheritedBlock{}, owns: map[uint64][][]ownValue{},
		values: map[string]string{}}
}

// interner is the Styler's, made the first time it is asked for: a Styler built
// by hand to prepare rules never styles anything and never needs one.
func (s *Styler) interner() *styleInterner {
	if s.intern == nil {
		s.intern = newStyleInterner()
	}
	return s.intern
}

// value is the document's one copy of a value string.
func (in *styleInterner) value(v string) string {
	if got, ok := in.values[v]; ok {
		return got
	}
	in.values[v] = v
	return v
}

// finish hands out the builder's style, with whatever it made replaced by an
// equal one already handed out in this document. The builder must not be used
// again.
func (in *styleInterner) finish(b *styleBuilder) ComputedStyle {
	cs := b.cs
	if b.ownInh {
		var h maphash.Hash
		h.SetSeed(in.seed)
		for _, v := range cs.inh.v {
			h.WriteString(v)
			h.WriteByte(0)
		}
		key := h.Sum64()
		found := false
		for _, prev := range in.blocks[key] {
			if equalStrings(prev.v, cs.inh.v) {
				cs.inh, found = prev, true
				break
			}
		}
		if !found {
			in.blocks[key] = append(in.blocks[key], cs.inh)
		}
	}
	if b.ownOwn && len(cs.own) > 0 {
		var h maphash.Hash
		h.SetSeed(in.seed)
		for _, o := range cs.own {
			h.WriteByte(byte(o.id))
			h.WriteByte(byte(o.id >> 8))
			h.WriteString(o.value)
			h.WriteByte(0)
		}
		key := h.Sum64()
		found := false
		for _, prev := range in.owns[key] {
			if equalOwn(prev, cs.own) {
				cs.own, found = prev, true
				break
			}
		}
		if !found {
			// Clipped, so that a later append to it — there is none; the
			// slice is immutable from here — could never reach into
			// capacity another style shares.
			cs.own = cs.own[:len(cs.own):len(cs.own)]
			in.owns[key] = append(in.owns[key], cs.own)
		}
	} else if len(cs.own) == 0 {
		cs.own = nil
	}
	return cs
}

// Inherited returns the style an anonymous box has: everything that inherits
// taken from the box it was generated inside, and everything that does not at
// its initial value.
//
// This is what the specification means by an anonymous box having no style of
// its own. It matters far more than it sounds, because the obvious shortcut —
// giving the anonymous box its parent's whole computed style — makes it a copy
// of the parent's *box model* as well: the anonymous block wrapped around a run
// of text inside <body> would take body's 8px margin, indent the text by it, and
// separate it from the block after it by a gap the author never wrote. Every
// number in that document is then plausible and wrong.
//
// It is the parent's inherited block and nothing else, so it costs nothing: the
// block is shared, and the non-inheriting properties are initial by not being
// stored. The zero style inherits the initial values, as an element with no
// parent does.
func Inherited(cs ComputedStyle) ComputedStyle {
	return childStyleBuilder(cs).cs
}

// compactIDs drops the repeats from a sorted list, in place.
func compactIDs(ids []propID) []propID {
	out := ids[:0]
	for _, id := range ids {
		if len(out) == 0 || id != out[len(out)-1] {
			out = append(out, id)
		}
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalOwn(a, b []ownValue) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
