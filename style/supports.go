package style

import (
	"strings"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/internal/ascii"
)

// CSS Conditional Rules §2's @supports: the rules inside it apply only if this
// engine understands the declaration it names.
//
// The point of the rule is that an author writes the fallback first and the
// better version inside the block, so a stage that drops the block renders the
// fallback — which is what the author asked for and what this did before, at
// the cost of a finding on every stylesheet that uses one. What it could not do
// is the other half: apply the block where the engine really does understand
// the declaration, which for a document written this way is the whole of the
// styling it meant to get.
//
// # What "supports" is answered with
//
// The engine's own claim, and not a second opinion: a declaration is supported
// when the cascade would apply it. That is the property in the registry (or a
// logical longhand, or a shorthand this engine expands), not named by
// unimplementedReason, and a value the value grammar accepts without naming
// anything this engine does not evaluate — the same judgement the cascade drops
// declarations by. So a condition cannot answer "yes" about a declaration the
// very next rule would drop or report, and "(position: bogus)" is false where
// it was true while only six properties had their values checked.
//
// What the grammar does not say is whether this engine draws every keyword it
// accepts: "(text-justify: inter-character)" is true, because the declaration
// is applied and the stage that reads it reports what it could not do. A block
// let in by that answer holds the declaration itself, so the author is told the
// same thing either way; what changes is that the rest of the block — the part
// that was understood — is applied rather than dropped along with it. Under a
// "not" that answer drops the fallback instead, and nothing reports it; that is
// the one narrowing left, and it is the difference between a value being CSS
// and a value being laid out, which only the reader knows.
//
// A condition this cannot read at all is a different matter and is reported,
// on the model of a media query asking about something unanswerable: selector()
// and font-tech() ask questions about facilities rather than declarations, and
// a browser printing the same document may apply rules this page does not have.
//
// A condition that is not a condition — "(a:b) and (c:d) or (e:f)", which mixes
// the two joins at one level without parentheses, or "not not (a:b)" — makes
// the @supports rule invalid, as Conditional 3 §2.1 has it: the rule is
// dropped, and the author is told it was malformed rather than unanswerable.

// supportsCondition evaluates an @supports prelude.
//
// unreadable names the piece it could not answer, empty when it answered all
// of it; a condition holding one is false — the rules inside are dropped —
// because the fallback outside the block is the answer the author wrote for an
// engine that does not understand. malformed says the prelude is not a
// <supports-condition> at all, and then the rule is invalid.
func supportsCondition(vals []css.ComponentValue) (matches bool, unreadable string, malformed bool) {
	it := items(vals)
	if len(it) == 0 {
		return false, "", true
	}
	got, why, ok := supportsCond(it)
	if !ok {
		return false, "", true
	}
	return got, why, false
}

// supportsCond reads "not <in-parens>", or in-parens joined by one keyword —
// all "and" or all "or", never both at one level. ok is false for anything
// else, which the caller decides the meaning of: at the top an invalid rule,
// inside parentheses §2.1's <general-enclosed>.
func supportsCond(it []css.ComponentValue) (bool, string, bool) {
	if isIdent(it[0], "not") {
		if len(it) != 2 {
			return false, "", false
		}
		got, why, ok := supportsInParens(it[1])
		if !ok {
			return false, "", false
		}
		if why != "" {
			// An unreadable condition is false, and the negation of a thing
			// that could not be read is not true — it is still unread. Saying
			// otherwise would turn "not (something unanswerable)" into a
			// licence to apply the block.
			return false, why, true
		}
		return !got, "", true
	}
	if len(it)%2 == 0 {
		return false, "", false
	}
	join := ""
	for i := 1; i < len(it); i += 2 {
		word, isWord := identOf(it[i])
		if !isWord || (word != "and" && word != "or") || (join != "" && word != join) {
			return false, "", false
		}
		join = word
	}
	result, unreadable := join != "or", ""
	for i := 0; i < len(it); i += 2 {
		got, why, ok := supportsInParens(it[i])
		if !ok {
			return false, "", false
		}
		// Every branch is read rather than stopping at the first that decides:
		// a condition this cannot read is worth reporting whichever side of a
		// join it is on, and the reading has no side effects to be spared.
		if why != "" && unreadable == "" {
			unreadable = why
		}
		if join == "or" {
			result = result || got
		} else {
			result = result && got
		}
	}
	return result, unreadable, true
}

// supportsInParens reads a parenthesised declaration, a parenthesised
// condition, or a function this engine cannot answer. Anything else in
// parentheses is <general-enclosed>: valid, and unanswerable.
func supportsInParens(v css.ComponentValue) (bool, string, bool) {
	if v.IsFunction() {
		// selector(), font-tech(), font-format(): questions about facilities
		// rather than about a declaration.
		return false, ascii.Lower(v.Token.Value) + "()", true
	}
	if !v.IsBlock() || v.Token.Kind != css.LeftParen {
		return false, "", false
	}
	inner := trimWhitespace(v.Values)
	// A declaration is "ident : value". Anything else inside the parentheses is
	// a nested condition.
	if name, value, ok := splitDeclaration(inner); ok {
		return supportsDeclaration(name, value), "", true
	}
	if it := items(inner); len(it) > 0 {
		if got, why, ok := supportsCond(it); ok {
			return got, why, true
		}
	}
	return false, serializeCondition(inner), true
}

// supportsDeclaration is the engine's own claim about a declaration: whether
// the cascade would apply it.
func supportsDeclaration(name string, value []css.ComponentValue) bool {
	if len(trimWhitespace(value)) == 0 {
		// A declaration with no value does not parse, so nothing supports it.
		return false
	}
	name = ascii.Lower(strings.TrimSpace(name))
	if strings.HasPrefix(name, "--") {
		// A custom property is supported by anything that parses CSS, which
		// §2 says in as many words. This engine parses it and cascades it.
		return true
	}
	if usesVar(value) {
		// Valid CSS for any property, and a value this engine does not
		// substitute: the cascade applies it as "unset" and says so.
		return false
	}
	_, registered := properties[name]
	sh, isShorthand := shorthands[name]
	if wideKeyword(value) != "" {
		// A CSS-wide keyword needs no taking apart: it sets every longhand to
		// itself, which is why expand has a case for it before the grammar or
		// the expander is reached.
		return registered || isLogicalLonghand(name) || isShorthand
	}
	if isShorthand {
		// A shorthand is not in the registry — it is not a property a computed
		// style holds — and asking the registry about one answered no to
		// "(margin: 0)", which every engine that parses CSS says yes to. It is
		// supported when this engine takes the value apart without leaving a
		// part out and applies every longhand it sets.
		parts, unsupported, ok := sh.expand(value)
		return ok && len(unsupported) == 0 && !judgeExpansion(name, value, parts).drop
	}
	if !registered && !isLogicalLonghand(name) {
		return false
	}
	if judgeLonghand(name, value).drop {
		// The declaration this condition names would be dropped whole, for
		// §4.2 or for naming something this engine does not evaluate.
		// Answering yes would be the cascade contradicting itself one rule
		// later.
		return false
	}
	if _, missing := unimplementedReason(name); registered && missing {
		return false
	}
	return true
}

// splitDeclaration cuts "ident : value" at its first colon.
func splitDeclaration(vals []css.ComponentValue) (string, []css.ComponentValue, bool) {
	if len(vals) == 0 || vals[0].Token.Kind != css.Ident {
		return "", nil, false
	}
	for i := 1; i < len(vals); i++ {
		if vals[i].Token.Kind == css.Colon {
			return vals[0].Token.Value, vals[i+1:], true
		}
		if vals[i].Token.Kind != css.Whitespace {
			return "", nil, false
		}
	}
	return "", nil, false
}

func isIdent(v css.ComponentValue, word string) bool {
	return v.Token.Kind == css.Ident && ascii.EqualFold(v.Token.Value, word)
}

func trimWhitespace(vals []css.ComponentValue) []css.ComponentValue {
	for len(vals) > 0 && vals[0].Token.Kind == css.Whitespace {
		vals = vals[1:]
	}
	for len(vals) > 0 && vals[len(vals)-1].Token.Kind == css.Whitespace {
		vals = vals[:len(vals)-1]
	}
	return vals
}

func serializeCondition(vals []css.ComponentValue) string {
	s := strings.TrimSpace(serialize(vals))
	if s == "" {
		return "an empty condition"
	}
	return s
}
