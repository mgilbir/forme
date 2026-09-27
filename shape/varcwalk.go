package shape

import "math"

// varcWalk is one glyph drawn through VARC: HarfBuzz's hb_varc_context_t,
// and what each leaf it reaches is handed to.
type varcWalk struct {
	v *varcTable
	// fontCoords are the coordinates the face is drawn at, which a component
	// that resets the axes it does not set starts from.
	fontCoords []int
	// leaf is handed each glyph a component draws that VARC does not
	// compose, at its coordinates and through its transform, and reports
	// whether the glyph has anything to draw.
	leaf      func(gid int, coords []int, xf xform32) bool
	dec       decycler
	edgesLeft int
	depthLeft int
	// work is what the walk may still spend; spent is set once it has run
	// out, and the walk draws nothing further.
	work, start int64
	spent       bool
}

// maxVarcWork bounds the work of one VARC glyph: a unit a component, a unit
// an axis value set or varied, and what each leaf's points cost besides.
const maxVarcWork = 1 << 24

func newVarcWalk(v *varcTable, fontCoords []int, work int64, leaf func(int, []int, xform32) bool) *varcWalk {
	return &varcWalk{
		v: v, fontCoords: fontCoords, leaf: leaf,
		dec:       decycler{tortoise: -1},
		edgesLeft: hbMaxEdges, depthLeft: hbMaxNesting, work: work, start: work,
	}
}

// used is how much of its work the walk spent.
func (w *varcWalk) used() int64 { return w.start - max(w.work, 0) }

func (w *varcWalk) spend(n int64) bool {
	if w.work -= n; w.work < 0 {
		w.spent = true
		return false
	}
	return true
}

// glyph is VARC::get_path_at: a glyph the table does not cover, or the one a
// component is in, is a leaf; a covered one is its components, drawn in turn.
func (w *varcWalk) glyph(gid int, coords []int, xf xform32, parent int) bool {
	idx := -1
	if gid != parent {
		idx = w.v.coverageIndex(gid)
	}
	if idx < 0 {
		if !w.spend(1) {
			return true
		}
		return w.leaf(gid, coords, xf)
	}
	if w.depthLeft <= 0 || w.edgesLeft <= 0 {
		return true
	}
	w.edgesLeft--
	w.dec.enter()
	defer w.dec.leave()
	if !w.dec.visit(gid) {
		return true
	}
	rec := w.v.records.item(w.v.t, idx)
	for len(rec) > 0 {
		rec = w.component(gid, coords, xf, rec)
	}
	return true
}

// component is VarComponent::get_path_at: one record drawn, and the records
// after it, or none where it does not decode.
func (w *varcWalk) component(parent int, coords []int, xf xform32, rec []byte) []byte {
	if !w.spend(4) {
		return nil
	}
	c, ok := w.v.decodeComponent(rec)
	if !ok || !w.spend(int64(len(c.axisIndices))) {
		return nil
	}
	show := true
	if c.flags&varcHaveCondition != 0 {
		show = w.v.condition(c.condition, coords)
	}
	if c.flags&varcAxisValuesHaveVariation != 0 && show && len(coords) > 0 {
		if !w.spend(int64(len(c.axisValues)) * int64(len(coords))) {
			return nil
		}
		w.v.delta(c.axisValuesVar, coords, c.axisValues)
	}
	componentCoords := coords
	if c.flags&varcResetUnspecifiedAxes != 0 || len(coords) > hbMaxVarcAxes {
		componentCoords = w.fontCoords
	}
	t := c.transform
	if show {
		if len(c.axisIndices) > 0 {
			if !w.spend(int64(len(componentCoords))) {
				return nil
			}
			componentCoords = setVarcCoords(componentCoords, c.axisIndices, c.axisValues)
		}
		if c.transformVar != varcNoVariation && len(coords) > 0 {
			var present []float32
			for i, f := range varcTransformFields {
				if c.flags&f.flag != 0 {
					present = append(present, t[i])
				}
			}
			if !w.spend(int64(len(present)) * int64(len(coords))) {
				return nil
			}
			w.v.delta(c.transformVar, coords, present)
			k := 0
			for i, f := range varcTransformFields {
				if c.flags&f.flag != 0 {
					t[i] = present[k]
					k++
				}
			}
		}
		for i, f := range varcTransformFields {
			if f.shift != 0 && c.flags&f.flag != 0 {
				t[i] = float32(t[i] * float32(1/float64(int(1)<<f.shift)))
			}
		}
		if c.flags&varcHaveScaleY == 0 {
			t[4] = t[3]
		}
		const pi = float32(math.Pi)
		t[2] = float32(t[2] * pi)
		t[5] = float32(t[5] * pi)
		t[6] = float32(t[6] * pi)
		w.depthLeft--
		w.glyph(c.gid, componentCoords, xf.then(varcTransform(t)), parent)
		w.depthLeft++
	}
	return rec[c.size:]
}

// setVarcCoords is coord_setter_t: the coordinates copied, each axis a
// component sets set to its value rounded as roundf rounds, the list grown
// with zeros to reach an axis past its end, and an axis past 4,095 not set.
func setVarcCoords(coords []int, axes []int, values []float32) []int {
	out := append([]int(nil), coords...)
	for i, a := range axes {
		if a < 0 || a >= hbMaxVarcAxes {
			continue
		}
		for len(out) <= a {
			out = append(out, 0)
		}
		out[a] = int(math.Floor(float64(float32(values[i] + 0.5))))
	}
	return out
}
