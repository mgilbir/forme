# Builds the CFF face cffink.py measures that no foundry made, into the
# directory it is given:
#
#   make hbcffink
#
# shape/cffink.go measures a CFF glyph's ink by running its charstring as
# HarfBuzz runs it, and the real CFF faces in the corpora — Unifont and the
# Noto CJK faces — exercise only part of what that reads: lines, curves, hints
# and subroutines, the way one font compiler writes them. This face is every
# other thing a charstring can say, one glyph each, so that no rule is held to
# HarfBuzz only by this package's own reading of HarfBuzz:
#
#   - every path operator, including the argument counts the specification
#     does not provide for, where what HarfBuzz does with the leftovers is the
#     answer (vhcurveto and hvcurveto at every count modulo eight, rcurveline
#     and rlinecurve short of eight, the odd leading argument of vvcurveto and
#     hhcurveto);
#   - the four flex operators, flex1 both ways, and each with a wrong count;
#   - a width in front of each operator that may carry one;
#   - hints: stems, the implicit vertical stems before a first hintmask, a mask
#     whose length is not recomputed for stems declared after it, and one the
#     charstring has no room for;
#   - numbers in every encoding, 16.16 fixed ones on a half, which HarfBuzz
#     rounds up where the C library would round away from zero, a fixed number
#     too short to be one, and a coordinate past 2^24, where HarfBuzz's scaling
#     through a float loses the last unit;
#   - local and global subroutines, nested to the limit and past it, called out
#     of range, and one that ends without returning;
#   - seac: an accented glyph, one with a path of its own, one whose base is a
#     line and so an empty box that the merge replaces, one whose own path is a
#     line that the merge drops, one naming a code with no glyph, and one whose
#     base is itself a seac;
#   - errors: the stack popped empty and pushed past its size, a return with
#     nothing to return to, and a charstring with no endchar;
#   - HarfBuzz's cap on one charstring: a glyph of 199,999 operators, which
#     has ink, and one of 200,000, which has none;
#   - a glyph that draws nothing, one that only moves, and one that is a
#     horizontal line.
#
# It is built with fontTools and its timestamps fixed, so that building it
# again produces the same bytes and the checksum the expectations record stays
# true.
import os
import struct
import sys

from oracle import fonttools

fonttools()
from fontTools.cffLib import SubrsIndex  # noqa: E402
from fontTools.fontBuilder import FontBuilder  # noqa: E402
from fontTools.misc.psCharStrings import T2CharString  # noqa: E402

# Operators, by the bytes they are written as.
OPS = {
    "hstem": [1], "vstem": [3], "vmoveto": [4], "rlineto": [5], "hlineto": [6],
    "vlineto": [7], "rrcurveto": [8], "callsubr": [10], "return": [11],
    "endchar": [14], "hstemhm": [18], "hintmask": [19], "cntrmask": [20],
    "rmoveto": [21], "hmoveto": [22], "vstemhm": [23], "rcurveline": [24],
    "rlinecurve": [25], "vvcurveto": [26], "hhcurveto": [27], "callgsubr": [29],
    "vhcurveto": [30], "hvcurveto": [31], "dotsection": [12, 0], "and": [12, 3],
    "add": [12, 10], "hflex": [12, 34], "flex": [12, 35], "hflex1": [12, 36],
    "flex1": [12, 37], "reserved2": [2], "reserved9": [9], "reserved13": [13],
    "vsindex": [15], "blend": [16], "reserved17": [17],
}


def num(v):
    """A number in the shortest Type 2 encoding, or a 16.16 fixed one for a
    number that is not whole."""
    if isinstance(v, float) and v != int(v):
        return bytes([255]) + struct.pack(">i", round(v * 65536))
    v = int(v)
    if -107 <= v <= 107:
        return bytes([v + 139])
    if 108 <= v <= 1131:
        v -= 108
        return bytes([(v >> 8) + 247, v & 0xFF])
    if -1131 <= v <= -108:
        v = -v - 108
        return bytes([(v >> 8) + 251, v & 0xFF])
    return bytes([28]) + struct.pack(">h", v)


def cs(*items):
    """A charstring: numbers, operator names, and raw bytes."""
    out = b""
    for it in items:
        if isinstance(it, bytes):
            out += it
        elif isinstance(it, str):
            out += bytes(OPS[it])
        else:
            out += num(it)
    return out


glyphs = {}


def glyph(name, code):
    glyphs[name] = code


# Every path operator.
glyph("rlineto", cs(10, 20, "rmoveto", 30, 40, -50, 60, 5, -300, "rlineto", "endchar"))
glyph("hlineto.even", cs(10, 20, "rmoveto", 100, 50, -30, 70, "hlineto", "endchar"))
glyph("hlineto.odd", cs(10, 20, "rmoveto", 100, 50, -30, "hlineto", "endchar"))
glyph("vlineto.even", cs(10, 20, "rmoveto", 100, 50, -30, 70, "vlineto", "endchar"))
glyph("vlineto.odd", cs(10, 20, "rmoveto", 100, 50, -30, "vlineto", "endchar"))
glyph("rrcurveto", cs(0, 0, "rmoveto", 10, 300, 200, 50, 30, -400,
                      -50, -80, -60, 20, -10, 10, "rrcurveto", "endchar"))
glyph("rrcurveto.short", cs(0, 0, "rmoveto", 10, 300, 200, 50, 30, -400, 7, 7, "rrcurveto", "endchar"))
glyph("rcurveline", cs(5, 5, "rmoveto", 10, 20, 30, 40, 50, -60, 70, 80, "rcurveline", "endchar"))
glyph("rcurveline.two", cs(5, 5, "rmoveto", 10, 20, 30, 40, 50, -60, 1, 2, 3, 4, 5, 6,
                           70, 80, 9, "rcurveline", "endchar"))
glyph("rcurveline.short", cs(5, 5, "rmoveto", 10, 20, 30, 40, 50, -60, 70, "rcurveline",
                             100, 100, "rlineto", "endchar"))
glyph("rlinecurve", cs(5, 5, "rmoveto", 70, 80, 10, 20, 30, 40, 50, -60, "rlinecurve", "endchar"))
glyph("rlinecurve.two", cs(5, 5, "rmoveto", 70, 80, -5, 300, 10, 20, 30, 40, 50, -60, 9,
                           "rlinecurve", "endchar"))
glyph("rlinecurve.short", cs(5, 5, "rmoveto", 70, 80, 10, 20, 30, 40, 50, "rlinecurve",
                             100, 100, "rlineto", "endchar"))
glyph("vvcurveto.even", cs(0, 0, "rmoveto", 100, 50, 60, 70, 80, 90, 100, 110, "vvcurveto", "endchar"))
glyph("vvcurveto.odd", cs(0, 0, "rmoveto", -40, 100, 50, 60, 70, 80, 90, 100, 110, "vvcurveto", "endchar"))
glyph("hhcurveto.even", cs(0, 0, "rmoveto", 100, 50, 60, 70, 80, 90, 100, 110, "hhcurveto", "endchar"))
glyph("hhcurveto.odd", cs(0, 0, "rmoveto", -40, 100, 50, 60, 70, 80, 90, 100, 110, "hhcurveto", "endchar"))
for op in ("vhcurveto", "hvcurveto"):
    for n in (3, 4, 5, 8, 9, 10, 12, 13, 16, 17, 20, 21, 24, 25):
        vals = [(-1) ** k * (30 + 17 * k) for k in range(n)]
        glyph("%s.%d" % (op, n), cs(0, 0, "rmoveto", *vals, op, "endchar"))

# The flex operators, and each with an argument too many.
FLEX = {
    "hflex": [20, 30, 40, 50, 60, -70, 80],
    "flex": [10, 20, 30, 40, 50, 60, 70, -80, 90, -100, 110, -120, 50],
    "hflex1": [10, 20, 30, 40, 50, 60, 70, -80, 90],
    "flex1": [10, 20, 30, 40, 50, 60, 70, -80, 90, 10, 55],
}
for op, vals in FLEX.items():
    glyph(op, cs(100, 100, "rmoveto", *vals, op, "endchar"))
    glyph(op + ".wrong", cs(100, 100, "rmoveto", *vals, 5, op, "endchar"))
glyph("flex1.vertical", cs(100, 100, "rmoveto", 10, 20, 30, 40, 50, 60, -70, 80, -10, 90, 55,
                           "flex1", "endchar"))

# Widths, in front of every operator that may carry one.
glyph("width.hmoveto", cs(500, 40, "hmoveto", 0, 100, "rlineto", "endchar"))
glyph("width.vmoveto", cs(500, 40, "vmoveto", 100, 0, "rlineto", "endchar"))
glyph("width.rmoveto", cs(500, 40, 50, "rmoveto", 100, 100, "rlineto", "endchar"))
glyph("width.hstem", cs(500, 10, 20, "hstem", 40, 50, "rmoveto", 100, 100, "rlineto", "endchar"))
glyph("width.endchar", cs(500, "endchar"))

# Hints. The mask after two pairs of stems is one byte; the implicit vstems in
# front of the first mask are counted; the stems after it are not, so the
# second mask is read at the first one's length.
glyph("hintmask", cs(10, 20, 30, 40, "hstemhm", 50, 60, "hintmask", b"\xe0",
                     5, 5, "rmoveto", 100, 200, "rlineto", "endchar"))
glyph("hintmask.nine", cs(1, 2, 3, 4, 5, 6, 7, 8, 9, 10, "hstemhm", 11, 12, 13, 14, 15, 16, 17, 18,
                          "hintmask", b"\xff\x80", 5, 5, "rmoveto", 100, 200, "rlineto", "endchar"))
glyph("hintmask.later", cs(10, 20, "hstemhm", "hintmask", b"\x80", 30, 40, 50, 60, 70, 80, 90, 100,
                           110, 120, 130, 140, 150, 160, 170, 180, "vstemhm", "hintmask", b"\x80",
                           5, 5, "rmoveto", 100, 200, "rlineto", "endchar"))
glyph("cntrmask", cs(10, 20, "hstem", "cntrmask", b"\x80", 5, 5, "rmoveto", 100, 200, "rlineto", "endchar"))
# A mask with no room: its operands stay, and the bytes after it are read as
# operators — here the endchar the mask would have swallowed.
glyph("hintmask.short", cs(0, 0, "rmoveto", 100, 200, "rlineto",
                           1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18,
                           "hstemhm", "hintmask", b"\x0e"))

# Numbers.
glyph("numbers", cs(0, 0, "rmoveto", 107, -107, 108, -108, 1131, -1131, 1132, -1132, 32767, -32768,
                    "rlineto", "endchar"))
glyph("fixed.half", cs(10.5, -10.5, "rmoveto", 100.5, 200.5, "rlineto", "endchar"))
glyph("fixed.fractions", cs(0.25, 0.75, "rmoveto", 99.4999847412109375, 0.5000152587890625,
                            -0.5, 0.49998474121, "rlineto", "endchar"))
glyph("fixed.short", cs(0, 0, "rmoveto", 100, 100, "rlineto", b"\xff\x8b\x8b\x0e"))
far = [cs(*([32767, 0] * 128), "rlineto") for _ in range(4)]
glyph("far", cs(0, 0, "rmoveto", *far, 513, 7, "rlineto", "endchar"))
glyph("far.left", cs(0, 0, "rmoveto", *[cs(*([-32768, 0] * 128), "rlineto") for _ in range(4)],
                     -1, 7, "rlineto", "endchar"))
glyph("shortint.truncated", cs(0, 0, "rmoveto", 100, 100, "rlineto", b"\x1c\x01"))

# Operators HarfBuzz does not run clear the stack and the run goes on.
glyph("reserved", cs(0, 0, "rmoveto", 100, 100, "rlineto", 5, 5, "reserved2", 7, "reserved9",
                     7, "reserved13", 7, "vsindex", 7, "blend", 7, "reserved17", 1, 2, "add",
                     1, 2, "and", "dotsection", 20, 20, "rlineto", "endchar"))

# Errors.
glyph("pop.empty", cs(5, "rmoveto", 100, 100, "rlineto", "endchar"))
glyph("stack.full", cs(0, 0, "rmoveto", *([1] * 514), "rlineto", "endchar"))
glyph("stack.fits", cs(0, 0, "rmoveto", *([1] * 512), "rlineto", "endchar"))
glyph("return.top", cs(0, 0, "rmoveto", 100, 100, "rlineto", "return", "endchar"))
glyph("no.endchar", cs(0, 0, "rmoveto", 100, 100, "rlineto"))
glyph("escape.last", cs(0, 0, "rmoveto", 100, 100, "rlineto", b"\x0c"))

# Nothing, a move, a line.
glyph("empty", cs("endchar"))
glyph("move", cs(100, 100, "rmoveto", 50, 50, "rmoveto", "endchar"))
glyph("line", cs(100, 100, "rmoveto", 300, 0, "rlineto", "endchar"))
glyph("two.paths", cs(100, 100, "rmoveto", 300, 0, "rlineto", -500, -500, "rmoveto",
                      0, 50, "rlineto", "endchar"))

# Subroutines. With fewer than 1240 local subroutines the bias is 107, and
# global ones likewise.
LOCAL = [
    cs(100, 0, 0, 100, "rlineto", "return"),                     # 0: a line pair
    cs(-107, "callsubr", 30, 30, "rlineto", "return"),           # 1: calls 0
    cs(20, 20, "rlineto"),                                       # 2: never returns
    cs(-104, "callsubr", "return"),                              # 3: calls 3: forever
    cs("endchar"),                                               # 4: ends the glyph
]
for k in range(5, 16):
    # 5..15: each calls the one before, down to 5, which draws.
    LOCAL.append(cs(10, 10, "rlineto", "return") if k == 5 else cs(k - 1 - 107, "callsubr", "return"))
GLOBAL = [
    cs(0, 0, "rmoveto", -107, "callsubr", "return"),             # 0: a move and local 0
    cs(-5, 7, "rlineto", "return"),                              # 1
]
glyph("subr.local", cs(0, 0, "rmoveto", -107, "callsubr", "endchar"))
glyph("subr.nested", cs(0, 0, "rmoveto", -106, "callsubr", "endchar"))
glyph("subr.global", cs(-107, "callgsubr", -106, "callgsubr", "endchar"))
glyph("subr.args", cs(0, 0, "rmoveto", 40, 40, -106, "callgsubr", "endchar"))
glyph("subr.noreturn", cs(0, 0, "rmoveto", -105, "callsubr", "endchar"))
glyph("subr.forever", cs(0, 0, "rmoveto", -104, "callsubr", "endchar"))
glyph("subr.endchar", cs(0, 0, "rmoveto", 50, 50, "rlineto", -103, "callsubr"))
glyph("subr.range", cs(0, 0, "rmoveto", 100, "callsubr", "endchar"))
glyph("subr.negative", cs(0, 0, "rmoveto", -108, "callsubr", "endchar"))
glyph("subr.fixed", cs(0, 0, "rmoveto", -106.75, "callsubr", "endchar"))
glyph("subr.depth10", cs(0, 0, "rmoveto", 14 - 107, "callsubr", "endchar"))  # 14..5: ten deep
glyph("subr.depth11", cs(0, 0, "rmoveto", 15 - 107, "callsubr", "endchar"))  # eleven
glyph("subr.empty", cs(0, 0, "rmoveto", "callsubr", "endchar"))

# seac: base and accent by StandardEncoding code — A is 65, acute 194, and
# code 1 names nothing.
glyph("A", cs(0, 0, "rmoveto", 300, 0, 150, 600, "rlineto", "endchar"))
glyph("acute", cs(0, 500, "rmoveto", 100, 150, 50, -30, "rlineto", "endchar"))
glyph("hyphen", cs(0, 250, "rmoveto", 200, 0, "rlineto", "endchar"))   # a line: empty box
glyph("Aacute", cs(100, 250, 65, 194, "endchar"))
glyph("Aacute.width", cs(600, 100, 250, 65, 194, "endchar"))
glyph("Aacute.path", cs(-50, -50, "rmoveto", 10, 10, "rlineto", 100, 250, 65, 194, "endchar"))
glyph("hyphen.acute", cs(20, 30, 45, 194, "endchar"))
glyph("acute.hyphen", cs(20, 30, 194, 45, "endchar"))
glyph("seac.nocode", cs(100, 250, 65, 1, "endchar"))
glyph("seac.zero", cs(0, 0, 0, 194, 0, 0, 0, 0, "endchar"))
glyph("B", cs(100, 250, 65, 194, "endchar"))    # a seac, named B so it can be a base
glyph("seac.ofseac", cs(0, 0, 66, 194, "endchar"))
glyph("seac.fixed", cs(10.5, 20.25, 65, 194, "endchar"))
# A path that is only a line is an empty box, which the seac's merge replaces
# rather than extends: the line is lost.
glyph("line.seac", cs(-100, -50, "rmoveto", 50, 0, "rlineto", 100, 250, 65, 194, "endchar"))

# HarfBuzz's cap on one charstring, 200,000 operators, which the one that
# reaches it fails: a glyph of 199,999 has ink and one of 200,000 has none.
# Subroutines 16, 17 and 18 cost 4, 61 and 631 operators a call, and a call
# two more; the glyph's own line and endchar are 7.
LEAF, MID, TOP = 16, 17, 18
LOCAL.append(cs(0, 0, "rmoveto", "return"))
LOCAL.append(cs(*([LEAF - 107, "callsubr"] * 10), "return"))
LOCAL.append(cs(*([MID - 107, "callsubr"] * 10), "return"))
calls = cs(*([TOP - 107, "callsubr"] * 315), *([MID - 107, "callsubr"] * 9),
           *([LEAF - 107, "callsubr"] * 5))
glyph("ops.199999", cs(0, 0, "rmoveto", 100, 100, "rlineto", calls, "endchar"))
glyph("ops.200000", cs(0, 0, "rmoveto", 100, 100, "rlineto", 500, calls, "endchar"))

order = [".notdef"] + list(glyphs)
glyphs[".notdef"] = cs("endchar")
fb = FontBuilder(1000, isTTF=False)
# fontTools would run the charstrings for the font's box, and several of these
# are ones it refuses to run.
fb.font.recalcBBoxes = False
fb.setupGlyphOrder(order)
# The characters cffink.py shapes: letters drawn plainly and by seac, and
# combining marks above, below and attached, one with no ink and one whose
# charstring never ends, so that it has none HarfBuzz can measure.
fb.setupCharacterMap({
    0x41: "A", 0x42: "B", 0x48: "hlineto.even", 0x2D: "hyphen", 0xB4: "acute",
    0xC1: "Aacute", 0x300: "rrcurveto", 0x301: "acute", 0x323: "hintmask",
    0x327: "move", 0x328: "no.endchar",
})
fb.setupCFF("CFFInk", {"FullName": "CFFInk"},
            {name: T2CharString(bytecode=glyphs[name]) for name in order},
            {"defaultWidthX": 500, "nominalWidthX": 0})
cff = fb.font["CFF "].cff
top = cff.topDictIndex[0]
top.Private.Subrs = SubrsIndex()
for code in LOCAL:
    top.Private.Subrs.append(T2CharString(bytecode=code))
for code in GLOBAL:
    cff.GlobalSubrs.append(T2CharString(bytecode=code))
fb.setupHorizontalMetrics({name: (500, 0) for name in order})
fb.setupHorizontalHeader(ascent=800, descent=-200)
fb.setupNameTable({"familyName": "CFFInk", "styleName": "Regular"})
fb.setupOS2(sTypoAscender=800, sTypoDescender=-200, usWinAscent=800, usWinDescent=200)
fb.setupPost()
fb.font["head"].created = fb.font["head"].modified = 0
out = os.path.join(sys.argv[1], "CFFInk.otf")
fb.save(out)
