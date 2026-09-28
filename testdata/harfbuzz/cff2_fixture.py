# Builds the CFF2 face cff2.py asks about that no foundry made, into the
# directory it is given:
#
#   make hbcff2
#
# The real CFF2 fonts (make cff-fonts) exercise what a font compiler writes:
# blends in charstrings and in Private DICTs, one or two groups of regions,
# local subroutines, hints. What they never do is land a blended number on a
# half, or make the order two axes' scalars are multiplied in decide how one
# rounds — which is where fontTools' instancer and a reader of the same numbers
# part — or ask for what a CFF charstring cannot hold. This face is those,
# one glyph each, so that each rule shape/cff2.go and shape/cff2cff.go follow
# is one some oracle's answer turns on:
#
#   - tie: numbers whose deltas come to exactly half a unit at the location
#     wght=8192 (a scalar of one half), which fontTools rounds half to even;
#   - order: a number blended against a region of two axes, at a location and
#     with peaks where multiplying the delta by the two scalars in the order of
#     the axes' tags — XOPQ before wght, the reverse of fvar's order — rounds
#     one way, and multiplying in fvar's order, or by their product, rounds the
#     other (found by a search, and checked below);
#   - vsindex: a glyph whose blends use the second group of regions;
#   - subr and gsubr: blends inside a local and a global subroutine, one of
#     them reading the defaults and deltas its caller pushed;
#   - hints: stems, a first hintmask declaring stems by its operands, a
#     cntrmask, and a mask later on;
#   - stems60: sixty stems, more in one operator than a CFF charstring's 48
#     operands hold;
#   - manystems: a hundred stems, more than the 96 a CFF charstring may have;
#   - long: a rlineto of 50 lines and a rrcurveto of 10 curves, more than a CFF
#     charstring's 48 operands;
#   - flex: the four flex operators, blended;
#   - fixed: 16.16 numbers, blended;
#   - fd1: a glyph in a second Font DICT, whose Private DICT names the second
#     group and blends its BlueValues;
#   - empty and moveonly: a glyph that draws nothing, and one that only moves.
#
# Its HVAR puts advances on halves at wght=8192, where HarfBuzz rounds half up
# and fontTools half to even; its VVAR moves vertical advances and, through
# VORG, vertical origins.
#
# The CFF2 table is written here byte by byte rather than by fontTools'
# builder, which writes one Font DICT and no groups of regions of its own
# choosing. HarfBuzz and fontTools both read it, which is the check that it is
# a CFF2 table. The timestamps are fixed, so the same bytes come out each time.
import io
import os
import struct
import sys

from oracle import fonttools

fonttools()
from fontTools.fontBuilder import FontBuilder  # noqa: E402
from fontTools.ttLib import TTFont, newTable  # noqa: E402
from fontTools.ttLib.tables import otTables as ot  # noqa: E402
from fontTools.ttLib.tables.otBase import OTTableWriter  # noqa: E402
from fontTools.varLib.builder import (  # noqa: E402
    buildVarData, buildVarIdxMap, buildVarRegionList, buildVarStore)

AXES = ["wght", "XOPQ"]  # fvar's order; their tags sort the other way

# The order glyph's region, location and delta: found by searching peaks and
# locations on the F2Dot14 grid for a delta that (d·sX)·sW rounds away from
# both (d·sW)·sX and d·(sX·sW).
PW, PX, CW, CX, D = 3000, 16169, 1311, 500, 333


def f2(v):
    return v / 16384


def order_check():
    sw, sx = (f2(CW) - 0.0) / (f2(PW) - 0.0), (f2(CX) - 0.0) / (f2(PX) - 0.0)
    ft, prod, rev = (D * sx) * sw, D * ((1.0 * sx) * sw), (D * sw) * sx
    if not (round(ft) != round(prod) and round(ft) != round(rev)):
        sys.exit("the order glyph's numbers no longer tell the orders apart")


order_check()

REGIONS = [
    {"wght": (0, f2(16384), f2(16384))},                     # 0
    {"XOPQ": (0, f2(16384), f2(16384))},                     # 1
    {"wght": (0, f2(PW), f2(16384)), "XOPQ": (0, f2(PX), f2(16384))},  # 2
]
GROUPS = [[0, 1], [2, 0]]  # vsindex 0 and 1


# Type 2 numbers and operators.
def num(v):
    if isinstance(v, float) and v != int(v):
        return b"\xff" + struct.pack(">i", round(v * 65536))
    v = int(v)
    if -107 <= v <= 107:
        return bytes([v + 139])
    if 108 <= v <= 1131:
        v -= 108
        return bytes([(v >> 8) + 247, v & 0xFF])
    if -1131 <= v <= -108:
        v = -v - 108
        return bytes([(v >> 8) + 251, v & 0xFF])
    return b"\x1c" + struct.pack(">h", v)


OPS = {"hstem": 1, "vstem": 3, "rlineto": 5, "rrcurveto": 8, "callsubr": 10,
       "vsindex": 15, "blend": 16, "hstemhm": 18, "hintmask": 19, "cntrmask": 20,
       "rmoveto": 21, "vstemhm": 23, "callgsubr": 29}
ESC = {"hflex": 34, "flex": 35, "hflex1": 36, "flex1": 37}


def cs(*items):
    out = b""
    for it in items:
        if isinstance(it, bytes):
            out += it
        elif isinstance(it, str):
            out += bytes([12, ESC[it]]) if it in ESC else bytes([OPS[it]])
        else:
            out += num(it)
    return out


def blend(defaults, deltas, k):
    """n defaults, k deltas each, the count and blend."""
    items = list(defaults)
    for d in deltas:
        assert len(d) == k
        items += d
    return items + [len(defaults), "blend"]


# Local subroutines of Font DICT 0, and the global ones. The bias is 107.
LOCAL = [
    cs("blend", "rlineto"),                                  # 0: blends what its caller pushed
    cs(*blend([30, 40], [[3, 1], [5, 1]], 2), "rlineto"),    # 1: blends its own
]
GLOBAL = [
    cs(*blend([-40, 25], [[-5, 2], [7, 1]], 2), "rlineto"),  # 0
]

glyphs = {}
fds = {}


def glyph(name, code, fd=0):
    glyphs[name] = code
    fds[name] = fd


glyph(".notdef", cs(50, 0, "rmoveto", 400, 0, 0, 700, -400, 0, "rlineto"))
glyph("tie", cs(*blend([100, 100], [[1, 3], [-1, 5]], 2), "rmoveto",
                *blend([200, 10, 10, 300], [[1, 1], [3, 1], [-1, 1], [-3, 1]], 2), "rlineto",
                *blend([-150, 20], [[5, 1], [7, 1]], 2), "rlineto"))
glyph("order", cs(1, "vsindex", 50, 50, "rmoveto",
                  *blend([100, 400], [[D, 0], [-D, 0]], 2), "rlineto",
                  *blend([-100, 7], [[D, 3], [3 * D, 0]], 2), "rlineto"))
glyph("subr", cs(20, 20, "rmoveto", 50, 10, 30, 3, 7, 1, 2, -107, "callsubr",
                 -106, "callsubr", -107, "callgsubr"))
glyph("hints", cs(*blend([10, 20], [[2, 1], [4, 1]], 2), 100, 30, "hstemhm",
                  50, 40, 200, 40, "hintmask", bytes([0xE0]),
                  60, 60, "rmoveto", *blend([300, 0], [[20, 5], [0, 0]], 2), "rlineto",
                  "cntrmask", bytes([0xA0]), 0, 300, -300, 0, "rlineto",
                  "hintmask", bytes([0x40]), 0, -300, "rlineto"))
glyph("stems60", cs(*([7, 3] * 40), "hstemhm", *([9, 4] * 20), "vstemhm",
                    "hintmask", bytes([0xA5] * 8), 0, 0, "rmoveto", 100, 0, 0, 100, "rlineto"))
glyph("manystems", cs(*([7, 3] * 100), "hstemhm", "hintmask", bytes([0x55] * 13),
                      0, 0, "rmoveto", 100, 0, 0, 100, "rlineto"))
glyph("long", cs(0, 0, "rmoveto", *([7, 3, -3, 5] * 25), "rlineto",
                 *([10, 0, 10, 10, 0, 10] * 10), "rrcurveto"))
glyph("flex", cs(0, 0, "rmoveto",
                 *blend([10, 20, 30, 40, 50, 60, 70], [[1, 2]] * 7, 2), "hflex",
                 10, 1, 20, 2, 30, 3, 40, -3, 50, -2, 60, -1, 0, "flex",
                 5, 5, 10, 10, 20, 20, 10, -5, 5, "hflex1",
                 *blend([10, 10, 10, 10, 10, 10, 10, -10, 10, -10, 30], [[1, 1]] * 11, 2),
                 "flex1"))
glyph("fixed", cs(10.25, 20.5, "rmoveto",
                  *blend([100.75, -3.125], [[1, 1], [2, 1]], 2), "rlineto",
                  33.5, 0, "rlineto"))
glyph("fd1", cs(0, 0, "rmoveto", *blend([400, 300], [[D, 0], [5, 2]], 2), "rlineto"), fd=1)
glyph("empty", b"")
glyph("moveonly", cs(40, 80, "rmoveto"))

order = list(glyphs)
N = len(order)


# CFF2 structures.
def index2(items):
    if not items:
        return struct.pack(">I", 0)
    offs, pos = [1], 1
    for it in items:
        pos += len(it)
        offs.append(pos)
    size = 1 if pos <= 0xFF else 2 if pos <= 0xFFFF else 3 if pos <= 0xFFFFFF else 4
    out = struct.pack(">IB", len(items), size)
    for o in offs:
        out += o.to_bytes(size, "big")
    return out + b"".join(items)


def dint(v):
    return b"\x1d" + struct.pack(">i", v)


def dnum(v):
    return num(v)  # a DICT's small integers are written as a charstring's are


def private(fd):
    if fd == 0:
        # BlueValues blended, StdHW blended, and Subrs.
        return (dnum(-12) + dnum(12) + dnum(500) + dnum(10)
                + dnum(-2) + dnum(1) + dnum(0) + dnum(0) + dnum(6) + dnum(3) + dnum(4) + dnum(2)
                + dnum(4) + bytes([23]) + bytes([6])
                + dnum(60) + dnum(15) + dnum(4) + dnum(1) + bytes([23]) + bytes([10]))
    # Font DICT 1: its blends use group 1.
    return (dnum(1) + bytes([22])
            + dnum(-15) + dnum(15) + dnum(D) + dnum(-3) + dnum(1) + dnum(1) + dnum(2) + bytes([23]) + bytes([6]))


def varstore():
    font = TTFont()
    font.setGlyphOrder(order)
    regions = buildVarRegionList(REGIONS, AXES)
    store = buildVarStore(regions, [buildVarData(g, [], optimize=False) for g in GROUPS])
    w = OTTableWriter()
    store.compile(w, font)
    return w.getAllData()


def cff2():
    charstrings = index2([glyphs[n] for n in order])
    gsubrs = index2(GLOBAL)
    local = index2(LOCAL)
    vstore = varstore()
    privs = [private(0), private(1)]
    fdselect = bytes([0]) + bytes(fds[n] for n in order)
    def top(cs_at, fdarray_at, fdselect_at, vstore_at):
        return (dint(cs_at) + bytes([17]) + dint(fdarray_at) + bytes([12, 36])
                + dint(fdselect_at) + bytes([12, 37]) + dint(vstore_at) + bytes([24]))
    head = bytes([2, 0, 5]) + struct.pack(">H", len(top(0, 0, 0, 0)))
    at = len(head) + len(top(0, 0, 0, 0)) + len(gsubrs)
    vstore_at = at
    at += 2 + len(vstore)
    cs_at = at
    at += len(charstrings)
    fdselect_at = at
    at += len(fdselect)
    fdarray_at = at

    def fdarray(priv_at):
        return index2([dint(len(p)) + dint(a) + bytes([18]) for p, a in zip(privs, priv_at)])

    at += len(fdarray([0, 0]))
    priv_at = []
    for i, p in enumerate(privs):
        priv_at.append(at)
        at += len(p)
        if i == 0:
            at += len(local)
    # Font DICT 0's Private DICT names its local subroutines right after it.
    privs[0] = privs[0] + dint(len(privs[0]) + 6) + bytes([19])
    # Its length changed: lay the rest out again.
    at = fdarray_at + len(fdarray([0, 0]))
    priv_at = []
    for i, p in enumerate(privs):
        priv_at.append(at)
        at += len(p)
        if i == 0:
            at += len(local)
    out = (head + top(cs_at, fdarray_at, fdselect_at, vstore_at) + gsubrs
           + struct.pack(">H", len(vstore)) + vstore + charstrings + fdselect + fdarray(priv_at)
           + privs[0] + local + privs[1])
    assert len(out) == at, (len(out), at)
    return out


fb = FontBuilder(1000, isTTF=False)
fb.font.recalcBBoxes = False
fb.setupGlyphOrder(order)
# Glyph g is the letter g places after 'a', so that the strings the shaping
# fuzzer sets in it reach its glyphs.
fb.setupCharacterMap({0x61 + i: n for i, n in enumerate(order[1:])})
advances = {n: (500 + 10 * i, 0) for i, n in enumerate(order)}
fb.setupHorizontalMetrics(advances)
fb.setupHorizontalHeader(ascent=880, descent=-120)
fb.setupNameTable({"familyName": "CFF2Blend", "styleName": "Regular"})
fb.setupOS2(sTypoAscender=880, sTypoDescender=-120, usWinAscent=880, usWinDescent=120,
            usWeightClass=100)
fb.setupPost(keepGlyphNames=False)
fb.setupFvar([("wght", 0, 0, 16384, "Weight"), ("XOPQ", 0, 0, 16384, "Thin stroke")], [])
table = newTable("CFF2")
table.decompile(cff2(), fb.font)
fb.font["CFF2"] = table
fb.setupVerticalMetrics({n: (1000, 120) for n in order})
fb.setupVerticalHeader(ascent=500, descent=-500)
fb.setupVerticalOrigins({n: 880 for n in order}, defaultVerticalOrigin=880)


def mtx_var(tag, advance_deltas, origin_deltas=None):
    """HVAR or VVAR: each glyph's advance deltas, one per region of group 0,
    mapped directly; and for VVAR, origin deltas through a mapping."""
    regions = buildVarRegionList(REGIONS, AXES)
    rows = [advance_deltas[n] for n in order]
    datas = [buildVarData([0, 1], rows, optimize=False)]
    if origin_deltas is not None:
        datas.append(buildVarData([0, 1], [origin_deltas[n] for n in order], optimize=False))
    store = buildVarStore(regions, datas)
    t = getattr(ot, tag)()
    t.Version = 0x00010000
    t.VarStore = store
    for m in ("AdvWidthMap", "LsbMap", "RsbMap", "AdvHeightMap", "TsbMap", "BsbMap", "VOrgMap"):
        setattr(t, m, None)
    if origin_deltas is not None:
        t.VOrgMap = buildVarIdxMap([(1 << 16) | i for i in range(N)], order)
    table = newTable(tag)
    table.table = t
    return table


# Halves at wght=8192 for the first glyphs; whole units elsewhere.
fb.font["HVAR"] = mtx_var("HVAR", {n: [(1, 3, -1, -3, 5)[i % 5], i % 7] for i, n in enumerate(order)})
fb.font["VVAR"] = mtx_var("VVAR", {n: [2 * i + 1, -i] for i, n in enumerate(order)},
                          {n: [-(i + 1), 3] for i, n in enumerate(order)})
fb.setupMaxp()
fb.font["head"].created = fb.font["head"].modified = 0
buf = io.BytesIO()
fb.save(buf)
# Read back, the check that fontTools takes it for a CFF2 table.
back = TTFont(io.BytesIO(buf.getvalue()))
cs2 = back["CFF2"].cff.topDictIndex[0].CharStrings
for n in back.getGlyphOrder():
    cs2[n].decompile()
with open(os.path.join(sys.argv[1], "CFF2Blend.otf"), "wb") as w:
    w.write(buf.getvalue())
