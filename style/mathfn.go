package style

import (
	"encoding/binary"
	"math"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/internal/ascii"
)

// The math functions of CSS Values 4 §10: calc() and everything that may stand
// beside it or inside it — min(), max(), clamp(), round(), mod(), rem(), abs(),
// sign(), the trigonometric functions and the exponential ones.
//
// # One reader, two questions
//
// A math function is read once, into a program, and the program answers both
// questions anybody asks of it. The value grammar asks what type it computes to
// and whether this engine can evaluate it, with no element in hand; a length's
// reader asks for its value, with the font size and the page known. They used
// to be two readers, and they disagreed: the grammar split "calc(1px +-2px)" at
// the "+" and called it a length, and the evaluator, which requires white space
// on both sides of a "+", called it nothing — so a declaration the cascade kept
// was one layout could not read.
//
// # Types
//
// A calculation's type is the kind of thing it measures — a number, a length,
// an angle, a time — and whether a percentage is in it. A percentage takes the
// type its context resolves percentages against: in width it is a length, so
// "min(10px, 50%)" is a length with a percentage in it; in opacity it resolves
// against a number, and §10.9 says a percentage then stays a percentage and
// does not add to a number, so "calc(.25 + 25%)" is invalid there.
//
// Products keep Values 3's rule, as calc() always has here: one side of a "*"
// is a number, and the right side of a "/" is one. Values 4 lets two lengths
// multiply as long as the units cancel by the top; that is not read, and such
// an expression is invalid rather than mis-evaluated.
//
// # Values, and the two that do not escape
//
// The arithmetic is IEEE-754's, as §10.9.2 asks: zero is signed, a division by
// zero is an infinity, and NaN infects everything it meets. None of the three
// escapes a top-level calculation — a NaN is censored to zero and an infinity
// to the largest value the context holds — and that happens where a program's
// result is handed out, never inside it, so that "calc(1 / calc(-5 * 0))" is
// minus infinity rather than plus.
//
// A length is carried in layout units, a sixty-fourth of a pixel, as a float:
// each dimension is quantized as it is read and each product as it is made,
// exactly as Unit's own arithmetic does, so that an expression calc() has
// always evaluated comes out the same to the unit. What the float adds is
// room: an infinity and a NaN, which a Unit cannot hold and §10.9.2 needs.
//
// # A percentage that cannot be settled here
//
// "calc(100% - 2em)" is linear, and carried to layout as an absolute part and a
// percentage — LengthCalc. "min(50%, 300px)" is not: which argument is smaller
// depends on what the percentage is of, and that is a containing block nobody
// has yet. A program with a percentage under anything but "+", "-", "*" and "/"
// is deferred: a length holds the program itself, as LengthMath, interned so
// that the Length stays comparable, and Resolve runs it once the basis is
// known. A reader of an angle-percentage has no basis to give and refuses one.

// mathType is what a calculation computes to.
type mathType struct {
	kind mathKind
	// pct says a percentage is in it, resolved against kind.
	pct bool
}

// addTypes is §10.9's type addition, which is also what "consistent type"
// means for the arguments of min() and its family.
func addTypes(a, b mathType) (mathType, bool) {
	if a.kind != b.kind {
		return mathType{}, false
	}
	return mathType{kind: a.kind, pct: a.pct || b.pct}, true
}

// mathScope is what a program is read against.
type mathScope struct {
	// ctx resolves the relative lengths.
	ctx LengthContext
	// typecheck says only the type is wanted: there is no element, so no unit
	// is resolved and a unit this engine cannot resolve makes the program
	// unevaluable rather than invalid.
	typecheck bool
	// pctAs is what a percentage resolves against: kindLength, kindAngle, or
	// kindPercent for a context where it stays a percentage of its own.
	pctAs mathKind
}

// mathProgram is a math function, read.
type mathProgram struct {
	code string
	typ  mathType
	// deferred says a percentage is under a function that is not linear in
	// it, so the program cannot be evaluated without the basis.
	deferred bool
	// evaluable says every value in it is one this engine resolves. It is
	// false only for a typecheck-mode program: an evaluating read refuses one.
	evaluable bool
}

// The program's instructions, in prefix order. Every node is an opcode and
// the kind of what it computes, then what the opcode carries, then its
// operands.
const (
	opNum   byte = iota + 1 // a value: 8 bytes
	opPct                   // a percentage of the basis: 8 bytes
	opAdd                   // two operands
	opSub                   //
	opMul                   //
	opDiv                   //
	opMin                   // a 4-byte count, then that many operands
	opMax                   //
	opHypot                 //
	opClamp                 // three operands, either end of which may be opNone
	opNone                  // no operands
	opRound                 // a strategy byte, then two operands
	opMod                   // two operands
	opRem                   //
	opAbs                   // one operand
	opSign                  //
	opSin                   // a byte that is 1 when the operand is in degrees, then it
	opCos                   //
	opTan                   //
	opAsin                  // one operand
	opAcos                  //
	opAtan                  //
	opAtan2                 // two operands
	opPow                   //
	opSqrt                  // one operand
	opExp                   //
	opLog                   // a count byte, then one or two operands
)

// The rounding strategies, as round()'s strategy byte.
const (
	roundNearest byte = iota
	roundUp
	roundDown
	roundToZero
)

// isMathFunction reports whether a component is one of CSS's math functions.
func isMathFunction(v css.ComponentValue) bool {
	return v.IsFunction() && mathFunctions[ascii.Lower(v.Token.Value)]
}

// compileMath reads one math function.
func compileMath(fn css.ComponentValue, s mathScope) (mathProgram, bool) {
	c := mathCompiler{scope: s, evaluable: true}
	n, ok := c.function(fn)
	if !ok {
		return mathProgram{}, false
	}
	return mathProgram{code: string(n.code), typ: n.typ, deferred: c.deferred,
		evaluable: c.evaluable}, true
}

// mathNode is a compiled operand.
type mathNode struct {
	code []byte
	typ  mathType
}

type mathCompiler struct {
	scope     mathScope
	deferred  bool
	evaluable bool
}

func node(op byte, k mathKind, rest ...[]byte) []byte {
	out := []byte{op, byte(k)}
	for _, r := range rest {
		out = append(out, r...)
	}
	return out
}

func leaf(op byte, k mathKind, v float64) []byte {
	return binary.LittleEndian.AppendUint64([]byte{op, byte(k)}, math.Float64bits(v))
}

// whole reads a calculation that must use every component it is given.
func (c *mathCompiler) whole(vals []css.ComponentValue) (mathNode, bool) {
	n, rest, ok := c.sum(vals)
	if !ok || len(skipSpace(rest)) != 0 {
		return mathNode{}, false
	}
	return n, true
}

// sum is the "+" and "-" level: the loosest-binding one, so it is read last
// and calls into the tighter one for each of its operands.
func (c *mathCompiler) sum(vals []css.ComponentValue) (mathNode, []css.ComponentValue, bool) {
	left, rest, ok := c.product(vals)
	if !ok {
		return mathNode{}, nil, false
	}
	for {
		// CSS requires white space around + and -, and the requirement is not
		// decoration: without it "calc(1px -2px)" would be a subtraction or a
		// length followed by a negative length depending on which way you
		// squint, and the tokenizer has already chosen the second. So an
		// operator is a delimiter with space on *both* sides, and anything else
		// ends the sum — "calc(1px +-2px)" is the mistake it looks like, not
		// minus one pixel.
		after := skipSpace(rest)
		if len(after) == len(rest) || len(after) == 0 {
			return left, rest, true
		}
		op, isOp := calcOperator(after[0], "+-")
		if !isOp {
			return left, rest, true
		}
		if tail := after[1:]; len(tail) == 0 || !tail[0].IsToken() ||
			tail[0].Token.Kind != css.Whitespace {
			return mathNode{}, nil, false
		}
		right, more, ok := c.product(after[1:])
		if !ok {
			return mathNode{}, nil, false
		}
		typ, ok := addTypes(left.typ, right.typ)
		if !ok {
			return mathNode{}, nil, false
		}
		code := opAdd
		if op == '-' {
			code = opSub
		}
		left = mathNode{code: node(code, typ.kind, left.code, right.code), typ: typ}
		rest = more
	}
}

// product is the "*" and "/" level, which binds tighter and needs no space
// around its operators.
func (c *mathCompiler) product(vals []css.ComponentValue) (mathNode, []css.ComponentValue, bool) {
	left, rest, ok := c.value(vals)
	if !ok {
		return mathNode{}, nil, false
	}
	for {
		after := skipSpace(rest)
		if len(after) == 0 {
			return left, rest, true
		}
		op, isOp := calcOperator(after[0], "*/")
		if !isOp {
			return left, rest, true
		}
		right, more, ok := c.value(after[1:])
		if !ok {
			return mathNode{}, nil, false
		}
		pct := left.typ.pct || right.typ.pct
		var typ mathType
		switch {
		case op == '/' && right.typ.kind != kindNumber:
			return mathNode{}, nil, false
		case right.typ.kind == kindNumber:
			typ = mathType{kind: left.typ.kind, pct: pct}
		case left.typ.kind == kindNumber:
			typ = mathType{kind: right.typ.kind, pct: pct}
		default:
			// A length times a length is an area, and there is nowhere in
			// CSS to put one.
			return mathNode{}, nil, false
		}
		code := opMul
		if op == '/' {
			code = opDiv
		} else if right.typ.kind != kindNumber {
			// The evaluator scales the left operand by the right one, so
			// the number goes on the right; a product commutes exactly.
			left, right = right, left
		}
		left = mathNode{code: node(code, typ.kind, left.code, right.code), typ: typ}
		rest = more
	}
}

// value is one operand: a number, a dimension, a percentage, a constant, a
// parenthesised sum, or a math function.
func (c *mathCompiler) value(vals []css.ComponentValue) (mathNode, []css.ComponentValue, bool) {
	vals = skipSpace(vals)
	if len(vals) == 0 {
		return mathNode{}, nil, false
	}
	v := vals[0]
	switch {
	case v.IsBlock() && v.Token.Kind == css.LeftParen:
		n, ok := c.whole(v.Values)
		return n, vals[1:], ok
	case v.IsFunction():
		n, ok := c.function(v)
		return n, vals[1:], ok
	case !v.IsToken():
		return mathNode{}, nil, false
	}
	n, ok := c.token(v.Token)
	return n, vals[1:], ok
}

// token is a number, a dimension, a percentage or one of §10.7's constants.
func (c *mathCompiler) token(t css.Token) (mathNode, bool) {
	// "Signed zeros can not be written directly in CSS; 0, +0 and -0 all
	// produce the standard unsigned zero" — §10.9.2. Adding zero is what
	// makes a written -0 the positive one.
	t.Number += 0
	number := func(v float64) (mathNode, bool) {
		return mathNode{code: leaf(opNum, kindNumber, v), typ: mathType{kind: kindNumber}}, true
	}
	switch t.Kind {
	case css.Number:
		return number(t.Number)

	case css.Percentage:
		if c.scope.pctAs == kindPercent {
			return mathNode{code: leaf(opNum, kindPercent, t.Number),
				typ: mathType{kind: kindPercent}}, true
		}
		return mathNode{code: leaf(opPct, c.scope.pctAs, t.Number),
			typ: mathType{kind: c.scope.pctAs, pct: true}}, true

	case css.Dimension:
		k := unitKind(t.Unit)
		typ := mathType{kind: k}
		switch k {
		case kindNone:
			return mathNode{}, false
		case kindAngle:
			deg, _ := degreesPer(t.Unit)
			return mathNode{code: leaf(opNum, k, t.Number*deg), typ: typ}, true
		case kindLength:
			px, known, supported := pxPerUnit(t.Unit, c.scope.ctx)
			if c.scope.typecheck {
				// The value is not wanted, only whether one could be had:
				// "1lh" is a length nothing here resolves, and "1vw" is one
				// that waits for the page.
				c.evaluable = c.evaluable && supported
				return mathNode{code: leaf(opNum, k, 0), typ: typ}, true
			}
			if !supported || !known {
				return mathNode{}, false
			}
			u, fits := FromPx(t.Number * px)
			if !fits {
				return mathNode{}, false
			}
			return mathNode{code: leaf(opNum, k, float64(u)), typ: typ}, true
		}
		// A time, a frequency, a resolution or a flex: types this engine
		// knows and reads nowhere.
		if !c.scope.typecheck {
			return mathNode{}, false
		}
		c.evaluable = false
		return mathNode{code: leaf(opNum, k, 0), typ: typ}, true

	case css.Ident:
		// §10.7: ASCII case-insensitive, like every CSS keyword.
		switch ascii.Lower(t.Value) {
		case "e":
			return number(math.E)
		case "pi":
			return number(math.Pi)
		case "infinity":
			return number(math.Inf(1))
		case "-infinity":
			return number(math.Inf(-1))
		case "nan":
			return number(math.NaN())
		}
	}
	return mathNode{}, false
}

// function is a math function: its arguments, their types, and its own.
func (c *mathCompiler) function(fn css.ComponentValue) (mathNode, bool) {
	name := ascii.Lower(fn.Token.Value)
	if !mathFunctions[name] {
		// A substitution function inside a calculation — "calc(env(x) +
		// 1px)" — cannot be typed before it is substituted, and nothing
		// here substitutes one.
		return mathNode{}, false
	}
	args := splitOnComma(fn.Values)
	if name == "calc" {
		if len(args) != 1 {
			return mathNode{}, false
		}
		return c.whole(args[0])
	}

	// Every other function decides between, or bends, its arguments, so
	// none of them is linear in a percentage among them.
	read := func(vals []css.ComponentValue) (mathNode, bool) {
		n, ok := c.whole(vals)
		if ok && n.typ.pct {
			c.deferred = true
		}
		return n, ok
	}
	readAll := func(vals [][]css.ComponentValue) ([]mathNode, mathType, bool) {
		out := make([]mathNode, 0, len(vals))
		var typ mathType
		for i, a := range vals {
			n, ok := read(a)
			if !ok {
				return nil, mathType{}, false
			}
			if i == 0 {
				typ = n.typ
			} else if typ, ok = addTypes(typ, n.typ); !ok {
				return nil, mathType{}, false
			}
			out = append(out, n)
		}
		return out, typ, true
	}
	join := func(op byte, typ mathType, extra []byte, ns []mathNode) (mathNode, bool) {
		code := append(node(op, typ.kind), extra...)
		for _, n := range ns {
			code = append(code, n.code...)
		}
		return mathNode{code: code, typ: typ}, true
	}
	number := func(t mathType) mathType { return mathType{kind: kindNumber, pct: t.pct} }
	angle := func(t mathType) mathType { return mathType{kind: kindAngle, pct: t.pct} }

	switch name {
	case "min", "max", "hypot":
		ns, typ, ok := readAll(args)
		if !ok {
			return mathNode{}, false
		}
		op := map[string]byte{"min": opMin, "max": opMax, "hypot": opHypot}[name]
		return join(op, typ, binary.LittleEndian.AppendUint32(nil, uint32(len(ns))), ns)

	case "clamp":
		if len(args) != 3 {
			return mathNode{}, false
		}
		ns := make([]mathNode, 3)
		var typ mathType
		seen := false
		for i, a := range args {
			if s, ok := singleIdent(a); ok && s == "none" && i != 1 {
				ns[i] = mathNode{code: node(opNone, kindNone)}
				continue
			}
			n, ok := read(a)
			if !ok {
				return mathNode{}, false
			}
			if !seen {
				typ, seen = n.typ, true
			} else if typ, ok = addTypes(typ, n.typ); !ok {
				return mathNode{}, false
			}
			ns[i] = n
		}
		return join(opClamp, typ, nil, ns)

	case "round":
		strategy := roundNearest
		if len(args) > 0 {
			if s, ok := singleIdent(args[0]); ok {
				switch s {
				case "nearest":
					args = args[1:]
				case "up":
					strategy, args = roundUp, args[1:]
				case "down":
					strategy, args = roundDown, args[1:]
				case "to-zero":
					strategy, args = roundToZero, args[1:]
				case "line-width":
					// Snapped as a border width is, to device pixels, and
					// a page has none of those: valid, and not evaluated.
					args = args[1:]
					ns, typ, ok := readAll(args)
					if !ok || len(ns) < 1 || len(ns) > 2 || typ.kind != kindLength ||
						!c.scope.typecheck {
						return mathNode{}, false
					}
					c.evaluable = false
					return join(opRound, typ, []byte{roundNearest}, ns)
				}
			}
		}
		if len(args) != 1 && len(args) != 2 {
			return mathNode{}, false
		}
		ns, typ, ok := readAll(args)
		if !ok {
			return mathNode{}, false
		}
		if len(ns) == 1 {
			// B may be left out only when A is a number, and is then 1.
			if typ.kind != kindNumber {
				return mathNode{}, false
			}
			ns = append(ns, mathNode{code: leaf(opNum, kindNumber, 1)})
		}
		return join(opRound, typ, []byte{strategy}, ns)

	case "mod", "rem", "atan2":
		if len(args) != 2 {
			return mathNode{}, false
		}
		ns, typ, ok := readAll(args)
		if !ok {
			return mathNode{}, false
		}
		switch name {
		case "mod":
			return join(opMod, typ, nil, ns)
		case "rem":
			return join(opRem, typ, nil, ns)
		}
		return join(opAtan2, angle(typ), nil, ns)

	case "abs", "sign":
		if len(args) != 1 {
			return mathNode{}, false
		}
		ns, typ, ok := readAll(args)
		if !ok {
			return mathNode{}, false
		}
		if name == "abs" {
			return join(opAbs, typ, nil, ns)
		}
		return join(opSign, number(typ), nil, ns)

	case "sin", "cos", "tan":
		if len(args) != 1 {
			return mathNode{}, false
		}
		ns, typ, ok := readAll(args)
		if !ok || (typ.kind != kindNumber && typ.kind != kindAngle) {
			return mathNode{}, false
		}
		var degrees byte
		if typ.kind == kindAngle {
			degrees = 1
		}
		op := map[string]byte{"sin": opSin, "cos": opCos, "tan": opTan}[name]
		return join(op, number(typ), []byte{degrees}, ns)

	case "asin", "acos", "atan", "sqrt", "exp":
		if len(args) != 1 {
			return mathNode{}, false
		}
		ns, typ, ok := readAll(args)
		if !ok || typ.kind != kindNumber {
			return mathNode{}, false
		}
		switch name {
		case "sqrt":
			return join(opSqrt, typ, nil, ns)
		case "exp":
			return join(opExp, typ, nil, ns)
		}
		op := map[string]byte{"asin": opAsin, "acos": opAcos, "atan": opAtan}[name]
		return join(op, angle(typ), nil, ns)

	case "pow", "log":
		if (name == "pow" && len(args) != 2) || (name == "log" && len(args) != 1 && len(args) != 2) {
			return mathNode{}, false
		}
		ns, typ, ok := readAll(args)
		if !ok || typ.kind != kindNumber {
			return mathNode{}, false
		}
		if name == "pow" {
			return join(opPow, typ, nil, ns)
		}
		return join(opLog, typ, []byte{byte(len(ns))}, ns)
	}
	return mathNode{}, false
}

// mathValue is a value part-way through a program: so much of the context's
// unit and so many per cent of its basis. Which of the two halves are present
// is carried as well, because an absent half is not a zero one: "calc(50% *
// infinity)" has no length in it to multiply, and multiplying the zero that
// stands for none would make a NaN out of nothing.
type mathValue struct {
	v, pct       float64
	hasV, hasPct bool
}

// mathBasis is what a percentage is of, when it is known: in layout units for
// a length, in degrees for an angle.
type mathBasis struct {
	of    float64
	known bool
}

// evalMath runs a program. ok is false only for a program that needs a basis
// it was not given.
func evalMath(code string, basis mathBasis) (mathValue, bool) {
	e := mathEval{code: code, basis: basis}
	v := e.node()
	return v, !e.failed && e.at == len(code)
}

type mathEval struct {
	code   string
	at     int
	basis  mathBasis
	failed bool
}

func (e *mathEval) next() byte {
	if e.at >= len(e.code) {
		e.failed = true
		return 0
	}
	b := e.code[e.at]
	e.at++
	return b
}

func (e *mathEval) float() float64 {
	if e.at+8 > len(e.code) {
		e.failed = true
		return 0
	}
	var bits uint64
	for i := 7; i >= 0; i-- {
		bits = bits<<8 | uint64(e.code[e.at+i])
	}
	e.at += 8
	return math.Float64frombits(bits)
}

func (e *mathEval) count() int {
	if e.at+4 > len(e.code) {
		e.failed = true
		return 0
	}
	n := int(e.code[e.at]) | int(e.code[e.at+1])<<8 | int(e.code[e.at+2])<<16 |
		int(e.code[e.at+3])<<24
	e.at += 4
	return n
}

// scalar is an operand of a function that is not linear: one number, which a
// percentage may not still be part of.
func (e *mathEval) scalar() float64 {
	v := e.node()
	if v.hasPct {
		// A percentage reaches here only in a deferred program, and one of
		// those is run only once the basis is known, when there are none.
		e.failed = true
	}
	return v.v
}

func plain(v float64) mathValue { return mathValue{v: v, hasV: true} }

// node evaluates the node at the cursor.
func (e *mathEval) node() mathValue {
	if e.failed {
		return mathValue{}
	}
	op, kind := e.next(), mathKind(e.next())
	out := e.apply(op, kind)
	if kind == kindLength {
		// A length is a whole number of units, taken towards zero, after
		// every step that can make a fraction of one — as Unit.Mul does.
		out.v = math.Trunc(out.v)
	}
	return out
}

func (e *mathEval) apply(op byte, kind mathKind) mathValue {
	switch op {
	case opNum:
		return plain(e.float())
	case opPct:
		p := e.float()
		if e.basis.known {
			return plain(e.basis.of * (p / 100))
		}
		return mathValue{pct: p, hasPct: true}

	case opAdd, opSub:
		a, b := e.node(), e.node()
		if op == opSub {
			b.v, b.pct = -b.v, -b.pct
		}
		out := mathValue{hasV: a.hasV || b.hasV, hasPct: a.hasPct || b.hasPct}
		out.v = sumOf(a.v, a.hasV, b.v, b.hasV)
		out.pct = sumOf(a.pct, a.hasPct, b.pct, b.hasPct)
		return out

	case opMul, opDiv:
		// The number is on the right: the compiler puts it there.
		x, n := e.node(), e.node()
		f := func(v float64) float64 {
			if op == opDiv {
				if kind == kindLength {
					// As Unit.Div: a multiplication by the reciprocal.
					return v * (1 / n.v)
				}
				return v / n.v
			}
			return v * n.v
		}
		out := mathValue{hasV: x.hasV, hasPct: x.hasPct}
		if x.hasV {
			out.v = f(x.v)
		}
		if x.hasPct {
			out.pct = f(x.pct)
		}
		return out

	case opMin, opMax, opHypot:
		n := e.count()
		acc := 0.0
		nan, inf := false, false
		for i := 0; i < n; i++ {
			v := e.scalar()
			switch {
			case math.IsNaN(v):
				nan = true
			case math.IsInf(v, 0):
				inf = true
			}
			switch {
			case i == 0 && op != opHypot:
				acc = v
			case op == opMin:
				acc = math.Min(acc, v)
			case op == opMax:
				acc = math.Max(acc, v)
			default:
				acc = math.Hypot(acc, v)
			}
		}
		switch {
		case nan:
			// NaN infects every function, hypot() included, which in Go
			// and in JavaScript lets an infinity win over it.
			return plain(math.NaN())
		case op == opHypot && inf:
			return plain(math.Inf(1))
		}
		return plain(acc)

	case opClamp:
		lo, hasLo := e.optional()
		v := e.scalar()
		hi, hasHi := e.optional()
		// max(MIN, min(VAL, MAX)), in that order, which is what makes MIN
		// win when the two are the wrong way round.
		if hasHi {
			v = math.Min(v, hi)
		}
		if hasLo {
			v = math.Max(lo, v)
		}
		return plain(v)

	case opRound:
		strategy := e.next()
		a, b := e.scalar(), e.scalar()
		return plain(roundTo(strategy, a, b))

	case opMod, opRem:
		a, b := e.scalar(), e.scalar()
		return plain(modulus(op == opMod, a, b))

	case opAbs:
		v := e.scalar()
		if v > 0 || v == 0 && !math.Signbit(v) {
			return plain(v)
		}
		return plain(-v)

	case opSign:
		v := e.scalar()
		switch {
		case v > 0:
			return plain(1)
		case v < 0:
			return plain(-1)
		}
		return plain(v) // ±0, or NaN

	case opSin, opCos, opTan:
		degrees := e.next() == 1
		v := e.scalar()
		return plain(trig(op, v, degrees))

	case opAsin, opAcos, opAtan:
		v := e.scalar()
		var r float64
		switch op {
		case opAsin:
			r = math.Asin(v)
		case opAcos:
			r = math.Acos(v)
		default:
			r = math.Atan(v)
		}
		return plain(r * 180 / math.Pi)

	case opAtan2:
		y, x := e.scalar(), e.scalar()
		// Go's Atan2 is C99's, whose table for the zeros and the infinities
		// is the one §10.4.1 gives.
		return plain(math.Atan2(y, x) * 180 / math.Pi)

	case opPow:
		a, b := e.scalar(), e.scalar()
		return plain(pow(a, b))

	case opSqrt:
		return plain(math.Sqrt(e.scalar()))

	case opExp:
		return plain(math.Exp(e.scalar()))

	case opLog:
		n := e.next()
		a := e.scalar()
		if n == 1 {
			return plain(math.Log(a))
		}
		b := e.scalar()
		return plain(logBase(a, b))
	}
	e.failed = true
	return mathValue{}
}

// sumOf adds two halves, either of which may be absent: an absent half is not
// a zero one, and in particular not a positive zero, which would turn a
// negative zero into a positive one.
func sumOf(a float64, hasA bool, b float64, hasB bool) float64 {
	switch {
	case hasA && hasB:
		return a + b
	case hasA:
		return a
	case hasB:
		return b
	}
	return 0
}

// optional is one end of a clamp(), which "none" leaves open.
func (e *mathEval) optional() (float64, bool) {
	if e.at < len(e.code) && e.code[e.at] == opNone {
		e.at += 2
		return 0, false
	}
	return e.scalar(), true
}

// roundTo is round(), §10.3, with the argument ranges of §10.3.1.
func roundTo(strategy byte, a, b float64) float64 {
	negZero := math.Copysign(0, -1)
	switch {
	case math.IsNaN(a) || math.IsNaN(b) || b == 0:
		return math.NaN()
	case math.IsInf(a, 0) && math.IsInf(b, 0):
		return math.NaN()
	case math.IsInf(a, 0):
		return a
	case math.IsInf(b, 0):
		positiveOrZero := a > 0 || a == 0 && !math.Signbit(a)
		switch strategy {
		case roundUp:
			switch {
			case a > 0:
				return math.Inf(1)
			case positiveOrZero:
				return 0
			}
			return negZero
		case roundDown:
			switch {
			case a < 0:
				return math.Inf(-1)
			case a == 0 && math.Signbit(a):
				return negZero
			}
			return 0
		}
		if positiveOrZero {
			return 0
		}
		return negZero
	}
	step := math.Abs(b)
	if math.Mod(a, step) == 0 {
		// A multiple of B already, and A exactly — its zero's sign too.
		return a
	}
	k := math.Floor(a / step)
	lower, upper := k*step, (k+1)*step
	// a/step is rounded, and can land on the integer above a value just short
	// of it; the two multiples have to be either side of A.
	if lower > a {
		lower, upper = (k-1)*step, k*step
	} else if upper < a {
		lower, upper = upper, (k+2)*step
	}
	// A multiple that is zero is 0⁺ as the lower one and 0⁻ as the upper.
	if lower == 0 {
		lower = 0
	}
	if upper == 0 {
		upper = negZero
	}
	switch strategy {
	case roundUp:
		return upper
	case roundDown:
		return lower
	case roundToZero:
		if math.Abs(lower) < math.Abs(upper) {
			return lower
		}
		return upper
	}
	if upper-a <= a-lower {
		return upper
	}
	return lower
}

// modulus is mod() and rem(), §10.3, with the argument ranges of §10.3.1.
// rem() keeps the sign of A, which is what Go's Mod does; mod() takes the
// sign of B.
func modulus(isMod bool, a, b float64) float64 {
	switch {
	case math.IsNaN(a) || math.IsNaN(b) || b == 0 || math.IsInf(a, 0):
		return math.NaN()
	case math.IsInf(b, 0):
		if isMod && math.Signbit(a) != math.Signbit(b) {
			return math.NaN()
		}
		return a
	}
	r := math.Mod(a, b)
	if !isMod {
		return r
	}
	if r == 0 {
		// The range starts at 0⁺ when B is positive and at 0⁻ when it is
		// negative.
		return math.Copysign(0, b)
	}
	if math.Signbit(r) != math.Signbit(b) {
		r += b
	}
	return r
}

// trig is sin(), cos() and tan() of radians, or of degrees when the argument
// is an angle.
//
// tan() at an asymptote is implementation-defined, and §10.4.1 asks an
// implementation that can represent the input exactly to give +∞ at 90deg +
// N·360deg and −∞ at -90deg + N·360deg. An angle written in degrees is exact
// here, so it does.
func trig(op byte, v float64, degrees bool) float64 {
	if degrees {
		if op == opTan && !math.IsInf(v, 0) && !math.IsNaN(v) {
			switch r := math.Mod(v, 360); r {
			case 90, -270:
				return math.Inf(1)
			case -90, 270:
				return math.Inf(-1)
			}
		}
		// sin(0⁻) and tan(0⁻) are 0⁻, and the conversion keeps the sign.
		v = v * math.Pi / 180
	}
	switch op {
	case opSin:
		return math.Sin(v)
	case opCos:
		return math.Cos(v)
	}
	return math.Tan(v)
}

// pow is pow(), §10.5, which is Go's Pow but for the three places the
// specification and C99 part: NaN infects even pow(NaN, 0), and a base of 1 or
// -1 raised to an infinity is NaN rather than 1.
func pow(a, b float64) float64 {
	switch {
	case math.IsNaN(a) || math.IsNaN(b):
		return math.NaN()
	case math.IsInf(b, 0) && (a == 1 || a == -1):
		return math.NaN()
	}
	return math.Pow(a, b)
}

// logBase is log(A, B), §10.5.1: a base of 1 or below zero is NaN, and the
// logarithm of 1 is 0⁺ whatever the base.
func logBase(a, b float64) float64 {
	switch {
	case math.IsNaN(a) || math.IsNaN(b) || b == 1 || b < 0:
		return math.NaN()
	case a == 1:
		return 0
	}
	return math.Log(a) / math.Log(b)
}

// censoredUnit is a top-level length's numeric part as a Unit: §10.9.2's NaN
// is zero, and an infinity, or anything past the range, the end of the range.
func censoredUnit(v float64) Unit {
	switch {
	case math.IsNaN(v):
		return 0
	case v >= float64(MaxUnit):
		return MaxUnit
	case v <= float64(MinUnit):
		return MinUnit
	}
	return Unit(v)
}
