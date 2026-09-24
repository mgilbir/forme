package layout

import (
	"strconv"
	"strings"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/html"
	"github.com/mgilbir/forme/internal/ascii"
	"github.com/mgilbir/forme/style"
)

// CSS counters, CSS 2.1 §12.4.
//
// A counter is the only piece of state in the cascade. Everything else about an
// element is decided by the element and its ancestors; a counter's value depends
// on how many elements came *before* it in the document, which is why this is a
// walk of its own in document order rather than something the box builder can
// answer while descending.
//
// # Scope, which is the part that is easy to get wrong
//
// "counter-reset" does not set a counter, it *creates* one. The new counter is
// in scope for the element, its descendants, and its following siblings and
// their descendants — and it hides any counter of the same name from an
// enclosing scope rather than overwriting it. That is what makes nested lists
// number independently, and what makes counters() able to produce "2.1.3": the
// three values are three counters of the same name, all alive at once, one per
// level.
//
// So the state is a stack per name rather than a value per name, and each entry
// remembers the depth of the element that created it. Leaving that depth pops
// it. A previous sibling's counter is kept — it was created at the same depth,
// and siblings share a scope — which is the single rule that separates this from
// a plain "reset on enter, restore on exit" and the reason the comparison is on
// depth rather than on identity.

// counterEntry is one counter in one scope.
type counterEntry struct {
	value int
	// depth is the tree depth of the element whose counter-reset created it. It
	// is what decides when the entry leaves scope.
	depth int
	// reversed says the counter counts down: CSS Lists 3's "reversed()", which
	// is what "<ol reversed>" maps to. It is incremented by the *negation* of
	// the increment, so the same "counter-increment: list-item" that numbers a
	// list upwards numbers this one downwards, and no other rule changes.
	reversed bool
	// tally, in the pass that finds where reversed counters start, adds up
	// the increments this counter receives. See reversedStarts.
	tally *reversedTally
}

// counterState is the set of counters visible at a point in the walk.
type counterState struct {
	stacks map[string][]counterEntry
	// opened is every counter created, in the order it was created, with the
	// depth it was created at. See enter, which is what it is for.
	opened []openedCounter
}

// openedCounter is one entry of counterState.opened.
type openedCounter struct {
	name  string
	depth int
}

// counterValues is what a counter() or counters() at an element resolves to.
type counterValues map[string][]int

// maxCounterDepth bounds how many counters of one name may be alive at once.
//
// The stack grows with nesting, and nesting is attacker-controlled. The HTML
// parser already caps element depth, so this cannot be reached through markup
// alone; it is here because the two limits are in different packages and a
// change to one should not silently uncap the other.
const maxCounterDepth = 512

// maxCounterNames bounds how many distinct counters a document may have.
//
// Each name is a map key taken from the document text, so without this a
// stylesheet of a hundred thousand distinct counter-reset names is a hundred
// thousand live maps. The number is far past any real document.
const maxCounterNames = 1024

func newCounterState() *counterState {
	return &counterState{stacks: map[string][]counterEntry{}}
}

// enter drops the counters created below the depth being entered.
//
// Popping on the way *down* rather than on the way back up is what lets a
// following sibling see a counter its predecessor created: that entry sits at
// the same depth, so it survives, while anything created deeper does not.
//
// It pops from the list of counters in the order they were created, and only
// the ones it drops. It used to look at every name in scope, at every element
// and every pseudo-element, which made the walk the document times the names:
// a stylesheet resetting a thousand counters on the body cost a thousand steps
// at each of the twenty thousand elements below it (audit C16). The list is in
// order of depth as well as of creation — every counter is created at the depth
// just entered, and entering pops everything deeper — so what leaves scope is
// always at its end, and each counter is pushed once and popped once.
func (c *counterState) enter(depth int) {
	n := len(c.opened)
	for n > 0 && c.opened[n-1].depth > depth {
		n--
		name := c.opened[n].name
		stack := c.stacks[name]
		// The last entry of its name's stack is this one: the stacks are
		// pushed in the order the list is, and popped from the same end.
		if len(stack) <= 1 {
			delete(c.stacks, name)
		} else {
			c.stacks[name] = stack[:len(stack)-1]
		}
	}
	c.opened = c.opened[:n]
}

// reset creates a counter in the scope of an element at the given depth.
//
// A second counter-reset of the same name at the same depth replaces the first
// rather than nesting inside it — they are the same scope, and two counters
// could not be told apart there.
//
// tally is where the increments this counter receives are added up, in the
// pass that needs them, and nil otherwise. See reversedStarts.
func (c *counterState) reset(name string, value, depth int, reversed bool, tally *reversedTally) {
	stack := c.stacks[name]
	if n := len(stack); n > 0 && stack[n-1].depth == depth {
		stack[n-1].value, stack[n-1].reversed, stack[n-1].tally = value, reversed, tally
		return
	}
	if len(c.stacks) >= maxCounterNames && stack == nil {
		return
	}
	if len(stack) >= maxCounterDepth {
		return
	}
	c.stacks[name] = append(stack, counterEntry{value: value, depth: depth,
		reversed: reversed, tally: tally})
	c.opened = append(c.opened, openedCounter{name: name, depth: depth})
}

// set writes the innermost counter of a name, without creating a scope.
//
// That is the whole difference from reset, and it is the whole reason the
// property exists: "counter-reset: n 3" makes a *new* counter that the
// element's following siblings are inside, and "counter-set: n 3" writes the
// one that is already there, leaving the scope where it was.
//
// A counter that is not in scope is instantiated at zero before being written,
// which css-lists-3 requires in as many words and which makes this the same
// answer as a reset for an element that had none — the two only differ where
// there was a counter to keep.
func (c *counterState) set(name string, value, depth int) {
	stack := c.stacks[name]
	if len(stack) == 0 {
		c.reset(name, value, depth, false, nil)
		return
	}
	stack[len(stack)-1].value = value
}

// increment adds to the innermost counter of a name.
//
// A counter that is not in scope is created on the spot with value zero before
// being incremented, which is what §12.4.3 requires — and is why "li {
// counter-increment: item }" numbers a list even with no counter-reset anywhere.
func (c *counterState) increment(name string, by, depth int) {
	stack := c.stacks[name]
	if len(stack) == 0 {
		c.reset(name, 0, depth, false, nil)
		stack = c.stacks[name]
		if len(stack) == 0 {
			// Refused by a cap.
			return
		}
	}
	n := len(stack) - 1
	if t := stack[n].tally; t != nil && by != 0 {
		// §4.4.2's sum, and the last increment once more; see reversedStarts.
		// The increment as written, before a reversed counter negates it.
		t.sum += int64(by)
		t.last = by
	}
	if stack[n].reversed {
		// A reversed counter is incremented by the negation of the increment,
		// which is the whole of what makes it count down. Everything else about
		// it — the scope, the stack, the saturation below — is a counter.
		by = -by
	}
	// Saturating, because the increment is a number out of the document and a
	// stylesheet asking for two billion twice should not wrap to a negative
	// count.
	stack[n].value = counterInt(float64(int64(stack[n].value) + int64(by)))
}

// counterInt is a number saturated to the range a counter holds, a 32-bit
// integer: the range every value a counter takes is kept in, whether it came
// from a reset, a set or an increment, so that no step can move a counter the
// wrong way by being clamped where another was not.
func counterInt(v float64) int {
	switch {
	case v != v:
		return 0
	case v >= 1<<31-1:
		return 1<<31 - 1
	case v <= -(1 << 31):
		return -(1 << 31)
	}
	return int(v)
}

// snapshotOf records the values a content value will read, outermost first,
// and reports how many it copied.
//
// Only the counters it names, and of those only the innermost unless it asks
// for the chain with counters(). It used to be every value of every counter in
// scope, for every pseudo-element that generates a box: a map and a slice per
// counter name, kept until the box was built, for a content value that nearly
// always reads one number or none. A thousand counters reset on the body and
// an empty ::before on each of twenty thousand elements allocated 2.6 GB
// (audit C16).
func (c *counterState) snapshotOf(refs []counterRef) (counterValues, int) {
	if len(refs) == 0 {
		return nil, 0
	}
	out := make(counterValues, len(refs))
	copied := 0
	for _, r := range refs {
		stack := c.stacks[r.name]
		if len(stack) == 0 {
			continue
		}
		if !r.chain {
			stack = stack[len(stack)-1:]
		}
		vals := make([]int, len(stack))
		for i, e := range stack {
			vals[i] = e.value
		}
		out[r.name] = vals
		copied += len(vals)
	}
	return out, copied
}

// counterRef is a counter a content value names: its name, and whether it
// asks for every level with counters() or only the innermost with counter().
type counterRef struct {
	name  string
	chain bool
}

// contentPlan is what the counter walk needs from a content value: the
// counters it reads and the quote keywords it moves the depth by, in order.
type contentPlan struct {
	refs   []counterRef
	quotes []quoteOp
}

// planContent reads a content value for the counter walk. The value is the
// stylesheet's and is shared by every element it applies to, so the walk reads
// each distinct one once. See computeCounters.
func planContent(raw string) contentPlan {
	trimmed := ascii.TrimCSSSpace(raw)
	switch ascii.Lower(trimmed) {
	case "", "normal", "none":
		return contentPlan{}
	}
	vals, _ := css.ParseComponentValues(trimmed)
	var p contentPlan
	at := map[string]int{}
	for _, v := range vals {
		switch {
		case v.IsFunction() && (ascii.EqualFold(v.Token.Value, "counter") ||
			ascii.EqualFold(v.Token.Value, "counters")):
			name, _, _, ok := counterArguments(v)
			if !ok {
				continue
			}
			chain := ascii.EqualFold(v.Token.Value, "counters")
			if i, seen := at[name]; seen {
				p.refs[i].chain = p.refs[i].chain || chain
				continue
			}
			at[name] = len(p.refs)
			p.refs = append(p.refs, counterRef{name: name, chain: chain})
		case v.IsToken() && v.Token.Kind == css.Ident:
			if op, isQuote := quoteKeyword(v.Token.Value); isQuote {
				p.quotes = append(p.quotes, op)
			}
		}
	}
	return p
}

// innermost is the value of one counter in the nearest scope holding it, and
// whether there is one at all.
//
// It is what a list item's marker reads, and reading it alone is what keeps the
// element side of the snapshots to an integer per list item rather than to
// every counter in the document per element.
func (c *counterState) innermost(name string) (int, bool) {
	stack := c.stacks[name]
	if len(stack) == 0 {
		return 0, false
	}
	return stack[len(stack)-1].value, true
}

// counterSnapshots is what every box that can name a counter sees.
//
// Elements and pseudo-elements are kept apart because they see different things:
// a ::before that resets a counter of its own must show its own value in its
// content, and the element it hangs from must not — the pseudo-element is a
// child of the element, so its scope is nested inside.
type counterSnapshots struct {
	// elements holds the "list-item" counter of every element that has one in
	// scope, which is the only counter an element itself can name: its marker's
	// number. Everything else a document says about a counter is said in a
	// "content" declaration, and content belongs to a pseudo-element.
	//
	// It used to be every counter of every element, taken with the same
	// snapshot the pseudo-elements get — a map and a slice per counter name per
	// element, for one integer that a list item might read. Two thousand
	// elements under a stylesheet naming a thousand counters came to 229 MB of
	// snapshots, and a document need not have a single list in it.
	elements map[*html.Node]int
	pseudo   map[style.PseudoKey]counterValues
	// quoteDepth is the level of quotation nesting each pseudo-element's content
	// begins at. It rides along with the counters because it is the same kind of
	// value — see quotes.go — and because threading it through a second walk of
	// the same tree in the same order would be two chances to disagree about
	// document order rather than one.
	quoteDepth map[style.PseudoKey]int
	// cut holds the pseudo-elements whose counters were not snapshotted
	// because the work budget refused them, and which therefore generate
	// nothing. It is nil for every document that was not cut short.
	cut map[style.PseudoKey]bool
}

// computeCounters walks the document and records what each box sees.
//
// The value an element sees is the one *after* its own resets and increments,
// because that is what its marker uses: "li { counter-increment: item }" must
// show the item's own number, not the previous one's.
//
// # A pseudo-element is a child, not a second helping of its element
//
// §12.4.1's scope is a tree scope, and ::before and ::after are in the tree: they
// are the element's first and last children. Applying their counter-reset in the
// *element's* scope instead — which is what this did — is wrong in a way that
// only shows when both carry a counter of the same name, and then it is wrong
// loudly. "html { counter-reset: c 0 c 4 c 0; counter-increment: c 1 c 3 }" with
// "html::before { counter-reset: c 9999; counter-increment: c 9999 }" must leave
// html's counter at 4, because the 9999 is a *nested* counter that dies with the
// pseudo-element; sharing the scope made the reset overwrite html's own and the
// document numbered from 19998.
//
// Document order decides the rest: ::before applies before the element's
// children and ::after after them, so a counter either of them creates is in
// scope for its following siblings exactly as any other child's would be.
//
// # What does not count
//
// §12.4.1 excludes two things and both of them are the difference between a rule
// that reads right and a document that numbers wrong:
//
//   - An element with "display: none" cannot reset or increment a counter, and
//     neither can anything inside it. It is not in the formatting structure at
//     all, so there is nothing to number.
//   - A pseudo-element that generates no box does not either, and "generates no
//     box" is decided by its content: the initial value is "normal", so *every*
//     element in every document has a ::before rule's worth of declarations
//     sitting on it that must do nothing. A stylesheet saying only
//     "#one::before { counter-increment: c }" increments nothing at all.
//
// "visibility: hidden" is deliberately not here. A hidden box is laid out and
// takes its room; only display removes it.
//
// # What it costs
//
// Every counter is pushed once and popped once (see counterState.enter), each
// pseudo-element copies only the values its content reads (see snapshotOf),
// and each distinct counter list and content value is read once for the
// document rather than once per element it applies to — they are the
// stylesheet's, and a thousand-name counter-reset on "*" was being parsed at
// every element. What the walk does at an element is then what the element's
// declarations ask for, and that is charged to the document's work budget; a
// document that asks for more stops being counted, and says so, rather than
// being counted for as long as it likes.
func computeCounters(root *html.Node, styles map[*html.Node]style.ComputedStyle,
	pseudo map[style.PseudoKey]style.ComputedStyle, rec *Recorder) counterSnapshots {

	out := counterSnapshots{
		elements:   map[*html.Node]int{},
		pseudo:     map[style.PseudoKey]counterValues{},
		quoteDepth: map[style.PseudoKey]int{},
	}
	w := newCounterWalk(styles, pseudo, rec)
	w.starts = reversedStarts(root, styles, pseudo, rec)
	// §12.3.1's level of quotation, which runs across the whole document in
	// document order and belongs to no element.
	depthOfQuotes := 0
	w.atElement = func(n *html.Node) {
		if v, ok := w.state.innermost("list-item"); ok {
			out.elements[n] = v
		}
	}
	w.atPseudo = func(key style.PseudoKey, cs style.ComputedStyle, plan contentPlan) {
		values, copied := w.state.snapshotOf(plan.refs)
		if !w.rec.charge(int64(copied+len(plan.quotes))*costCounterValue,
			"the counters past that point") {
			w.stop()
		}
		if w.stopped {
			// Not snapshotted, so not generated: see boxBuilder.generated.
			if out.cut == nil {
				out.cut = map[style.PseudoKey]bool{}
			}
			out.cut[key] = true
			return
		}
		out.pseudo[key] = values
		// The depth this pseudo-element's content *starts* at, recorded before
		// its own keywords move it: "content: open-quote" draws the mark for the
		// level it is opening, not for the one it leaves behind.
		out.quoteDepth[key] = depthOfQuotes
		if len(plan.quotes) > 0 {
			quotes := parseQuotes(cs.Get("quotes"))
			for _, op := range plan.quotes {
				_, depthOfQuotes = applyQuote(op, depthOfQuotes, quotes)
			}
		}
	}
	w.walk(root, 0)
	return out
}

// counterWalk is one walk of the document in the order counters are applied.
//
// There are two, and they are one walk so that they cannot disagree about the
// order: the one that numbers, and before it — only when a document has a
// reversed counter with no number — the one that finds where those begin. See
// reversedStarts.
type counterWalk struct {
	styles map[*html.Node]style.ComputedStyle
	pseudo map[style.PseudoKey]style.ComputedStyle
	rec    *Recorder
	state  *counterState

	// starts is where each reversed counter with no number begins, in the
	// walk that numbers. tallies is where the increments that decide it are
	// added up, in the walk that finds it. At most one of the two is set.
	starts  map[counterStart]int
	tallies map[counterStart]*reversedTally

	// lists and plans are the counter lists and content values already read.
	// They are the stylesheet's, so there are as many as it wrote, and each is
	// read once however many elements it applies to.
	lists map[string][]counterRequest
	plans map[string]contentPlan

	// stopped says the work budget refused the walk, which applies no counter
	// from then on. See stop.
	stopped bool

	atElement func(n *html.Node)
	atPseudo  func(key style.PseudoKey, cs style.ComputedStyle, plan contentPlan)
}

// counterStart names one reversed counter created with no number: the box
// that created it, and its name.
type counterStart struct {
	box  style.PseudoKey
	name string
}

// reversedTally is the increments one reversed counter received, which is what
// its start is made of. See reversedStarts.
type reversedTally struct {
	sum  int64
	last int
}

func newCounterWalk(styles map[*html.Node]style.ComputedStyle,
	pseudo map[style.PseudoKey]style.ComputedStyle, rec *Recorder) *counterWalk {

	return &counterWalk{
		styles: styles, pseudo: pseudo, rec: rec, state: newCounterState(),
		lists: map[string][]counterRequest{}, plans: map[string]contentPlan{},
	}
}

// stop ends the counting, once. A counter applied after some were refused
// would be a number that looks right and is not; none at all is a gap the
// budget's finding explains.
func (w *counterWalk) stop() { w.stopped = true }

// list is parseCounterList, read once per distinct value.
func (w *counterWalk) list(raw string, byDefault int) []counterRequest {
	if raw == "" {
		return nil
	}
	key := raw
	if byDefault != 0 {
		// The default is part of the reading: "c" is c 0 as a reset and c 1
		// as an increment. A byte no value holds keeps the two apart.
		key = "\x00" + raw
	}
	if got, ok := w.lists[key]; ok {
		return got
	}
	got := parseCounterList(raw, byDefault)
	w.lists[key] = got
	return got
}

// plan is planContent, read once per distinct value.
func (w *counterWalk) plan(raw string) contentPlan {
	if got, ok := w.plans[raw]; ok {
		return got
	}
	got := planContent(raw)
	w.plans[raw] = got
	return got
}

// apply runs one box's declarations in one scope. Reset before increment:
// "counter-reset: n 0; counter-increment: n" on one element yields 1, and the
// specification fixes the order rather than leaving it to the declaration
// order.
func (w *counterWalk) apply(key style.PseudoKey, cs style.ComputedStyle, depth int) {
	if w.stopped {
		return
	}
	resets := w.list(cs.Get("counter-reset"), 0)
	increments := w.list(cs.Get("counter-increment"), 1)
	sets := w.list(cs.Get("counter-set"), 0)
	if n := len(resets) + len(increments) + len(sets); n > 0 &&
		!w.rec.charge(int64(n)*costCounterValue, "the counters past that point") {
		w.stop()
		return
	}
	for _, r := range resets {
		value := r.value
		var tally *reversedTally
		if r.reversed && r.implied {
			// A reversed counter with no number begins at the number of
			// things in its scope that increment it, which for a list is the
			// count of its items. That is the one thing about a counter that
			// cannot be read off the element: it is ahead of the walk.
			at := counterStart{box: key, name: r.name}
			if w.tallies != nil {
				tally = &reversedTally{}
				w.tallies[at] = tally
			} else {
				value = w.starts[at]
			}
		}
		w.state.reset(r.name, value, depth, r.reversed, tally)
	}
	for _, r := range increments {
		w.state.increment(r.name, r.value, depth)
	}
	// After the increment, which css-lists-3 §4.3 calls a deliberate
	// choice: "<li value=3>" is three on an element whose own increment has
	// already run, and it would be four the other way round.
	for _, r := range sets {
		w.state.set(r.name, r.value, depth)
	}
}

// walk visits a node and what is inside it, in document order.
func (w *counterWalk) walk(n *html.Node, depth int) {
	if n.Type == html.ElementNode {
		cs := w.styles[n]
		if displayIsNone(cs) {
			return
		}
		w.state.enter(depth)
		w.apply(style.PseudoKey{Node: n}, cs, depth)
		if w.atElement != nil && !w.stopped {
			w.atElement(n)
		}
		w.visitPseudo(n, "before", depth+1)
	}
	// The same bound box generation is under, and for the same reason:
	// this runs first, over the document rather than over the boxes, and
	// recurses once per level. A branch box.go will not build is a branch
	// whose counters nothing reads, so stopping here changes no answer that
	// survives — and leaving it unbounded left the whole cap doing nothing,
	// since this walk is what a deep tree reaches first.
	// depth is the level this node sits on, counted from zero, so the
	// deepest level box generation will build is maxBoxDepth-1 and the
	// children below it are the ones it stops at.
	if depth+1 < maxBoxDepth {
		for _, child := range n.Children {
			w.walk(child, depth+1)
		}
	}
	if n.Type == html.ElementNode {
		w.visitPseudo(n, "after", depth+1)
	}
}

// visitPseudo applies one pseudo-element's declarations in a scope of its own
// and hands it to atPseudo, which records what its content() will see.
func (w *counterWalk) visitPseudo(n *html.Node, name string, depth int) {
	key := style.PseudoKey{Node: n, Name: name}
	cs, ok := w.pseudo[key]
	if !ok || !generatesPseudoBox(cs) {
		return
	}
	w.state.enter(depth)
	w.apply(key, cs, depth)
	if w.atPseudo != nil {
		w.atPseudo(key, cs, w.plan(cs.Get("content")))
	}
}

// displayIsNone reports the one display value that takes a box out of the
// formatting structure rather than merely changing its shape.
func displayIsNone(cs style.ComputedStyle) bool {
	outer, _, _ := displayOf(cs)
	return outer == OuterNone
}

// generatesPseudoBox reports whether a ::before or ::after produces a box, which
// is what decides whether its counters happen at all.
//
// The content value is read here rather than through resolveContent because the
// question is asked before any counter has a value — and it can be, since the
// three values that mean "no box" are the three that need nothing resolved.
func generatesPseudoBox(cs style.ComputedStyle) bool {
	if displayIsNone(cs) {
		return false
	}
	switch ascii.Lower(ascii.TrimCSSSpace(cs.Get("content"))) {
	case "", "normal", "none":
		return false
	}
	return true
}

// counterRequest is one name-and-number pair from a counter-reset or
// counter-increment.
type counterRequest struct {
	name  string
	value int
	// reversed is CSS Lists 3's "reversed(name)", and implied says the
	// declaration named no number — which for a reversed counter is not the
	// same as naming zero: it begins at the count of the things in its scope
	// that increment it. See reversedStarts.
	reversed bool
	implied  bool
}

// parseCounterList reads "chapter section 2 note -1".
//
// A name may be followed by a number; when it is not, the default applies — zero
// for a reset and one for an increment, which is why the caller passes it.
func parseCounterList(raw string, byDefault int) []counterRequest {
	raw = ascii.TrimCSSSpace(raw)
	if raw == "" || ascii.EqualFold(raw, "none") {
		return nil
	}
	vals, _ := css.ParseComponentValues(raw)
	var out []counterRequest
	for _, v := range vals {
		if v.IsFunction() && ascii.EqualFold(v.Token.Value, "reversed") {
			// CSS Lists 3's reversed(): one name, and the counter it creates
			// counts down. A number may follow the function exactly as it may
			// follow a bare name, which is what the Number case below handles.
			name, ok := singleIdent(v.Values)
			if !ok {
				return nil
			}
			if len(out) >= maxCounterNames {
				return out
			}
			out = append(out, counterRequest{
				name: name, value: byDefault, reversed: true, implied: true,
			})
			continue
		}
		if !v.IsToken() {
			continue
		}
		switch v.Token.Kind {
		case css.Whitespace:
		case css.Ident:
			if len(out) >= maxCounterNames {
				return out
			}
			out = append(out, counterRequest{
				name: v.Token.Value, value: byDefault, implied: true,
			})
		case css.Number:
			// The number belongs to the name before it. One with no name before
			// it is a malformed declaration, and dropping it is what the
			// specification's grammar does.
			//
			// An <integer>, and one that fits in a counter. A number that is
			// not an integer makes the declaration invalid. The cascade's
			// grammar refuses it before it gets here on every path found, and
			// this reader refuses it too rather than rely on that: the list
			// is then at its initial value, "none". A number too large for a counter is saturated to the
			// range, the same range increment saturates to, and not converted
			// as it stands: Go's conversion of a float out of int's range is
			// implementation-defined ("counter-reset: c 1e30" read as the
			// smallest int64), and a value outside the counter's range was
			// lowered by the first increment it met, down to 2147483647.
			if !v.Token.IsInteger {
				return nil
			}
			if len(out) > 0 {
				out[len(out)-1].value = counterInt(v.Token.Number)
				out[len(out)-1].implied = false
			}
		default:
			// Anything else makes the declaration invalid.
			return nil
		}
	}
	return out
}

// reversedStarts is the implied initial value of every reversed counter in the
// document: for each box that creates one with no number, §4.4.2's count of the
// increments in that counter's scope — their sum, and the last of them once
// more, so that three items incrementing by one begin at four and three
// incrementing by two begin at eight.
//
// It is a pass of its own because it is the one thing about a counter that
// cannot be read off the element as the walk reaches it. A counter's scope is
// the element, its descendants *and its following siblings with theirs*, so the
// number is ahead of the walk in both directions at once.
//
// # The walk that numbers, run once ahead
//
// The increments in a counter's scope are exactly the increments the numbering
// walk applies to that counter: an increment goes to the innermost counter of
// its name, which is this one everywhere in its scope except inside a nested
// scope of the same name — and those are the increments §4.4.2 leaves out. So
// the pass is that same walk, with each reversed counter adding up what it
// receives instead of being numbered, and it cannot disagree with the
// numbering about what a scope is, because it is the numbering's own idea of
// one.
//
// It used to be a walk of its own, carrying a vector of every reversed
// counter's name through every node and a map of all of them for every element
// (audit C17): a thousand reversed() names over ten thousand elements
// allocated a gigabyte, and the set of names was every name in every rule, with
// no bound. This costs what the numbering does.
//
// Two answers differ from that walk's, and both were its mistakes. A
// pseudo-element that resets the counter opens a scope of its own, which the
// element's increments are not in; and a reversed counter created by a
// pseudo-element is counted, where it began at nought.
//
// §4.4.2 has a case this does not: an element that *sets* the counter stops the
// count there, "add that integer value to num and break this loop". It is left
// out because the sum over the whole scope is the same answer for the shape a
// document actually writes — a reversed list whose items all increment, with a
// value on one of them — and the case where the two differ is a counter-set
// early in a long reversed list, which is a number nobody has asked this engine
// for. It is named here rather than left to be discovered.
//
// Nothing is walked for a document with no reversed counter in it, which is
// nearly all of them: the styles are scanned for the word first.
func reversedStarts(root *html.Node, styles map[*html.Node]style.ComputedStyle,
	pseudo map[style.PseudoKey]style.ComputedStyle, rec *Recorder) map[counterStart]int {

	if !createsReversedCounters(styles, pseudo) {
		return nil
	}
	w := newCounterWalk(styles, pseudo, rec)
	w.tallies = map[counterStart]*reversedTally{}
	w.walk(root, 0)
	out := make(map[counterStart]int, len(w.tallies))
	for at, t := range w.tallies {
		out[at] = int(t.sum + int64(t.last))
	}
	return out
}

// createsReversedCounters reports whether any box resets a counter with
// reversed(), which is the only thing that makes reversedStarts worth a walk.
func createsReversedCounters(styles map[*html.Node]style.ComputedStyle,
	pseudo map[style.PseudoKey]style.ComputedStyle) bool {

	for _, cs := range styles {
		if ascii.ContainsFold(cs.Get("counter-reset"), "reversed(") {
			return true
		}
	}
	for _, cs := range pseudo {
		if ascii.ContainsFold(cs.Get("counter-reset"), "reversed(") {
			return true
		}
	}
	return false
}

// singleIdent is the one name a reversed() takes.
func singleIdent(vals []css.ComponentValue) (string, bool) {
	name := ""
	for _, v := range vals {
		if !v.IsToken() {
			return "", false
		}
		switch v.Token.Kind {
		case css.Whitespace:
		case css.Ident:
			if name != "" {
				return "", false
			}
			name = v.Token.Value
		default:
			return "", false
		}
	}
	return name, name != ""
}

// formatCounter renders one counter value in a list style.
//
// It shares markerText with list markers deliberately: "counter(n, upper-roman)"
// and "list-style-type: upper-roman" are the same numbering in the
// specification, and two implementations of it would drift.
func formatCounter(value int, listStyle string) string {
	if listStyle == "" {
		listStyle = "decimal"
	}
	if ascii.EqualFold(ascii.TrimCSSSpace(listStyle), "none") {
		return ""
	}
	if text := markerText(listStyle, value); text != "" {
		return strings.TrimSuffix(text, ".")
	}
	return strconv.Itoa(value)
}
