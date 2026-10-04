# Builds the faces aatpos.py asks HarfBuzz to position, into the directory it
# is given:
#
#   make hbaatpos
#
# HarfBuzz's own tests have no face with a kerx, and one with a trak; these
# state what each table can say, written byte by byte, since fontTools builds
# neither from nothing:
#
#   KerxPairs.ttf     the pair formats: a sorted list of pairs (0), a class
#                     array (2), row and column arrays with short values and
#                     with long ones read through a format 10 lookup (6), a
#                     pair stated as a variation tuple, a pair across the line
#                     whose chain raises every glyph after it, and a subtable
#                     that walks the run backwards and one for a vertical line,
#                     neither of which kerns a horizontal one
#   KerxMachines.ttf  the state machines: a stack of glyphs popped with a list
#                     of values (1) along the line and across it, the value
#                     that resets an attachment, the flag that clears the
#                     stack; and attachment by a point of each glyph (4), by
#                     anchors of the ankr table, by coordinates, and by
#                     outline points
#   KerxPoints.ttf    attachment by outline points of composite glyphs — a
#                     component moved, and one scaled by half — and by a
#                     point a glyph does not have, which attaches nothing
#   KerxPlan*.ttf     what decides whether kerx positions the run at all: a
#                     face with GSUB and GPOS (GPOS does), with GPOS and no
#                     GSUB (kerx does), with GSUB and a GPOS offering no 'kern'
#                     (GPOS does, and no mark loses its advance), and with a
#                     legacy kern table beside it (never applied)
#   TrakCases.ttf     tracking: three tracks at three sizes, and a mark, an
#                     emoji sequence and a pair of regional indicators, each
#                     tracked once; TrakNoSTAT.ttf is it without STAT, which
#                     HarfBuzz does not track
#
# They are built with fontTools and their timestamps fixed, so that building
# them again produces the same bytes and the checksums the expectations record
# stay true.
import os
import struct
import sys

from oracle import fonttools

fonttools()
from fontTools.feaLib.builder import addOpenTypeFeaturesFromString  # noqa: E402
from fontTools.fontBuilder import FontBuilder  # noqa: E402
from fontTools.otlLib.builder import buildStatTable  # noqa: E402
from fontTools.pens.ttGlyphPen import TTGlyphPen  # noqa: E402
from fontTools.ttLib import newTable  # noqa: E402
from fontTools.ttLib.tables._k_e_r_n import KernTable_format_0  # noqa: E402
from fontTools.ttLib.tables.DefaultTable import DefaultTable  # noqa: E402
from fontTools.ttLib.tables._g_l_y_f import Glyph, GlyphComponent  # noqa: E402

GLYPHS = [".notdef", "A", "V", "T", "o", "B", "C", "D", "acutecomb", "smile", "heart", "zwj",
          "ri_j", "ri_p", "Bcomp", "Ccomp"]
GID = {name: i for i, name in enumerate(GLYPHS)}
CMAP = {ord("A"): "A", ord("V"): "V", ord("T"): "T", ord("o"): "o", ord("B"): "B", ord("C"): "C",
        ord("D"): "D", 0x0301: "acutecomb", 0x1F600: "smile", 0x2764: "heart", 0x200D: "zwj",
        0x1F1EF: "ri_j", 0x1F1F5: "ri_p", 0xE000: "Bcomp", 0xE001: "Ccomp"}
FFFF = 0xFFFF


def box(width):
    pen = TTGlyphPen(None)
    pen.moveTo((50, 0))
    pen.lineTo((50, 600))
    pen.lineTo((max(width, 120) - 50, 600))
    pen.lineTo((max(width, 120) - 50, 0))
    pen.closePath()
    return pen.glyph()


def composite(parts):
    """A composite glyph of (name, dx, dy, scale) components."""
    g = Glyph()
    g.numberOfContours = -1
    g.components = []
    for name, dx, dy, scale in parts:
        c = GlyphComponent()
        c.glyphName, c.x, c.y, c.flags = name, dx, dy, 0
        if scale != 1:
            c.transform = [[scale, 0], [0, scale]]
        g.components.append(c)
    return g


COMPOSITES = {
    # B moved, then the acute above it: its points after B's four.
    "Bcomp": [("B", 30, 40, 1), ("acutecomb", 100, 500, 1)],
    # C at half its size, moved.
    "Ccomp": [("C", 20, 0, 0.5)],
}


def outlines():
    out = {n: box(advance(n)) for n in GLYPHS if n not in COMPOSITES}
    out.update({n: composite(parts) for n, parts in COMPOSITES.items()})
    return out


def advance(name):
    return {"acutecomb": 300, "zwj": 0}.get(name, 600)


def lookup6(pairs, size=2):
    """A lookup table in format 6: glyph to value, with the terminating
    unit."""
    pairs = sorted(pairs)
    fmt = ">H" + ("H" if size == 2 else "I")
    units = b"".join(struct.pack(fmt, g, v) for g, v in pairs)
    units += struct.pack(fmt, FFFF, (1 << (8 * size)) - 1)
    n = len(pairs) + 1
    unit = 2 + size
    power = 1 << (n.bit_length() - 1)
    return struct.pack(">HHHHHH", 6, unit, n, unit * power, power.bit_length() - 1, unit * (n - power)) + units


def lookup8(first, values):
    return struct.pack(">HHH", 8, first, len(values)) + b"".join(struct.pack(">H", v) for v in values)


def lookup10(first, values, size):
    body = b"".join(v.to_bytes(size, "big") for v in values)
    return struct.pack(">HHHH", 10, size, first, len(values)) + body


def pad(b):
    return b + b"\0" * (-len(b) % 4)


def layout(header_len, parts):
    """Offsets for parts placed after a header of header_len bytes, each on
    a four-byte boundary, and the bytes after the header."""
    offsets, body, at = [], b"", header_len
    for p in parts:
        offsets.append(at)
        body += pad(p)
        at += len(pad(p))
    return offsets, body


def subtable(fmt, coverage, tuples, body):
    return struct.pack(">III", 12 + len(body), coverage | fmt, tuples) + body


VERTICAL, CROSS, BACKWARDS = 0x80000000, 0x40000000, 0x10000000


def format0(pairs, coverage=0, tuples=0, tail=b""):
    pairs = sorted(pairs, key=lambda p: (GID[p[0]], GID[p[1]]))
    n = len(pairs)
    power = 1 << (n.bit_length() - 1) if n else 0
    head = struct.pack(">IIII", n, 6 * power, max(power.bit_length() - 1, 0), 6 * (n - power))
    body = head + b"".join(struct.pack(">HHh", GID[a], GID[b], v) for a, b, v in pairs) + tail
    return subtable(0, coverage, tuples, body)


def format2():
    # Left values are the start of a row, right values the column; the value
    # is at their sum.
    left = lookup6([(GID["A"], 0), (GID["T"], 3)])
    right = lookup6([(GID["V"], 1), (GID["o"], 2)])
    array = struct.pack(">6h", 0, -10, 0, 0, 0, -20)
    offsets, body = layout(12 + 16, [left, right, array])
    return subtable(2, 0, 0, struct.pack(">IIII", 3, *offsets) + body)


def format6(long):
    if long:
        rows = lookup10(GID["D"], [0], 4)
        cols = lookup10(GID["A"], [1], 4)
        array = struct.pack(">2i", 0, -15)
    else:
        rows = lookup8(GID["B"], [0, 2])
        cols = lookup8(GID["B"], [0, 1])
        array = struct.pack(">4h", -5, -7, -9, -11)
    offsets, body = layout(12 + 24, [rows, cols, array])
    head = struct.pack(">IHHIIII", 1 if long else 0, 2, 2, offsets[0], offsets[1], offsets[2], 0)
    return subtable(6, 0, 0, head + body)


def kerx(subtables):
    return struct.pack(">HHI", 2, 0, len(subtables)) + b"".join(subtables)


def pairs_kerx():
    # A pair stated as a variation tuple: its value is the offset, from the
    # subtable's start, of its tuple.
    tuple_at = 12 + 16 + 6
    tupled = format0([("V", "T", tuple_at)], tuples=1, tail=struct.pack(">h", -25))
    return kerx([
        format0([("A", "V", -80), ("V", "A", -60), ("T", "o", -100)]),
        format2(),
        format6(False),
        format6(True),
        format0([("o", "T", -999)], BACKWARDS),
        format0([("A", "V", -500)], VERTICAL),
        tupled,
        format0([("B", "B", 100)], CROSS),
    ])


def state_table(classes, rows, entries):
    """An extended state table: its header, class table, state array and
    entries, each entry its new state, flags and one data word."""
    n_classes = max(classes.values()) + 1
    class_table = lookup6(classes.items())
    state_array = b"".join(struct.pack(">" + "H" * n_classes, *r) for r in rows)
    entry_table = b"".join(struct.pack(">HHH", *e) for e in entries)
    return n_classes, [class_table, state_array, entry_table]


def machine_subtable(fmt, coverage, classes, rows, entries, tail_word, tail):
    """A state machine subtable: the state table, then a word the format
    reads after it — format 1's offset to its values, format 4's flags and
    offset — and the data it names, all from the state table's start."""
    n_classes, parts = state_table(classes, rows, entries)
    head_len = 16 + 4
    offsets, body = layout(head_len, parts + [tail])
    word = tail_word(offsets[3])
    st = struct.pack(">IIII", n_classes, offsets[0], offsets[1], offsets[2]) + struct.pack(">I", word) + body
    return subtable(fmt, coverage, 0, st)


PUSH, RESET, MARK = 0x8000, 0x2000, 0x8000


def machines_kerx():
    # Format 1 along the line: A pushed and the machine into state 1; V
    # pushed and the values from index 4 (read as 2) popped onto them: V
    # by -40, A by -22, the odd value ending the list even where the stack
    # holds another A, which the value after it is not popped onto. T clears
    # the stack.
    classes = {GID["A"]: 4, GID["V"]: 5, GID["T"]: 6}
    entries = [
        (0, 0, FFFF),  # nothing
        (1, PUSH, FFFF),  # A
        (0, PUSH, 4),  # V after A
        (0, RESET, FFFF),  # T
        (1, 0, FFFF),  # stay in 1
    ]
    rows = [[0, 0, 0, 0, 1, 0, 3], [0, 4, 4, 4, 1, 2, 3]]
    along = machine_subtable(1, 0, classes, rows, entries, lambda off: off,
                             struct.pack(">5h", 999, 999, -40, -21, -77))
    # Format 1 across the line: o pushed and raised by 60; B pushed and its
    # attachment reset, which -0x8000 says.
    classes = {GID["o"]: 4, GID["B"]: 5}
    entries = [(0, 0, FFFF), (0, PUSH, 0), (0, PUSH, 2)]
    rows = [[0, 0, 0, 0, 1, 2], [0, 0, 0, 0, 1, 2]]
    across = machine_subtable(1, CROSS, classes, rows, entries, lambda off: off,
                              struct.pack(">2h", 60, -0x8000))
    # Format 4 by anchors of the ankr: A marked; the acute after it attached
    # by A's anchor 0 and its own anchor 1.
    classes = {GID["A"]: 4, GID["acutecomb"]: 5, GID["D"]: 6, GID["C"]: 7}
    entries = [(0, 0, FFFF), (0, MARK, FFFF), (0, 0, 0)]
    rows = [[0, 0, 0, 0, 1, 2, 0, 0], [0, 0, 0, 0, 1, 2, 0, 0]]
    anchors = machine_subtable(4, 0, classes, rows, entries, lambda off: (1 << 30) | off,
                               struct.pack(">2H", 0, 1))
    # Format 4 by coordinates: D marked; the acute after it attached by the
    # points the action states.
    entries = [(0, 0, FFFF), (0, MARK, FFFF), (0, 0, 0)]
    rows = [[0, 0, 0, 0, 0, 2, 1, 0], [0, 0, 0, 0, 0, 2, 1, 0]]
    coords = machine_subtable(4, 0, classes, rows, entries, lambda off: (2 << 30) | off,
                              struct.pack(">4h", 120, 650, 40, -10))
    # Format 4 by outline points: C marked, and the acute after it attached
    # by C's point 1 and its own point 2, and marking itself.
    entries = [(0, 0, FFFF), (0, MARK, FFFF), (0, MARK, 0)]
    rows = [[0, 0, 0, 0, 0, 2, 0, 1], [0, 0, 0, 0, 0, 2, 0, 1]]
    points = machine_subtable(4, 0, classes, rows, entries, lambda off: off, struct.pack(">2H", 1, 2))
    return kerx([along, across, anchors, coords, points])


def points_kerx():
    # Format 4 by outline points: Bcomp, Ccomp or D marked, each into a state
    # of its own, from which the acute is attached by the action that state
    # names: Bcomp's point 5 (the acute component's second) to the acute's
    # point 0; Ccomp's point 2 to the acute's point 1; and D's point 40, which
    # D does not have, which attaches nothing and marks nothing.
    classes = {GID["Bcomp"]: 4, GID["Ccomp"]: 5, GID["acutecomb"]: 6, GID["D"]: 7}
    entries = [(0, 0, FFFF), (1, MARK, FFFF), (2, MARK, FFFF), (3, MARK, FFFF),
               (0, MARK, 0), (0, MARK, 1), (0, MARK, 2)]
    marks = [1, 2, 0, 3]
    rows = [[0, 0, 0, 0] + marks[:2] + [act] + marks[3:] for act in (0, 4, 5, 6)]
    points = machine_subtable(4, 0, classes, rows, entries, lambda off: off,
                              struct.pack(">6H", 5, 0, 2, 1, 40, 0))
    return kerx([points])


def ankr():
    # Each glyph's anchors: A's (300, 700); the acute's (0, 0) and (150, -20).
    data_a = struct.pack(">I2h", 1, 300, 700)
    data_acute = struct.pack(">I4h", 2, 0, 0, 150, -20)
    look = lookup6([(GID["A"], 0), (GID["acutecomb"], len(data_a))])
    lookup_at = 12
    data_at = lookup_at + len(pad(look))
    return struct.pack(">HHII", 0, 0, lookup_at, data_at) + pad(look) + data_a + data_acute


def trak():
    sizes = [9.0, 12.0, 24.0]
    tracks = [(-1.0, [-50, -60, -70]), (0.0, [30, 10, -40]), (1.0, [100, 100, 100])]
    head = 12
    data = head
    entries_at = data + 8
    sizes_at = entries_at + 8 * len(tracks)
    values_at = sizes_at + 4 * len(sizes)
    out = struct.pack(">IHHHH", 0x00010000, 0, data, 0, 0)
    out += struct.pack(">HHI", len(tracks), len(sizes), sizes_at)
    for i, (t, _) in enumerate(tracks):
        out += struct.pack(">iHH", int(t * 65536), 256 + i, values_at + 2 * len(sizes) * i)
    out += b"".join(struct.pack(">i", int(s * 65536)) for s in sizes)
    for _, values in tracks:
        out += struct.pack(">%dh" % len(sizes), *values)
    return out


def raw(tag, data):
    t = DefaultTable(tag)
    t.data = data
    return t


def base(name, extra=None, fea=None, kern_table=None, stat=False):
    fb = FontBuilder(1000, isTTF=True)
    fb.setupGlyphOrder(GLYPHS)
    fb.setupCharacterMap(CMAP)
    fb.setupGlyf(outlines())
    fb.setupHorizontalMetrics({n: (advance(n), 50) for n in GLYPHS})
    fb.setupHorizontalHeader(ascent=800, descent=-200)
    fb.setupOS2(sTypoAscender=800, sTypoDescender=-200, usWinAscent=800, usWinDescent=200)
    fb.setupNameTable({"familyName": name, "styleName": "Regular"})
    fb.setupPost()
    if fea:
        addOpenTypeFeaturesFromString(fb.font, fea)
    if kern_table:
        kern = newTable("kern")
        kern.version = 0
        sub = KernTable_format_0()
        sub.coverage, sub.format, sub.tupleIndex = 1, 0, 0
        sub.kernTable = kern_table
        kern.kernTables = [sub]
        fb.font["kern"] = kern
    if stat:
        buildStatTable(fb.font, [{"tag": "wght", "name": "Weight", "values": [{"value": 400, "name": "Regular"}]}])
    for tag, data in (extra or {}).items():
        fb.font[tag] = raw(tag, data)
    fb.font["head"].created = fb.font["head"].modified = 3660681600
    fb.font.recalcTimestamp = False
    return fb.font


LS = """
languagesystem DFLT dflt;
languagesystem latn dflt;
"""
GPOS_KERN = """
feature kern { pos A V -10; } kern;
"""
GSUB = """
feature ccmp { sub D by D; } ccmp;
"""
GPOS_MARK_ONLY = """
markClass acutecomb <anchor 150 0> @TOP;
feature mark { pos base A <anchor 300 700> mark @TOP; } mark;
"""
PLAN_KERX = kerx([format0([("A", "V", -80)])])


def build(directory):
    def save(font, name):
        font.save(os.path.join(directory, name))

    save(base("KerxPairs", {"kerx": pairs_kerx()}), "KerxPairs.ttf")
    save(base("KerxMachines", {"kerx": machines_kerx(), "ankr": ankr()}), "KerxMachines.ttf")
    save(base("KerxPoints", {"kerx": points_kerx()}), "KerxPoints.ttf")
    save(base("KerxPlanGSUBGPOS", {"kerx": PLAN_KERX}, fea=LS + GSUB + GPOS_KERN), "KerxPlanGSUBGPOS.ttf")
    save(base("KerxPlanGPOS", {"kerx": PLAN_KERX}, fea=LS + GPOS_KERN), "KerxPlanGPOS.ttf")
    save(base("KerxPlanNoKern", {"kerx": PLAN_KERX}, fea=LS + GSUB + GPOS_MARK_ONLY), "KerxPlanNoKern.ttf")
    save(base("KerxPlanLegacyKern", {"kerx": PLAN_KERX}, kern_table={("A", "V"): -33}),
         "KerxPlanLegacyKern.ttf")
    save(base("TrakCases", {"trak": trak()}, stat=True), "TrakCases.ttf")
    save(base("TrakNoSTAT", {"trak": trak()}), "TrakNoSTAT.ttf")


if __name__ == "__main__":
    build(sys.argv[1])
