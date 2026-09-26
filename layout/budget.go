package layout

// The document's work budget.
//
// Every bound this package had was local. maxContentLength is per
// pseudo-element, the tile cap is per background layer, the pixel budget was
// charged only for a picture that decoded, the counter caps are per
// declaration, and the findings list was bounded while the memo behind it was
// not. Each said how much one piece of a document might cost and nothing about
// what the document as a whole might, and a document is not obliged to have one
// piece: a rule that costs a megabyte once costs a gigabyte on a thousand
// elements, and every one of those bounds held while it did.
//
// So there is one allowance per render, held by the Recorder that render
// reports into — which is the one object every stage of a render is already
// handed, and the one whose lifetime is exactly one render. Build and Compose
// make it; a caller calling Layout or PaintReporting with a recorder of its own
// gives those stages the budget that recorder carries.
//
// # Charged for work attempted
//
// A stage asks before it does the work, and the amount it asks for is what
// the work will cost whether or not it succeeds: a picture's declared pixels
// before it is decoded, the characters of a counters() before they are joined,
// the rectangles of a tiling before one of them is emitted. What is refused is
// not done, and not charged. Asking afterwards is how the pixel budget came to
// charge nothing for a PNG whose header decoded and whose body did not —
// sixty-four megabytes allocated and thrown away, as often as the document
// cared to name it.
//
// # What it is not
//
// It is not the safety argument for ordinary documents, and it must not be the
// thing an ordinary document meets. Every algorithm a document can make
// super-linear is made linear where it stands — see counter.go, content.go,
// finding.go and control.go — and the per-stage caps stay as the early exits
// they were. What is left for this to stop is *amplification*: work linear in
// something a small input can make large, like a counters() whose separator is
// ten kilobytes and whose depth is two hundred, or a gradient tiled at a
// hundredth of a pixel. Those are linear in their output, and their output is
// not bounded by their input, and nothing else here can see the product.
//
// # The unit
//
// A step is about a byte of what the engine makes on the document's behalf:
// the weights below are the rough size of each thing charged, so that the
// allowance is also a statement about memory. The allowance is a floor for
// every document plus a share for each byte of input, the shape HarfBuzz gives
// its own max_ops — a large document legitimately does more work, and a small
// one that does a great deal is what is being looked for.
//
// # A reserve for the document itself
//
// One allowance shared by every stage has a failure of its own: the first
// stage to amplify spends it, and every stage after finds nothing left. Counters
// on every element would take the budget before a single box was built, and
// the page would be blank — which is not "some of the work was left out", it is
// all of it. So a reserve — a quarter of the floor, and most of what the
// input's own size earns — is held back from the work a small input can
// multiply: generated content, counters, pictures, repeated marks, findings.
// Only the work the document's own content costs, one box and one mark at a
// time, may spend it: half of it for building boxes, and the rest for painting
// them. See charge, chargeOwn and chargeMark.
//
// # The other bound: laying the document out again
//
// There is a second allowance per document, and it is not an oversight that it
// is not this one: maxLayoutWork, in speculative.go, bounds how many times the
// layout may lay boxes out and make line boxes. The two measure different
// things at scales that do not meet, and they were reconciled by making them
// agree on where their size comes from and on how a cut is reported, rather than
// by pouring them into one number.
//
// This one is a statement about memory: a step is about a byte of what the
// engine makes and keeps — a box, an op, a pixel, a byte of generated text —
// and its floor is sized for a page that holds a large picture. That one is a
// statement about time spent on work that is thrown away: a speculative pass
// keeps nothing, so its cost is not bytes but passes, and its size is a
// multiple of what one pass over the tree costs, with a floor of 65,536
// layouts. One pool would need one rate between a layout and a step, and there
// is none that serves both:
//
//   - At 4,096 steps a layout, the rate that keeps the layout floor where it is
//     against this floor of 2^28, a byte of input earns a thirty-second of a
//     layout, and one honest pass costs more than that: measured, a table of
//     one-letter cells lays out a third of a box or line per byte of markup, a
//     column of short divs a sixth, a list an eighth. A table past about two
//     hundred kilobytes would run out on its first pass.
//   - Granting the layout's share into this pool instead, so that the ten
//     megabytes get their passes, hands that share to everything else charged
//     here: a document of ten thousand boxes would get two and a half billion
//     more steps for tiling a gradient at a hundredth of a pixel, which is the
//     amplification this budget is for.
//
// What the two must agree on is the rule under "The unit": an allowance grows
// with what the document supplied, and never with what it made. The layout's
// did not. Its "one pass" counted every box and every byte of text in the
// tree, generated content included, and generated content is exactly what this
// budget lets a small input make a great deal of: forty kilobytes of markup
// whose ::before repeated an attribute sixty-four times made two and a half
// megabytes of text, which raised the bound on laying it out from its floor of
// 65,536 layouts to 164 million. It counts what the document wrote now; see
// onePass.
//
// And a cut is reported one way, by refuseAt, whichever allowance ran out: one
// RuleLimit finding per kind of work left out, saying what was left out, placed
// where the first refusal was when there is a place, counted once per refusal,
// and never itself refused for want of budget. The layout's used to be an
// ordinary finding charged to this budget, so a document that had spent both
// could have the finding that said so dropped.
type workBudget struct {
	// left is what may still be spent, and reserve how much of it only the
	// document's own content may spend.
	left, reserve int64
	// cut is each kind of work that was refused, in the order it was first
	// refused, so that each is reported once.
	cut []string
}

// workFloor is the allowance every document has, whatever its size.
//
// 256 Mi steps is a quarter of a gigabyte of what the weights below stand for,
// which is two million painted rectangles or four megabytes of generated text:
// a page holds a few thousand of the one and a few kilobytes of the other.
//
// A variable so that a test can lower it; a bound that is never seen to fire
// is one nobody knows works.
var workFloor int64 = 1 << 28

// workPerInputByte is the allowance added for each byte of markup and
// stylesheet the caller supplied. A book-length document does book-length
// work, and must not meet a floor sized for a page.
//
// Most of it is reserved for that content itself (see grant): a byte of
// markup is at most about a seventh of a box, which is 37 steps, and the marks
// that box paints, and what a document's own markup costs is what the reserve
// is for. The rest is what that input may amplify, beside the floor.
const workPerInputByte = 128

// The weights, in steps.
const (
	// costGeneratedByte is one byte of generated content. It is heavier than a
	// byte because it is not the end of the work: the text is boxed, shaped,
	// broken into lines and painted, and each of those costs per character.
	costGeneratedByte = 64
	// costBox is one box, which is about what a Box holds.
	costBox = 256
	// costCounterValue is one counter value copied or visited on a
	// document's behalf.
	costCounterValue = 8
	// costFetchedByte is one byte of a resource read to be decoded; a decoded
	// pixel costs the same, since it is four bytes held and one touched.
	costFetchedByte = 1
	costPixel       = 1
	// costFindingByte is one byte of a finding the recorder is asked to
	// remember.
	costFindingByte = 1
	// costOp is one operation in the display list, which is about what a
	// FillRect holds once it is in the slice.
	costOp = 128
	// costMarkCompared is one mark an opacity group's faithfulness check
	// reads. See group.settle.
	costMarkCompared = 16
)

// newWorkBudget is the floor.
func newWorkBudget() workBudget {
	return workBudget{left: workFloor, reserve: workFloor / 4}
}

// grant adds the share for some bytes of input: three quarters of it to the
// reserve, so that a document's own boxes are paid for by its own size however
// much of the rest something in it amplified.
func (w *workBudget) grant(inputBytes int) {
	g := satMul(int64(inputBytes), workPerInputByte)
	w.left = satAdd(w.left, g)
	w.reserve = satAdd(w.reserve, g/4*3)
}

// satMul and satAdd keep a charge from wrapping: the numbers multiplied are
// counts out of a document, and a product that wrapped negative would be a
// charge that *added* to the allowance.
func satMul(a, b int64) int64 {
	if a <= 0 || b <= 0 {
		return 0
	}
	if a > (1<<63-1)/b {
		return 1<<63 - 1
	}
	return a * b
}

func satAdd(a, b int64) int64 {
	if b > 0 && a > 1<<63-1-b {
		return 1<<63 - 1
	}
	return a + b
}

// charge asks for steps of work a small input can multiply, and reports
// whether it may be done. When it may not, the work is left undone and the
// document is told what was cut. what names the work in a finding, as the
// thing left out: "the generated content past that point". It is a phrase from
// this package and never from the document, so that one kind of refusal is one
// finding however many times it happens — the count beside it says how many.
//
// A refused charge takes nothing. The budget is for the document, and the
// piece that asked for too much is what is cut: the paragraph after a tiling
// that did not fit is still drawn, which a budget that emptied itself on the
// refusal would not do. And it never takes the reserve; see workBudget.
func (r *Recorder) charge(steps int64, what string) bool {
	if r == nil {
		return true
	}
	return r.take(steps, r.work.reserve, what)
}

// chargeOwn is charge for the work the document's own content costs — a box
// for an element — which may spend half the reserve. The other half is for
// drawing what was built: a tree cut short by its own size is still painted, as
// far as it goes. See chargeMark.
func (r *Recorder) chargeOwn(steps int64, what string) bool {
	if r == nil {
		return true
	}
	return r.take(steps, r.work.reserve/2, what)
}

// chargeMark is charge for a mark a fragment paints for itself, which may spend
// all of what is left: it is the last stage, and nothing after it needs any.
func (r *Recorder) chargeMark(steps int64, what string) bool {
	if r == nil {
		return true
	}
	return r.take(steps, 0, what)
}

// take spends steps if that leaves at least keep, and reports the cut if not.
func (r *Recorder) take(steps, keep int64, what string) bool {
	if steps <= r.work.left-keep {
		if steps > 0 {
			r.work.left -= steps
		}
		return true
	}
	r.refuse(what)
	return false
}

// refuse reports one kind of cut work, once, and counts it every time.
func (r *Recorder) refuse(what string) {
	r.refuseAt(what, NoSource, "", 1)
}

// refuseAt is the one finding either of a document's allowances makes when it
// runs out — this budget, or the layout's (see "The other bound" above): what
// was left out, reported once per kind, first at src and path where the refusal
// had a place, and counted times.
func (r *Recorder) refuseAt(what string, src Source, path string, times int) {
	times = max(times, 1)
	for _, c := range r.work.cut {
		if c == what {
			r.counts[RuleLimit] += times
			return
		}
	}
	r.work.cut = append(r.work.cut, what)
	// Not through ReportDetail's own charge: the budget is what ran out, and
	// the finding saying so is the one thing that must not be refused for it.
	r.record(Finding{
		Rule: RuleLimit, Source: src, Path: path,
		Message: "this document asks for more work than this engine does for one " +
			"document; left out: " + what,
	}, false)
	r.counts[RuleLimit] += times - 1
}

// workLeft is what the budget still holds, for a test.
func (r *Recorder) workLeft() int64 { return r.work.left }
