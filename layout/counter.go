package layout

import (
	"sort"
	"strconv"
	"strings"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/html"
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
}

// counterState is the set of counters visible at a point in the walk.
type counterState struct {
	stacks map[string][]counterEntry
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
func (c *counterState) enter(depth int) {
	for name, stack := range c.stacks {
		n := len(stack)
		for n > 0 && stack[n-1].depth > depth {
			n--
		}
		if n == 0 {
			delete(c.stacks, name)
			continue
		}
		c.stacks[name] = stack[:n]
	}
}

// reset creates a counter in the scope of an element at the given depth.
//
// A second counter-reset of the same name at the same depth replaces the first
// rather than nesting inside it — they are the same scope, and two counters
// could not be told apart there.
func (c *counterState) reset(name string, value, depth int, reversed bool) {
	stack := c.stacks[name]
	if n := len(stack); n > 0 && stack[n-1].depth == depth {
		stack[n-1].value, stack[n-1].reversed = value, reversed
		return
	}
	if len(c.stacks) >= maxCounterNames && stack == nil {
		return
	}
	if len(stack) >= maxCounterDepth {
		return
	}
	c.stacks[name] = append(stack, counterEntry{value: value, depth: depth, reversed: reversed})
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
		c.reset(name, value, depth, false)
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
		c.reset(name, 0, depth, false)
		stack = c.stacks[name]
		if len(stack) == 0 {
			// Refused by a cap.
			return
		}
	}
	n := len(stack) - 1
	if stack[n].reversed {
		// A reversed counter is incremented by the negation of the increment,
		// which is the whole of what makes it count down. Everything else about
		// it — the scope, the stack, the saturation below — is a counter.
		by = -by
	}
	// Saturating, because the increment is a number out of the document and a
	// stylesheet asking for two billion twice should not wrap to a negative
	// count.
	sum := int64(stack[n].value) + int64(by)
	switch {
	case sum > 1<<31-1:
		stack[n].value = 1<<31 - 1
	case sum < -(1 << 31):
		stack[n].value = -(1 << 31)
	default:
		stack[n].value = int(sum)
	}
}

// snapshot records every value of every counter in scope, outermost first.
//
// All of them, rather than the innermost, because counters() needs the whole
// chain — "2.1.3" is one counter name at three levels.
func (c *counterState) snapshot() counterValues {
	if len(c.stacks) == 0 {
		return nil
	}
	out := make(counterValues, len(c.stacks))
	for name, stack := range c.stacks {
		vals := make([]int, len(stack))
		for i, e := range stack {
			vals[i] = e.value
		}
		out[name] = vals
	}
	return out
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
func computeCounters(root *html.Node, styles map[*html.Node]style.ComputedStyle,
	pseudo map[style.PseudoKey]style.ComputedStyle) counterSnapshots {

	out := counterSnapshots{
		elements:   map[*html.Node]int{},
		pseudo:     map[style.PseudoKey]counterValues{},
		quoteDepth: map[style.PseudoKey]int{},
	}
	state := newCounterState()
	// §12.3.1's level of quotation, which runs across the whole document in
	// document order and belongs to no element.
	depthOfQuotes := 0

	// apply runs one box's declarations in one scope. Reset before increment:
	// "counter-reset: n 0; counter-increment: n" on one element yields 1, and the
	// specification fixes the order rather than leaving it to the declaration
	// order.
	starts := reversedStarts(root, styles, pseudo)
	apply := func(n *html.Node, cs style.ComputedStyle, depth int) {
		for _, r := range parseCounterList(cs.Get("counter-reset"), 0) {
			value := r.value
			if r.reversed && r.implied {
				// A reversed counter with no number begins at the number of
				// things in its scope that increment it, which for a list is
				// the count of its items. That is the one thing about a counter
				// that cannot be read off the element: it is ahead of the walk.
				value = starts[n][r.name]
			}
			state.reset(r.name, value, depth, r.reversed)
		}
		for _, r := range parseCounterList(cs.Get("counter-increment"), 1) {
			state.increment(r.name, r.value, depth)
		}
		// After the increment, which css-lists-3 §4.3 calls a deliberate
		// choice: "<li value=3>" is three on an element whose own increment has
		// already run, and it would be four the other way round.
		for _, r := range parseCounterList(cs.Get("counter-set"), 0) {
			state.set(r.name, r.value, depth)
		}
	}
	// atPseudo applies one pseudo-element's declarations in a scope of its own
	// and records what its content() will see.
	atPseudo := func(n *html.Node, name string, depth int) {
		key := style.PseudoKey{Node: n, Name: name}
		cs, ok := pseudo[key]
		if !ok || !generatesPseudoBox(cs) {
			return
		}
		state.enter(depth)
		apply(n, cs, depth)
		out.pseudo[key] = state.snapshot()
		// The depth this pseudo-element's content *starts* at, recorded before
		// its own keywords move it: "content: open-quote" draws the mark for the
		// level it is opening, not for the one it leaves behind.
		out.quoteDepth[key] = depthOfQuotes
		depthOfQuotes = quoteDepthAfter(cs.Get("content"), depthOfQuotes, parseQuotes(cs.Get("quotes")))
	}

	var walk func(n *html.Node, depth int)
	walk = func(n *html.Node, depth int) {
		if n.Type == html.ElementNode {
			cs := styles[n]
			if displayIsNone(cs) {
				return
			}
			state.enter(depth)
			apply(n, cs, depth)
			if v, ok := state.innermost("list-item"); ok {
				out.elements[n] = v
			}
			atPseudo(n, "before", depth+1)
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
				walk(child, depth+1)
			}
		}
		if n.Type == html.ElementNode {
			atPseudo(n, "after", depth+1)
		}
	}
	walk(root, 0)
	return out
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
	switch strings.ToLower(strings.TrimSpace(cs.Get("content"))) {
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
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "none") {
		return nil
	}
	vals, _ := css.ParseComponentValues(raw)
	var out []counterRequest
	for _, v := range vals {
		if v.IsFunction() && strings.EqualFold(v.Token.Value, "reversed") {
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
			if len(out) > 0 {
				out[len(out)-1].value = int(v.Token.Number)
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
// document: for each element that creates one with no number, the count of the
// things in that counter's scope that increment it.
//
// It is a pass of its own because it is the one thing about a counter that
// cannot be read off the element as the walk reaches it. A counter's scope is
// the element, its descendants *and its following siblings with theirs*, so the
// number is ahead of the walk in both directions at once.
//
// One pass answers it for every element at once, which is what keeps it from
// being a look-ahead per reversed counter — that would be the document times
// the number of countdown lists in it. Children are visited last to first, so
// the running total is the scope of the *next* sibling, and an element's own
// scope is its subtree plus that.
//
// Nothing is built for a document with no reversed counter in it, which is
// nearly all of them: the styles are scanned for the word first, and the scan
// stops at the first one found.
func reversedStarts(root *html.Node, styles map[*html.Node]style.ComputedStyle,
	pseudo map[style.PseudoKey]style.ComputedStyle) map[*html.Node]map[string]int {

	names := reversedNames(styles)
	if len(names) == 0 {
		return nil
	}
	out := map[*html.Node]map[string]int{}
	// Two running totals per name, because §4.4.2 adds the last increment to
	// the sum: three items incrementing by one begin at four, and three
	// incrementing by two begin at eight. The "last" is the last in document
	// order, which walking backwards makes the *first* one seen.
	increments := func(cs style.ComputedStyle, sum, last []int) {
		for _, r := range parseCounterList(cs.Get("counter-increment"), 1) {
			for i, name := range names {
				if name != r.name || r.value == 0 {
					continue
				}
				sum[i] += r.value
				if last[i] == 0 {
					last[i] = r.value
				}
			}
		}
	}
	// creates says an element instantiates a counter of its own, which takes it
	// and everything after it out of the scope being counted.
	creates := func(cs style.ComputedStyle, into []bool) {
		for _, r := range parseCounterList(cs.Get("counter-reset"), 0) {
			for i, name := range names {
				if name == r.name {
					into[i] = true
				}
			}
		}
	}
	// §4.4.2 has a third case this does not: an element that *sets* the counter
	// stops the count there, "add that integer value to num and break this
	// loop". It is left out because the sum over the whole scope is the same
	// answer for the shape a document actually writes — a reversed list whose
	// items all increment, with a value on one of them — and the case where the
	// two differ is a counter-set early in a long reversed list, which is a
	// number nobody has asked this engine for. It is named here rather than
	// left to be discovered.
	// Two numbers travel up out of every node: what its subtree contributes to
	// the counter *it* creates, and what it contributes to the one enclosing
	// it. They are the same unless the node creates the counter itself, in
	// which case the second is nothing — its subtree is in its own scope.
	var walk func(n *html.Node) (ownSum, ownLast []int, instantiates []bool)
	walk = func(n *html.Node) ([]int, []int, []bool) {
		sum, last := make([]int, len(names)), make([]int, len(names))
		mine := make([]bool, len(names))
		if n.Type == html.ElementNode {
			cs := styles[n]
			if displayIsNone(cs) {
				// No box, so no increment and no counter: it is not in any
				// scope, which is what the walk below does with it too.
				return sum, last, mine
			}
			creates(cs, mine)
			increments(cs, sum, last)
			for _, name := range []string{"before", "after"} {
				if ps, ok := pseudo[style.PseudoKey{Node: n, Name: name}]; ok && generatesPseudoBox(ps) {
					increments(ps, sum, last)
				}
			}
		}
		// Last to first, so the running total is the scope of the *next*
		// sibling: an element's own scope is its subtree plus that.
		afterSum, afterLast := make([]int, len(names)), make([]int, len(names))
		for i := len(n.Children) - 1; i >= 0; i-- {
			childSum, childLast, childMine := walk(n.Children[i])
			if n.Children[i].Type == html.ElementNode {
				scope := make(map[string]int, len(names))
				for j, name := range names {
					// §4.4.2: the sum of the increments in scope, and the last
					// of them once more. Three items incrementing by one begin
					// at four; three incrementing by two begin at eight.
					total, l := childSum[j]+afterSum[j], afterLast[j]
					if childLast[j] != 0 {
						l = childLast[j]
					}
					scope[name] = total + l
				}
				out[n.Children[i]] = scope
			}
			for j := range afterSum {
				if childMine[j] {
					// The child instantiated the counter, so everything from
					// here back is in *its* scope rather than in this one.
					afterSum[j], afterLast[j] = 0, 0
					continue
				}
				afterSum[j] += childSum[j]
				if childLast[j] != 0 {
					afterLast[j] = childLast[j]
				}
			}
		}
		for j := range sum {
			sum[j] += afterSum[j]
			if afterLast[j] != 0 && last[j] == 0 {
				last[j] = afterLast[j]
			}
		}
		return sum, last, mine
	}
	walk(root)
	return out
}

// reversedNames is every counter a document creates with reversed(), or nothing
// when it creates none.
func reversedNames(styles map[*html.Node]style.ComputedStyle) []string {
	seen := map[string]bool{}
	for _, cs := range styles {
		raw := cs.Get("counter-reset")
		if !strings.Contains(raw, "reversed(") && !strings.Contains(raw, "REVERSED(") {
			continue
		}
		for _, r := range parseCounterList(raw, 0) {
			if r.reversed && r.implied {
				seen[r.name] = true
			}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
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
	if strings.EqualFold(strings.TrimSpace(listStyle), "none") {
		return ""
	}
	if text := markerText(listStyle, value); text != "" {
		return strings.TrimSuffix(text, ".")
	}
	return strconv.Itoa(value)
}
