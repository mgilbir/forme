package style

import (
	"strings"

	"github.com/mgilbir/forme/css"
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
// # What "supports" is answered with, and the narrowing it keeps
//
// The engine's own claim, and not a second opinion: a property is supported
// when it is in the registry and unimplementedReason does not name it. That is
// the same pair of facts the "not implemented" finding is raised from, so a
// condition cannot answer "yes" about a property the very next declaration
// would be reported for.
//
// The second of those two cannot fire today, and that is worth saying rather
// than leaving to be discovered: unimplementedProperties is empty, so every
// registered property is an implemented one. It is asked anyway because the
// state it is for is one this engine has been in within the day — a property
// registered so that its value cascades, before anything reads it, which is
// where font-feature-settings sat this morning. Asking the same question the
// cascade asks means the answer follows it without anyone remembering to.
//
// It is answered about the *property* and not the value. §2 tests whether the
// declaration would parse, and this engine has no single place that says
// whether a value parses — that is decided per property, in the stage that
// reads it. So "(position: sticky)" is answered yes where position is
// implemented and sticky is not.
//
// That narrowing is sound rather than merely convenient, and the reason is
// where the report goes. A block let in by this answer holds the declaration
// itself, and a value this engine cannot act on is reported *there*, by the
// stage that could not act on it. The author is told the same thing either way;
// what changes is that the rest of the block — the part that was understood —
// is applied rather than dropped along with it.
//
// A condition this cannot read at all is a different matter and is reported,
// on the model of a media query asking about something unanswerable: selector()
// and font-tech() ask questions about facilities rather than declarations, and
// a browser printing the same document may apply rules this page does not have.

// supportsCondition evaluates an @supports prelude.
//
// The second result names the piece it could not read, empty when it read all
// of it. A condition that cannot be read is false — the rules inside are
// dropped — because the fallback outside the block is the answer the author
// wrote for an engine that does not understand.
func supportsCondition(vals []css.ComponentValue) (bool, string) {
	vals = trimWhitespace(vals)
	if len(vals) == 0 {
		return false, "an empty condition"
	}
	return supportsOr(vals)
}

// supportsOr reads the "or"-joined list, which binds loosest.
func supportsOr(vals []css.ComponentValue) (bool, string) {
	parts, ok := splitKeyword(vals, "or")
	if !ok {
		return supportsAnd(vals)
	}
	result, unreadable := false, ""
	for _, part := range parts {
		got, why := supportsAnd(part)
		if why != "" && unreadable == "" {
			unreadable = why
		}
		// Every branch is read rather than stopping at the first true one: a
		// condition this cannot read is worth reporting whichever side of an
		// "or" it is on, and the reading has no side effects to be spared.
		result = result || got
	}
	return result, unreadable
}

// supportsAnd reads the "and"-joined list.
func supportsAnd(vals []css.ComponentValue) (bool, string) {
	parts, ok := splitKeyword(vals, "and")
	if !ok {
		return supportsNot(vals)
	}
	result, unreadable := true, ""
	for _, part := range parts {
		got, why := supportsNot(part)
		if why != "" && unreadable == "" {
			unreadable = why
		}
		result = result && got
	}
	return result, unreadable
}

// supportsNot reads the negation, which takes a single condition after it.
func supportsNot(vals []css.ComponentValue) (bool, string) {
	vals = trimWhitespace(vals)
	if len(vals) > 0 && isIdent(vals[0], "not") {
		got, why := supportsNot(vals[1:])
		if why != "" {
			// An unreadable condition is false, and the negation of a thing
			// that could not be read is not true — it is still unread. Saying
			// otherwise would turn "not (something unanswerable)" into a
			// licence to apply the block.
			return false, why
		}
		return !got, ""
	}
	return supportsPrimary(vals)
}

// supportsPrimary reads a parenthesised declaration, a parenthesised condition,
// or a function this engine cannot answer.
func supportsPrimary(vals []css.ComponentValue) (bool, string) {
	vals = trimWhitespace(vals)
	if len(vals) != 1 {
		return false, serializeCondition(vals)
	}
	v := vals[0]
	if v.IsFunction() {
		// selector(), font-tech(), font-format(): questions about facilities
		// rather than about a declaration.
		return false, strings.ToLower(v.Token.Value) + "()"
	}
	if !v.IsBlock() || v.Token.Kind != css.LeftParen {
		return false, serializeCondition(vals)
	}
	inner := trimWhitespace(v.Values)
	// A declaration is "ident : value". Anything else inside the parentheses is
	// a nested condition.
	if name, value, ok := splitDeclaration(inner); ok {
		return supportsDeclaration(name, value), ""
	}
	return supportsOr(inner)
}

// supportsDeclaration is the engine's own claim about a property.
func supportsDeclaration(name string, value []css.ComponentValue) bool {
	if len(trimWhitespace(value)) == 0 {
		// A declaration with no value does not parse, so nothing supports it.
		return false
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if strings.HasPrefix(name, "--") {
		// A custom property is supported by anything that parses CSS, which
		// §2 says in as many words. This engine parses it and cascades it.
		return true
	}
	if isLogicalLonghand(name) {
		// Implemented by being renamed to the physical property it sets, which
		// is why it is not in the registry. See prepareDecl.
		return true
	}
	if _, known := properties[name]; !known {
		return false
	}
	_, missing := unimplementedReason(name)
	return !missing
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

// splitKeyword cuts a condition on a top-level keyword, reporting whether it
// was there at all.
func splitKeyword(vals []css.ComponentValue, word string) ([][]css.ComponentValue, bool) {
	var parts [][]css.ComponentValue
	last, found := 0, false
	for i, v := range vals {
		if !isIdent(v, word) {
			continue
		}
		found = true
		parts = append(parts, vals[last:i])
		last = i + 1
	}
	if !found {
		return nil, false
	}
	return append(parts, vals[last:]), true
}

func isIdent(v css.ComponentValue, word string) bool {
	return v.Token.Kind == css.Ident && strings.EqualFold(v.Token.Value, word)
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
