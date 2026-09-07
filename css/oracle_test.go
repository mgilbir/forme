package css

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The css package against an external oracle: the CSS parsing tests, written by
// Simon Sapin, dedicated to the public domain under CC0, and used as the
// conformance suite by tinycss2 (Python), rust-cssparser (Rust, and so Servo)
// and Crass (Ruby).
//
// # Why this and not more tests of our own
//
// Two earlier attempts at a guard here guarded nothing — one tested this
// package's own trivial output, and one's consistency check was tautological —
// and the lesson both drew is the same: a check built from the same
// understanding as the thing it checks cannot find a misunderstanding. The tokenizer and parser tests next door are
// worth having and they have that exact weakness. They asserts that the code
// does what its author read the specification to say.
//
// These expectations were written by someone else, from the specification, and
// three independent implementations are held to them. So a disagreement here is
// evidence about forme.
//
// # How our results are expressed in the suite's notation
//
// The suite writes an AST as nested JSON arrays, described in its README. Two
// things need saying about the translation, because a projection that quietly
// rewrites a result until it matches would be the tautology again.
//
// First, the suite models a *parse error as a node in the stream* — tinycss2
// returns them inline — while forme keeps tokens and diagnostics apart, tokens in
// the tree and problems in an Errors slice. Its nine error spellings split
// cleanly in two, and the split is the README's, not one invented here:
//
//   - "bad-string", "bad-url", ")", "]" and "}" are *nodes*. The README defines
//     each as the representation of a token — a <bad-string-token>, a
//     <bad-url-token>, an unmatched close delimiter — so each maps to the token
//     forme produces, and each is compared.
//   - "invalid", "eof-in-string", "eof-in-url", "empty" and "extra-input" are
//     *diagnostics*. They stand for nothing in the value stream; they say the
//     input was malformed. These are dropped from the expected node sequence and
//     checked against this engine's Errors slice instead — by presence, not by message,
//     because matching our wording to tinycss2's would be a hand-written table
//     that could be tuned until it passed.
//
// Second, nothing else is adjusted. Where forme disagrees with the suite the
// case is listed in deviations, with the reason, and the reason has to be about
// the specification rather than about this code.

// oracleEnv is the environment variable `make css-tests` sets.
const oracleEnv = "CSS_PARSING_TESTS"

// diagnostics are the suite's error spellings that stand for a problem rather
// than for a node. See the note above.
var diagnostics = map[string]bool{
	"invalid":       true,
	"eof-in-string": true,
	"eof-in-url":    true,
	"empty":         true,
	"extra-input":   true,
}

// The suite is built on the 2021 Candidate Recommendation draft of CSS Syntax
// Level 3, and the specification has moved since. Where the two disagree forme
// follows the current text: a browser today does what the current text says, and
// matching a superseded draft would put forme alone.
//
// Each disagreement is excused by a *rule* naming the construct that was
// removed, rather than by a list of inputs. That is deliberate. A list keyed on
// input strings excuses whatever those inputs happen to produce, so a real
// regression in an excused case would go unnoticed; a rule keyed on the
// superseded construct excuses only cases that actually exercise it, and stops
// applying by itself if the suite is ever regenerated against the current text.
//
// deviationRules is checked against the *expected* result, so the question asked
// is "does this case test something the specification no longer has", never
// "did forme fail here".
var deviationRules = []struct {
	name string
	why  string
	// applies reports whether a case exercises the construct. It is given the
	// input as well as the expected result, because one of the constructs below
	// is a *position* in the input rather than a node in the output: the
	// superseded algorithm turned it into nothing, so there is nothing in the
	// expected result to key on.
	applies func(input string, expected any) bool
	// only names the one oracle file a rule may excuse, or is empty for a rule
	// about the tokenizer, which every file exercises.
	//
	// It exists because a deviation can belong to one *algorithm* rather than to
	// the token stream: a block among declarations is a nested rule now, and at
	// the top of a stylesheet it always was one. Without this the rule below
	// took cases out of stylesheet.json and rule_list.json that were passing,
	// which is the exemption quietly widening — the thing this whole mechanism
	// is built to prevent.
	only string
}{
	{
		name: "unicode-range is no longer a token",
		// The tokenizer produces <unicode-range-token> only when its "unicode
		// ranges allowed" flag is set, and §4.3.14 notes the algorithm "is not
		// produced by the tokenizer under normal circumstances". The production
		// moved to the value layer (§5.5.11), reached only from the
		// unicode-range descriptor of @font-face — which is not in the subset
		// this engine implements. So "U+1?" is an ident, a delimiter and a
		// number, which is what forme produces.
		why: "the current specification tokenizes U+1? as ident, delim and number",
		applies: func(_ string, v any) bool {
			return containsNode(v, func(arr []any) bool {
				tag, ok := arr[0].(string)
				return ok && tag == "unicode-range"
			})
		},
	},
	{
		name: "the attribute-match tokens were removed",
		// <include-match-token> and its five siblings — ~= |= ^= $= *= and the
		// column token || — are absent from the token list in §4. Selectors
		// Level 4 parses each from the two delimiters it is written with, so
		// "^=" is a "^" and an "=", which is what forme produces.
		why: "~= |= ^= $= *= and || are two delimiters, not one token",
		applies: func(_ string, v any) bool {
			return containsString(v, func(s string) bool {
				switch s {
				case "~=", "|=", "^=", "$=", "*=", "||":
					return true
				}
				return false
			})
		},
	},
	{
		name: "the C1 controls are not ident code points",
		// The draft this suite was built on admitted every code point at or
		// above U+0080 into a name. The current definition of "non-ASCII ident
		// code point" is an explicit list that begins at U+00B7 and leaves out
		// the C1 controls, the bidirectional formatting characters, the private
		// use areas and the non-characters. forme implements that list, so
		// U+0080 is a delimiter rather than part of an identifier.
		why: "non-ASCII ident code points are an explicit list starting at U+00B7",
		applies: func(_ string, v any) bool {
			return containsNode(v, func(arr []any) bool {
				tag, _ := arr[0].(string)
				if tag != "ident" && tag != "at-keyword" && tag != "hash" {
					return false
				}
				name, ok := arr[1].(string)
				if !ok {
					return false
				}
				for _, r := range name {
					if r >= 0x80 && r <= 0x9F {
						return true
					}
				}
				return false
			})
		},
	},
	{
		name: "a style rule among declarations was a parse error",
		// §5.4.4 in the draft this suite was built on had one answer for
		// anything in a declaration list that was not an at-rule and did not
		// begin with an ident: it was a parse error, and everything up to the
		// next semicolon went with it. The current text consumes a *qualified
		// rule* there instead — that is what CSS Nesting is — so the block is a
		// rule whose selector the layer above then judges, and the declaration
		// after the semicolon survives rather than being swallowed by the
		// recovery.
		//
		// The construct is a position in the input, not a node in the output:
		// the old algorithm produced nothing for it but the error marker, and
		// the difference in the result is a declaration that is *missing*. So
		// this is the one rule that reads the input, and it reads it through
		// the tokenizer rather than by looking for a brace in the text, so that
		// a "{" inside a string or a comment is not one.
		why:  "a block among declarations is a nested rule, not a parse error",
		only: "declaration_list.json",
		applies: func(input string, v any) bool {
			if !containsNode(v, func(arr []any) bool {
				tag, _ := arr[0].(string)
				if tag != "error" || len(arr) < 2 {
					return false
				}
				kind, _ := arr[1].(string)
				return kind == "invalid"
			}) {
				return false
			}
			return opensABlockBeforeASemicolon(input)
		},
	},
}

// opensABlockBeforeASemicolon reports whether the input's first declaration-list
// item is a block rather than a declaration, which is the position the rule
// above is about.
func opensABlockBeforeASemicolon(input string) bool {
	toks, _ := Tokenize(input)
	depth := 0
	for _, t := range toks {
		switch t.Kind {
		case LeftBrace:
			if depth == 0 {
				return true
			}
			depth++
		case LeftParen, LeftSquare, Function:
			depth++
		case RightBrace, RightParen, RightSquare:
			if depth > 0 {
				depth--
			}
		case Semicolon:
			if depth == 0 {
				return false
			}
		}
	}
	return false
}

// containsNode reports whether any array node anywhere in an expected result
// satisfies pred.
func containsNode(v any, pred func([]any) bool) bool {
	arr, ok := v.([]any)
	if !ok {
		return false
	}
	if len(arr) > 1 {
		if _, isStr := arr[0].(string); isStr && pred(arr) {
			return true
		}
	}
	for _, e := range arr {
		if containsNode(e, pred) {
			return true
		}
	}
	return false
}

// containsString reports whether any bare string anywhere in an expected result
// satisfies pred. A bare string is how the notation writes a delimiter, so this
// is how a token that is no longer one is spotted.
func containsString(v any, pred func(string) bool) bool {
	switch t := v.(type) {
	case string:
		return pred(t)
	case []any:
		for _, e := range t {
			if containsString(e, pred) {
				return true
			}
		}
	}
	return false
}

// excuse returns the rule that lets a case through, if any.
func excuse(file, input string, expected any) (name, why string, ok bool) {
	for _, r := range deviationRules {
		if r.only != "" && r.only != file {
			continue
		}
		if r.applies(input, expected) {
			return r.name, r.why, true
		}
	}
	return "", "", false
}

// oracleDir is the fetched suite: the variable if it is set, the checkout's own
// fetch directory otherwise, and a failure rather than a skip when a directory
// was named and the suite is not in it.
//
// The same rule bidi's testDir and fonttest's NotoDir follow, and for the same
// reason: a skip is indistinguishable from a pass, so a mistyped path would
// leave this reporting success having read no case at all.
func oracleDir(t *testing.T) string {
	t.Helper()
	env := os.Getenv(oracleEnv)
	dir := env
	if env == "" {
		dir = filepath.Join("..", "testdata", "css-parsing-tests")
	}
	if _, err := os.Stat(filepath.Join(dir, oracleMarker)); err == nil {
		return dir
	}
	if env == "" {
		t.Skipf("the CSS parsing tests are not in this checkout; run `make css-tests`")
	}
	t.Fatalf("%s is set to %q, and there is no %s there.\n"+
		"Failing rather than skipping: this is the external oracle for the "+
		"parser, and a skip would report success having read no case.",
		oracleEnv, env, oracleMarker)
	return ""
}

// oracleMarker is the file whose presence says a directory is the suite.
const oracleMarker = "component_value_list.json"

// loadPairs reads one suite file, whose JSON is a flat array alternating input
// and expected result.
func loadPairs(t *testing.T, dir, name string) [][2]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	var flat []any
	if err := json.Unmarshal(raw, &flat); err != nil {
		t.Fatalf("parsing %s: %v", name, err)
	}
	if len(flat)%2 != 0 {
		t.Fatalf("%s holds %d entries, which is not a whole number of pairs", name, len(flat))
	}
	out := make([][2]any, 0, len(flat)/2)
	for i := 0; i < len(flat); i += 2 {
		out = append(out, [2]any{flat[i], flat[i+1]})
	}
	return out
}

// splitDiagnostics removes the suite's diagnostic markers from an expected
// result, returning what remains and how many were removed. It recurses, because
// a diagnostic can sit inside a block or a function.
func splitDiagnostics(v any) (any, int) {
	arr, ok := v.([]any)
	if !ok {
		return v, 0
	}
	// ["error", kind] is a marker; anything else is a node whose contents may
	// still hold markers.
	if len(arr) == 2 {
		if tag, ok := arr[0].(string); ok && tag == "error" {
			if kind, ok := arr[1].(string); ok && diagnostics[kind] {
				return nil, 1
			}
		}
	}
	out := make([]any, 0, len(arr))
	total := 0
	for _, e := range arr {
		kept, n := splitDiagnostics(e)
		total += n
		if n > 0 && kept == nil {
			continue
		}
		out = append(out, kept)
	}
	return out, total
}

// numberType is the suite's spelling of a numeric token's type flag.
func numberType(t Token) string {
	if t.IsInteger {
		return "integer"
	}
	return "number"
}

// oracleValue renders one component value in the suite's notation.
func oracleValue(c ComponentValue) any {
	t := c.Token
	switch t.Kind {
	case Ident:
		return []any{"ident", t.Value}
	case AtKeyword:
		return []any{"at-keyword", t.Value}
	case Hash:
		kind := "unrestricted"
		if t.IsID {
			kind = "id"
		}
		return []any{"hash", t.Value, kind}
	case String:
		return []any{"string", t.Value}
	case BadString:
		return []any{"error", "bad-string"}
	case URL:
		return []any{"url", t.Value}
	case BadURL:
		return []any{"error", "bad-url"}
	case Delim:
		return t.Value
	case Number:
		return []any{"number", t.Repr, t.Number, numberType(t)}
	case Percentage:
		return []any{"percentage", t.Repr, t.Number, numberType(t)}
	case Dimension:
		return []any{"dimension", t.Repr, t.Number, numberType(t), t.Unit}
	case Whitespace:
		return " "
	case CDO:
		return "<!--"
	case CDC:
		return "-->"
	case Colon:
		return ":"
	case Semicolon:
		return ";"
	case Comma:
		return ","
	case Function:
		return append([]any{"function", t.Value}, oracleValues(c.Values)...)
	case LeftParen:
		return append([]any{"()"}, oracleValues(c.Values)...)
	case LeftSquare:
		return append([]any{"[]"}, oracleValues(c.Values)...)
	case LeftBrace:
		return append([]any{"{}"}, oracleValues(c.Values)...)

	// An unmatched close delimiter is a preserved token here and an error node
	// there, which the README states outright.
	case RightParen:
		return []any{"error", ")"}
	case RightSquare:
		return []any{"error", "]"}
	case RightBrace:
		return []any{"error", "}"}
	}
	return fmt.Sprintf("<unrepresentable token kind %d>", t.Kind)
}

// oracleValues renders a list, always as a non-nil slice: the suite writes an
// empty list as [], which unmarshals to an empty slice and not to null.
func oracleValues(vals []ComponentValue) []any {
	out := make([]any, 0, len(vals))
	for _, v := range vals {
		out = append(out, oracleValue(v))
	}
	return out
}

func oracleRule(r Rule) any {
	if r.At {
		// The block slot is null for a statement at-rule such as @import, and a
		// list — possibly empty — for a block at-rule such as @media.
		var block any
		if r.HasBlock {
			block = oracleValues(r.Block)
		}
		return []any{"at-rule", r.Name, oracleValues(r.Prelude), block}
	}
	return []any{"qualified rule", oracleValues(r.Prelude), oracleValues(r.Block)}
}

func oracleDeclaration(d Declaration) any {
	return []any{"declaration", d.Name, oracleValues(d.Value), d.Important}
}

// run is one suite file: how to parse its inputs and how to render the result.
type run struct {
	file  string
	parse func(input string) (nodes []any, errs []Error)
}

func oracleRuns() []run {
	return []run{
		{"component_value_list.json", func(in string) ([]any, []Error) {
			vals, errs := ParseComponentValues(in)
			return oracleValues(vals), errs
		}},
		{"stylesheet.json", func(in string) ([]any, []Error) {
			rules, errs := ParseStylesheet(in)
			return renderRules(rules), errs
		}},
		{"rule_list.json", func(in string) ([]any, []Error) {
			rules, errs := ParseRules(in)
			return renderRules(rules), errs
		}},
		{"declaration_list.json", func(in string) ([]any, []Error) {
			decls, rules, errs := ParseDeclarations(in)
			return renderDeclarationList(decls, rules), errs
		}},
		// The current algorithm for the same thing, and the file that says so.
		//
		// declaration_list.json is the 2021 draft's §5.4.4, where anything in a
		// declaration list that did not begin with an ident was a parse error
		// and everything up to the next semicolon went with it. The current text
		// consumes a *qualified rule* there — that is CSS Nesting — and
		// blocks_contents.json is the suite's file for it.
		//
		// Which is what forme implements, and it was not being run. The
		// nesting behaviour was visible only as a deviation rule taking cases
		// *out* of declaration_list.json, so the old answers were excused and
		// the new ones were checked by nothing. Thirteen cases, and they pass
		// with no excuse at all.
		{"blocks_contents.json", func(in string) ([]any, []Error) {
			decls, rules, errs := ParseDeclarations(in)
			return renderDeclarationList(decls, rules), errs
		}},
	}
}

// TestCSSOracleAnB checks the An+B microsyntax against the suite's file for it.
//
// It is separate from the runs above because its file has a different shape:
// the expected result is not an AST but a pair of integers, or null for input
// that is not an An+B at all. Roughly half its cases are the null ones, which is
// the half worth having — "3 n", "+ 2n" and "3.1n" all look like An+B values and
// are not, and each is a place where a reader that is merely permissive selects
// elements the author did not ask for.
func TestCSSOracleAnB(t *testing.T) {
	dir := oracleDir(t)

	pairs := loadPairs(t, dir, "An+B.json")
	if len(pairs) == 0 {
		t.Fatal("An+B.json holds no cases")
	}
	var valid, invalid int

	for _, pair := range pairs {
		input, ok := pair[0].(string)
		if !ok {
			t.Fatalf("an input that is not a string: %v", pair[0])
		}
		vals, _ := ParseComponentValues(input)
		got, gotOK := ParseAnB(vals)

		want, wantOK := pair[1].([]any)
		if !wantOK {
			// null: the input is not an An+B.
			invalid++
			if gotOK {
				t.Errorf("%q read as %dn%+d, and is not an An+B at all", input, got.A, got.B)
			}
			continue
		}
		valid++
		if len(want) != 2 {
			t.Fatalf("%q: expected result is not a pair: %v", input, want)
		}
		wantA, okA := want[0].(float64)
		wantB, okB := want[1].(float64)
		if !okA || !okB {
			t.Fatalf("%q: expected result is not two numbers: %v", input, want)
		}
		if !gotOK {
			t.Errorf("%q was rejected, and is the An+B %vn%+v", input, wantA, wantB)
			continue
		}
		if float64(got.A) != wantA || float64(got.B) != wantB {
			t.Errorf("%q read as %dn%+d, want %vn%+v", input, got.A, got.B, wantA, wantB)
		}
	}
	t.Logf("An+B.json: %d valid and %d invalid cases checked", valid, invalid)

	// A suite that had drifted to all-valid or all-invalid would still pass
	// every assertion above while checking almost nothing.
	if valid == 0 || invalid == 0 {
		t.Errorf("the suite gave %d valid and %d invalid cases; both are needed", valid, invalid)
	}
}

func renderRules(rules []Rule) []any {
	out := make([]any, 0, len(rules))
	for _, r := range rules {
		out = append(out, oracleRule(r))
	}
	return out
}

// renderDeclarationList puts declarations and at-rules back into source order.
//
// ParseDeclarations returns them separately, because an at-rule inside a
// declaration block is CSS Nesting and out of the implemented subset, so no
// caller in this engine wants them interleaved. The suite does compare order,
// and both carry the offset they began at, so the order is recoverable exactly
// rather than approximated.
func renderDeclarationList(decls []Declaration, rules []Rule) []any {
	type item struct {
		offset int
		value  any
	}
	items := make([]item, 0, len(decls)+len(rules))
	for _, d := range decls {
		items = append(items, item{d.Offset, oracleDeclaration(d)})
	}
	for _, r := range rules {
		items = append(items, item{r.Offset, oracleRule(r)})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].offset < items[j].offset })

	out := make([]any, 0, len(items))
	for _, it := range items {
		out = append(out, it.value)
	}
	return out
}

func TestCSSOracle(t *testing.T) {
	dir := oracleDir(t)

	// How many cases each deviation rule accounted for, so that a rule which
	// excuses nothing can be caught and deleted.
	excusedBy := map[string]int{}

	for _, r := range oracleRuns() {
		t.Run(r.file, func(t *testing.T) {
			pairs := loadPairs(t, dir, r.file)
			if len(pairs) == 0 {
				t.Fatalf("%s holds no cases", r.file)
			}
			var checked, excused int

			for _, pair := range pairs {
				input, ok := pair[0].(string)
				if !ok {
					t.Fatalf("%s: an input that is not a string: %v", r.file, pair[0])
				}
				if name, _, ok := excuse(r.file, input, pair[1]); ok {
					excused++
					excusedBy[name]++
					continue
				}
				checked++

				want, wantErrs := splitDiagnostics(pair[1])
				got, errs := r.parse(input)

				if !reflect.DeepEqual(got, want) {
					t.Errorf("%s\ninput %q\n got %s\nwant %s",
						r.file, input, mustJSON(got), mustJSON(want))
					continue
				}
				// The suite says the input was malformed, so forme must have
				// noticed something.
				//
				// Only this direction is asserted. The converse — that forme is
				// silent wherever the suite is — would be false, and not because
				// forme is noisy: the suite's markers are the errors tinycss2
				// chose to put in the tree, while these are every parse error
				// the specification defines. An unterminated comment, a
				// backslash at end of input and an unclosed block are all parse
				// errors in §4 and §5, and the suite marks none of the three.
				// Requiring agreement would mean copying another parser's
				// reporting policy and calling it conformance. That forme stays
				// quiet on correct input is asserted next door, over stylesheets
				// written to be correct, where it is a claim about forme rather
				// than about tinycss2.
				if wantErrs > 0 && len(errs) == 0 {
					t.Errorf("%s\ninput %q\nparsed identically but reported nothing, "+
						"while the suite marks %d problem(s)", r.file, input, wantErrs)
				}
			}
			t.Logf("%s: %d cases checked, %d deliberately excused", r.file, checked, excused)
		})
	}

	// A rule that excuses nothing has outlived its reason — the suite was
	// regenerated, or the rule never matched what its author thought. Either
	// way it must go, or the list becomes a place where exemptions accumulate
	// unread.
	for _, r := range deviationRules {
		if excusedBy[r.name] == 0 {
			t.Errorf("the deviation rule %q excused no case; the suite no longer "+
				"tests %s, so the rule should be removed", r.name, r.why)
		}
	}
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// TestCSSOracleHasTeeth is the check on the check, on the model of
// TestArlingtonOracleHasTeeth. An oracle whose comparison silently accepts
// everything is worse than no oracle, because it reads as coverage.
//
// It plants three faults of the kinds that matter — a wrong value, a wrong
// structure, and a dropped node — and requires the comparison to reject each.
func TestCSSOracleHasTeeth(t *testing.T) {
	ident := func(name string) any { return []any{"ident", name} }

	cases := []struct {
		name  string
		got   any
		want  any
		equal bool
	}{
		{"identical", []any{ident("a")}, []any{ident("a")}, true},
		{"a different value", []any{ident("a")}, []any{ident("b")}, false},
		{"a different kind", []any{ident("a")}, []any{[]any{"string", "a"}}, false},
		{"a dropped node", []any{ident("a")}, []any{ident("a"), " "}, false},
		{"a flattened block", []any{[]any{"{}", ident("a")}}, []any{[]any{"{}"}}, false},
		{
			"an integer where a number was written",
			[]any{[]any{"number", "1.0", 1.0, "number"}},
			[]any{[]any{"number", "1.0", 1.0, "integer"}},
			false,
		},
		{
			"a number equal in value but not as written",
			[]any{[]any{"number", "1.0", 1.0, "number"}},
			[]any{[]any{"number", "1", 1.0, "number"}},
			false,
		},
	}
	for _, tc := range cases {
		if got := reflect.DeepEqual(tc.got, tc.want); got != tc.equal {
			t.Errorf("%s: comparison said equal=%v, want %v", tc.name, got, tc.equal)
		}
	}

	// And the diagnostic split must actually remove markers, or every
	// malformed-input case would compare against a stream forme never produces.
	in := []any{ident("a"), []any{"error", "eof-in-string"}, []any{"error", "bad-url"}}
	kept, n := splitDiagnostics(in)
	if n != 1 {
		t.Errorf("splitDiagnostics removed %d markers, want 1", n)
	}
	want := []any{ident("a"), []any{"error", "bad-url"}}
	if !reflect.DeepEqual(kept, want) {
		t.Errorf("splitDiagnostics kept %s, want %s", mustJSON(kept), mustJSON(want))
	}
}

// TestCSSOracleDeviationsAreNarrow is the check on the exemptions. A rule that
// excused more than the construct it names would hide real failures behind a
// reason that sounds good, which is the failure mode a list of exemptions
// always has.
//
// Each rule is given a result that does *not* exercise its construct and must
// decline it, and one that does and must take it.
func TestCSSOracleDeviationsAreNarrow(t *testing.T) {
	ordinary := []any{
		[]any{"ident", "a"}, " ", ":", []any{"string", "b"},
		[]any{"function", "rgb", []any{"number", "1", 1.0, "integer"}},
		[]any{"{}", []any{"ident", "c"}}, "~", "=", "|",
	}
	// The input is an ordinary one too, and it holds a block — so a rule that
	// keyed on the input alone would take this and be caught here.
	for _, r := range deviationRules {
		if r.applies("a { b: c }", ordinary) {
			t.Errorf("the rule %q excuses an ordinary result, so it would hide real failures", r.name)
		}
	}
	// And the case the last rule is for is not excused by its expected result
	// alone: "z;a:b" expects the same error marker and must keep being checked,
	// because the input has no block in it and the two algorithms agree.
	blocky := []any{[]any{"error", "invalid"},
		[]any{"declaration", "a", []any{[]any{"ident", "b"}}, false}}
	for _, r := range deviationRules {
		if r.applies("z;a:b", blocky) {
			t.Errorf("the rule %q excuses a parse error that is still a parse "+
				"error, so a real regression there would go unnoticed", r.name)
		}
	}
	// The input half of the rule, on its own. It is the half no case in the
	// suite exercises — the one excused case opens its block first — so a wrong
	// answer to any of these would widen the exemption with nothing to notice
	// it.
	for _, tc := range []struct {
		input string
		want  bool
		why   string
	}{
		{"@ media screen { div{;}} a:b", true, "a block before any semicolon"},
		{"z;a:b", false, "no block at all"},
		{"a: b; c { d }", false, "a declaration first, and the block after its semicolon"},
		{"a: b(c { d });", false, "a brace inside a function is not at this level"},
		{"a: b[c { d }];", false, "a brace inside a square block is not at this level"},
		{"a: 'x { y' ; b { c }", false, "a brace inside a string is not a brace"},
	} {
		if got := opensABlockBeforeASemicolon(tc.input); got != tc.want {
			t.Errorf("opensABlockBeforeASemicolon(%q) = %v, want %v — %s",
				tc.input, got, tc.want, tc.why)
		}
	}

	// Nor in a file whose algorithm never had the construct. A block at the top
	// of a stylesheet is a qualified rule under both texts, so those cases stay
	// checked.
	for _, file := range []string{"stylesheet.json", "rule_list.json", "component_value_list.json"} {
		if name, _, ok := excuse(file, "@ media screen { div{;}} a:b",
			[]any{[]any{"error", "invalid"}}); ok {
			t.Errorf("%s had a case excused by %q; the deviation is in the "+
				"declaration-list algorithm and nowhere else", file, name)
		}
	}

	// And each rule takes the construct it is for, so that none is dead.
	exercises := []struct {
		rule     string
		expected any
	}{
		{"unicode-range is no longer a token", []any{[]any{"unicode-range", 16.0, 31.0}}},
		{"the attribute-match tokens were removed", []any{[]any{"[]", []any{"ident", "h"}, "^=", []any{"ident", "x"}}}},
		{"the C1 controls are not ident code points", []any{[]any{"ident", "a\u0080b"}}},
	}
	for _, tc := range exercises {
		name, _, ok := excuse("", "", tc.expected)
		if !ok {
			t.Errorf("no rule excused %s, which tests a superseded construct", mustJSON(tc.expected))
			continue
		}
		if name != tc.rule {
			t.Errorf("%s was excused by %q, want %q", mustJSON(tc.expected), name, tc.rule)
		}
	}

	// The nesting rule needs both halves, so it is exercised with both.
	name, _, ok := excuse("declaration_list.json", "@ media screen { div{;}} a:b",
		[]any{[]any{"error", "invalid"}})
	if !ok || name != "a style rule among declarations was a parse error" {
		t.Errorf("a block among declarations was excused by %q (ok=%v), want the "+
			"nesting rule", name, ok)
	}
}

// unrunSuiteFiles are the suite's files this package does not answer, each with
// why.
//
// A file is never simply absent from the run. blocks_contents.json was, and
// nothing said so: it is the file for the algorithm forme implements, and its
// thirteen cases were checked by nobody while the superseded file beside it was
// checked with a deviation rule excusing the difference. The one thing that
// would have caught that is this list, so here it is.
//
// The three below are algorithms rather than tables, and the entry points they
// name are not ones this package has. That is a real absence rather than a
// dodge: "parse a declaration" consumes its whole input as one declaration —
// "foo:;bar:;" is one declaration whose value holds the second — where
// ParseDeclarations splits at semicolons, so answering the file from the public
// API would mean writing a second parser in a test and comparing this
// repository's two guesses with each other.
var unrunSuiteFiles = map[string]string{
	"one_declaration.json": "§5.3.7 parses one declaration out of a whole input " +
		"without splitting at semicolons; ParseDeclarations splits, and there is " +
		"no caller for the other",
	"one_rule.json": "§5.3.5 parses exactly one rule and refuses trailing input; " +
		"ParseRules parses a list, and there is no caller for the other",
	"stylesheet_bytes.json": "the input is bytes and the answer includes the " +
		"encoding, which is decided before this package sees a string — see " +
		"style's @charset handling and html's sniffing",
}

// TestEverySuiteFileIsRunOrExplained stops a file from being quietly skipped.
//
// The colour files have their own accounting next door in style; everything
// else the suite ships has to be answered here or named above with a reason,
// and a file that arrives in a regenerated suite fails rather than going
// unnoticed.
func TestEverySuiteFileIsRunOrExplained(t *testing.T) {
	dir := oracleDir(t)

	present, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(present) == 0 {
		t.Fatalf("no suite files in %s: %v", dir, err)
	}

	answered := map[string]bool{
		// Its own test, because the file's shape is different: the expected
		// result is a pair of integers rather than an AST.
		"An+B.json": true,
		// Its own test, because the expected result is a single component
		// value rather than a list of them.
		"one_component_value.json": true,
	}
	for _, r := range oracleRuns() {
		answered[r.file] = true
	}

	for _, path := range present {
		name := filepath.Base(path)
		if strings.HasPrefix(name, "color_") {
			continue // style/color_test.go accounts for these
		}
		if answered[name] {
			continue
		}
		if _, ok := unrunSuiteFiles[name]; !ok {
			t.Errorf("the suite ships %s, which is neither answered nor listed "+
				"with a reason for not being.\nA file nobody runs and nobody "+
				"mentions is the whole of how blocks_contents.json went "+
				"unchecked.", name)
		}
	}

	// And the reasons do not outlive their files.
	for name := range unrunSuiteFiles {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s is listed as unrun and the suite no longer ships it", name)
		}
	}
}

// parseOneComponentValue is §5.3.7, written over ParseComponentValues because
// the algorithm is that plus two conditions: leading and trailing whitespace is
// ignored, and exactly one value has to remain.
//
// Unlike "parse a declaration" and "parse a rule", nothing here is a second
// reading of the specification — the work is all in ParseComponentValues, and
// what is added is the counting.
func parseOneComponentValue(input string) (val ComponentValue, problem string) {
	vals, _ := ParseComponentValues(input)
	for len(vals) > 0 && vals[0].Token.Kind == Whitespace {
		vals = vals[1:]
	}
	for len(vals) > 0 && vals[len(vals)-1].Token.Kind == Whitespace {
		vals = vals[:len(vals)-1]
	}
	switch {
	case len(vals) == 0:
		return ComponentValue{}, "empty"
	case len(vals) > 1:
		return ComponentValue{}, "extra-input"
	}
	return vals[0], ""
}

// TestCSSOracleOneComponentValue runs one_component_value.json.
//
// Four of its ten cases are input that is not a component value at all —
// nothing, whitespace, a comment, and a value with something after it — and
// those are the half worth having: a reader that answers "." for ".foo" has
// taken the first thing it saw and called the rest a success.
func TestCSSOracleOneComponentValue(t *testing.T) {
	dir := oracleDir(t)

	pairs := loadPairs(t, dir, "one_component_value.json")
	if len(pairs) == 0 {
		t.Fatal("one_component_value.json holds no cases")
	}
	var values, refusals int
	for _, pair := range pairs {
		input, ok := pair[0].(string)
		if !ok {
			t.Fatalf("an input that is not a string: %v", pair[0])
		}
		got, problem := parseOneComponentValue(input)

		// A top-level ["error", kind] where kind is one of the suite's
		// diagnostics is the algorithm refusing. An error node *inside* a value
		// — an unmatched ")" — is a preserved token and not a refusal, which is
		// why this looks at the top level only.
		if want, kind := refusedAs(pair[1]); want {
			refusals++
			if problem == "" {
				t.Errorf("input %q\nparsed as %s, and the suite says %q",
					input, mustJSON(oracleValue(got)), kind)
			} else if problem != kind {
				t.Errorf("input %q\nrefused as %q, and the suite says %q",
					input, problem, kind)
			}
			continue
		}
		values++
		if problem != "" {
			t.Errorf("input %q\nrefused as %q, and the suite gives a value: %s",
				input, problem, mustJSON(pair[1]))
			continue
		}
		if rendered := oracleValue(got); !reflect.DeepEqual(rendered, pair[1]) {
			t.Errorf("input %q\n got %s\nwant %s", input, mustJSON(rendered), mustJSON(pair[1]))
		}
	}
	t.Logf("one_component_value.json: %d values and %d refusals checked", values, refusals)
}

// refusedAs reports whether an expected result is the algorithm refusing, and
// how the suite spells the refusal.
func refusedAs(v any) (bool, string) {
	arr, ok := v.([]any)
	if !ok || len(arr) != 2 {
		return false, ""
	}
	tag, _ := arr[0].(string)
	kind, _ := arr[1].(string)
	if tag != "error" || !diagnostics[kind] {
		return false, ""
	}
	return true, kind
}

// TestTheReadmeCountsTheSyntaxSuite.
//
// The README's figure is the only place a reader learns how much of the suite
// is run, and it went on saying 206 while blocks_contents.json's thirteen cases
// sat unrun beside it. A number in prose that nothing checks is a number that
// stops being true the first time the run changes, and this is the check.
func TestTheReadmeCountsTheSyntaxSuite(t *testing.T) {
	dir := oracleDir(t)
	checked, excused := suiteTotals(t, dir)

	text, err := os.ReadFile(filepath.Join("..", "README.md"))
	if err != nil {
		t.Fatalf("reading the README: %v", err)
	}
	if got := readmeCount(t, string(text), `([\d,]+) cases from the suite`); got != checked {
		t.Errorf("the README says %d cases are run and the suite gives %d", got, checked)
	}
	if got := readmeCount(t, string(text), `with ([\d,]+) more deliberately excused`); got != excused {
		t.Errorf("the README says %d cases are excused and %d are", got, excused)
	}
}

// suiteTotals counts what the oracle above checks and what it excuses, without
// asserting anything about the answers — those are checked where they are
// produced. It is separate so that the count and the run cannot disagree: both
// walk the same files and the same deviation rules.
func suiteTotals(t *testing.T, dir string) (checked, excused int) {
	t.Helper()
	for _, r := range oracleRuns() {
		for _, pair := range loadPairs(t, dir, r.file) {
			input, _ := pair[0].(string)
			if _, _, ok := excuse(r.file, input, pair[1]); ok {
				excused++
				continue
			}
			checked++
		}
	}
	// The two files with a shape of their own. Neither has ever needed an
	// excuse: An+B is a microsyntax the current text has not moved, and a
	// component value is the token stream itself.
	checked += len(loadPairs(t, dir, "An+B.json"))
	checked += len(loadPairs(t, dir, "one_component_value.json"))
	return checked, excused
}

// readmeCount pulls a number out of the README.
func readmeCount(t *testing.T, text, pattern string) int {
	t.Helper()
	m := regexp.MustCompile(pattern).FindStringSubmatch(text)
	if m == nil {
		t.Fatalf("the README no longer says %q, so this test cannot check it", pattern)
	}
	n, err := strconv.Atoi(strings.ReplaceAll(m[1], ",", ""))
	if err != nil {
		t.Fatalf("the README's %q is not a number: %v", m[1], err)
	}
	return n
}
