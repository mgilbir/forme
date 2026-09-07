package style

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mgilbir/forme/css"
)

// The cascade keeps a winning value as text, and every reader of that value
// tokenizes it again. So serialize has one job beyond being readable: what it
// writes has to come back as the value it was given.
//
// It did not, and both ways it failed were quiet — the value came back as
// something the author could have typed, so nothing anywhere reported a
// problem:
//
//   - Escapes in a name were dropped. "\31 23" is an identifier whose name is
//     "123"; written back as "123" it is a number. "a\ b" is one identifier;
//     written back as "a b" it is two, so a font-family of "a b" became the two
//     families "a" and "b".
//
//   - An unmatched ")", "]" or "}" was dropped outright. The specification
//     calls those preserved tokens and this engine's own parser keeps them, so
//     a value round-tripped through the cascade lost a character it was given.
//
// A single value would only pin the case it was written for. This runs every
// input of the CSS parsing tests through the round trip instead, which is where
// the pathological names in this file's comments came from.

// roundTripFiles are the suite files whose inputs are values. The colour files
// are inputs to ParseColor and are checked next door; the rest of the suite is
// stylesheets and rules, which reach serialize only as the values inside them.
var roundTripFiles = []string{
	"component_value_list.json",
	"declaration_list.json",
	"blocks_contents.json",
	"one_component_value.json",
}

// TestSerializingAValueGivesTheValueBack.
func TestSerializingAValueGivesTheValueBack(t *testing.T) {
	dir := colorOracleDir(t)

	checked, skipped := 0, 0
	for _, name := range roundTripFiles {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("reading %s: %v\nThe suite is in place, so this is an "+
				"unfinished fetch.", name, err)
		}
		var flat []any
		if err := json.Unmarshal(raw, &flat); err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		for i := 0; i < len(flat); i += 2 {
			input, ok := flat[i].(string)
			if !ok {
				continue
			}
			vals, _ := css.ParseComponentValues(input)
			if hasUnrepresentable(vals) {
				// A bad-string or a bad-url did not tokenize in the first
				// place, so there is no text that would bring it back;
				// serialize says "<invalid>" and means it.
				skipped++
				continue
			}
			checked++
			text := serialize(vals)
			again, _ := css.ParseComponentValues(text)
			if diff := valuesDiffer(trimEnds(vals), trimEnds(again)); diff != "" {
				t.Errorf("%s\ninput      %q\nserialized %q\n%s",
					name, input, text, diff)
			}
		}
	}
	// The suite's four value files hold 83 inputs, seven of which carry a
	// bad-string or a bad-url. A floor rather than the number itself, so that a
	// case added upstream does not fail here — but a floor all the same,
	// because a run that read nothing would otherwise be a pass.
	if checked < 60 {
		t.Fatalf("only %d values were round-tripped and %d skipped; the suite "+
			"holds more than that, so this has stopped reading them",
			checked, skipped)
	}
	t.Logf("%d values round-tripped through the cascade's text form, %d skipped "+
		"as untokenizable", checked, skipped)
}

// trimEnds drops leading and trailing whitespace, which serialize trims and
// which no reader of a declaration's value can see, and collapses a run of
// whitespace tokens into one.
//
// A run is what a comment leaves behind: "; /**/ ;" tokenizes as two whitespace
// tokens with nothing between them, because the comment is removed after the
// boundary is drawn. Nothing can serialise to two adjacent whitespace tokens
// and nothing reads the difference — a value's whitespace is a separator, and
// two separators are one.
func trimEnds(vals []css.ComponentValue) []css.ComponentValue {
	out := make([]css.ComponentValue, 0, len(vals))
	for _, v := range vals {
		if v.Token.Kind == css.Whitespace && len(out) > 0 &&
			out[len(out)-1].Token.Kind == css.Whitespace {
			continue
		}
		out = append(out, v)
	}
	for len(out) > 0 && out[0].Token.Kind == css.Whitespace {
		out = out[1:]
	}
	for len(out) > 0 && out[len(out)-1].Token.Kind == css.Whitespace {
		out = out[:len(out)-1]
	}
	return out
}

// hasUnrepresentable reports whether a value holds a token that did not
// tokenize, anywhere inside it.
func hasUnrepresentable(vals []css.ComponentValue) bool {
	for _, v := range vals {
		if v.Token.Kind == css.BadString || v.Token.Kind == css.BadURL {
			return true
		}
		if hasUnrepresentable(v.Values) {
			return true
		}
	}
	return false
}

// valuesDiffer describes the first difference between two component value
// lists, or returns "" when they are the same value.
//
// Token by token rather than text against text: a serialisation that drops an
// escape produces text that is a *fixed point* — "\31 23" writes as "123",
// which writes as "123" again — so comparing the two strings would call the
// worst of these faults a pass.
func valuesDiffer(a, b []css.ComponentValue) string {
	if len(a) != len(b) {
		return fmt.Sprintf("it came back as %d values and was %d: %s against %s",
			len(b), len(a), sketch(b), sketch(a))
	}
	for i := range a {
		if d := valueDiffers(a[i], b[i]); d != "" {
			return fmt.Sprintf("value %d: %s", i, d)
		}
	}
	return ""
}

func valueDiffers(a, b css.ComponentValue) string {
	at, bt := a.Token, b.Token
	switch {
	case at.Kind != bt.Kind:
		return fmt.Sprintf("it came back as a %v and was a %v", bt.Kind, at.Kind)
	case at.Value != bt.Value:
		return fmt.Sprintf("its value came back as %q and was %q", bt.Value, at.Value)
	case at.Kind == css.Hash && at.IsID != bt.IsID:
		return fmt.Sprintf("its id flag came back %v and was %v — only an id "+
			"hash matches an ID selector", bt.IsID, at.IsID)
	case at.Unit != bt.Unit:
		return fmt.Sprintf("its unit came back as %q and was %q", bt.Unit, at.Unit)
	case at.Number != bt.Number:
		return fmt.Sprintf("its number came back as %v and was %v", bt.Number, at.Number)
	case at.IsInteger != bt.IsInteger:
		return fmt.Sprintf("its integer flag came back %v and was %v", bt.IsInteger, at.IsInteger)
	}
	return valuesDiffer(a.Values, b.Values)
}

// sketch is a short rendering of a value list for a failure message.
func sketch(vals []css.ComponentValue) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, v := range vals {
		if i > 0 {
			b.WriteString(" ")
		}
		fmt.Fprintf(&b, "%v(%q)", v.Token.Kind, v.Token.Value)
	}
	b.WriteByte(']')
	return b.String()
}
