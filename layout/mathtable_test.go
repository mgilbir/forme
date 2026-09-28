package layout

import "testing"

// Tables: MathML Core §3.5. An <mtable> is a CSS table — its rows and cells
// laid out by the table algorithm — whose middle is on the math axis; each
// <mtd> lays its children out as a row.

// TestATableIsCentredOnTheAxis: a table of two rows of "1" is its cells' height
// with their padding (§3.5.3's 0.5ex above and below each, in this face's
// x-height), and its middle is 256 above the formula's baseline — where the
// fraction bar and the minus sign are.
func TestATableIsCentredOnTheAxis(t *testing.T) {
	root, findings := mathLayoutIn(t, mathFaceWith(t, nil), `<math id="m"><mn id="n">1</mn>`+
		`<mtable id="t"><mtr><mtd><mn>1</mn></mtd></mtr><mtr><mtd><mn>1</mn></mtd></mtr></mtable></math>`)
	for _, f := range findings {
		t.Errorf("finding: %v", f)
	}
	tb := find(t, root, "t")
	m := find(t, root, "m")
	middle := tb.BorderRect.Y.Add(tb.BorderRect.H.Div(2))
	baseline := m.ContentRect().Y.Add(m.mathBaseline)
	if baseline.Sub(middle) != 256 {
		t.Errorf("the table's middle is %d above the baseline, want 256", baseline.Sub(middle))
	}
}

// TestACellIsARow: two tokens in one <mtd> sit side by side, as in a row, and
// not one over the other as two blocks in a cell would.
func TestACellIsARow(t *testing.T) {
	root, _ := mathLayoutIn(t, mathFaceWith(t, nil), `<math><mtable><mtr><mtd>`+
		`<mn id="a">1</mn><mo id="p">+</mo><mn id="b">2</mn></mtd></mtr></mtable></math>`)
	a, p, b := find(t, root, "a"), find(t, root, "p"), find(t, root, "b")
	if a.BorderRect.Y.Add(a.mathBaseline) != b.BorderRect.Y.Add(b.mathBaseline) {
		t.Error("the cell's tokens are not on one baseline")
	}
	// "+" is an infix operator between the two, spaced 227 either side.
	if p.BorderRect.X.Sub(a.BorderRect.X) != 512+227 || b.BorderRect.X.Sub(p.BorderRect.X) != 768+227 {
		t.Errorf("the cell's children are at %d, %d, %d", a.BorderRect.X, p.BorderRect.X, b.BorderRect.X)
	}
}

// TestAColumnspanSpansColumns: §3.5.3's columnspan is HTML's colspan.
func TestAColumnspanSpansColumns(t *testing.T) {
	root, _ := mathLayoutIn(t, mathFaceWith(t, nil), `<math><mtable>`+
		`<mtr><mtd id="w" columnspan="2"><mn>1</mn></mtd></mtr>`+
		`<mtr><mtd id="a"><mn>11</mn></mtd><mtd id="b"><mn>11</mn></mtd></mtr></mtable></math>`)
	w, a, b := find(t, root, "w"), find(t, root, "a"), find(t, root, "b")
	if w.BorderRect.W != b.BorderRect.X.Add(b.BorderRect.W).Sub(a.BorderRect.X) {
		t.Errorf("the spanning cell is %d wide, the two below %d", w.BorderRect.W, b.BorderRect.X.Add(b.BorderRect.W).Sub(a.BorderRect.X))
	}
}

// TestAnMtableWithDisplayMathIsATableCentredOnTheAxis: §4.1, an <mtable>
// whose display is "inline math" is a table all the same, centred as one.
func TestAnMtableWithDisplayMathIsATableCentredOnTheAxis(t *testing.T) {
	root, _ := mathLayoutIn(t, mathFaceWith(t, nil), `<math id="m"><mtable id="t" style="display: inline math">`+
		`<mtr><mtd><mn>1</mn></mtd></mtr></mtable></math>`)
	tb, m := find(t, root, "t"), find(t, root, "m")
	middle := tb.BorderRect.Y.Add(tb.BorderRect.H.Div(2))
	if m.ContentRect().Y.Add(m.mathBaseline).Sub(middle) != 256 {
		t.Errorf("the table's middle is %d above the baseline, want 256", m.ContentRect().Y.Add(m.mathBaseline).Sub(middle))
	}
}

// TestTextInACellIsReportedAsInARow: a cell's children are a row's, and text
// written straight into one is dropped and reported naming the cell.
func TestTextInACellIsReportedAsInARow(t *testing.T) {
	_, findings := mathLayoutIn(t, mathFaceWith(t, nil), `<math><mtable><mtr><mtd>x<mn>1</mn></mtd></mtr></mtable></math>`)
	if !mathFinding(findings, RuleInvalidMarkup, "inside <mtd>") {
		t.Errorf("text in a cell is not reported: %v", findings)
	}
}

// TestAParenthesisCoversATable: the brackets of a matrix are stretched to the
// table, which is ink from its top to its bottom.
func TestAParenthesisCoversATable(t *testing.T) {
	root, _, _ := mathComposed(t, mathStretchFace(t, nil), `<math><mo id="p">(</mo><mtable id="t">`+
		`<mtr><mtd><mn>1</mn></mtd></mtr><mtr><mtd><mn>1</mn></mtd></mtr><mtr><mtd><mn>1</mn></mtd></mtr></mtable></math>`)
	p, tb := find(t, root, "p"), find(t, root, "t")
	if p.BorderRect.H < tb.BorderRect.H {
		t.Errorf("the parenthesis is %d tall beside a table %d tall", p.BorderRect.H, tb.BorderRect.H)
	}
}

// TestOnlyAnMtableIsCentredOnTheAxis: another MathML element an author makes
// a table is a CSS box in the formula, on its first baseline.
func TestOnlyAnMtableIsCentredOnTheAxis(t *testing.T) {
	root, _ := mathLayoutIn(t, mathFaceWith(t, nil), `<math id="m"><mn id="n">1</mn>`+
		`<mrow id="r" style="display: inline-table"><mtr><mtd><mn id="a">1</mn></mtd></mtr>`+
		`<mtr><mtd><mn>1</mn></mtd></mtr></mrow></math>`)
	n, a := find(t, root, "n"), find(t, root, "a")
	if got, want := a.BorderRect.Y.Add(a.mathBaseline), n.BorderRect.Y.Add(n.mathBaseline); got != want {
		t.Errorf("the table's first row is on %d, the formula's baseline %d", got, want)
	}
}
