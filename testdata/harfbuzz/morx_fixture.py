# Builds MorxCases.ttf, a face whose morx states what the text-rendering tests'
# suite does not reach, into the directory it is given:
#
#   make hbmorx
#
# morx.py shapes strings in it with HarfBuzz beside the suite's cases. The
# table is written byte by byte, since fontTools builds no morx from nothing,
# and holds:
#
#   - a chain whose flags switch one of its noncontextual subtables on and
#     the other off (A becomes X; B would become Y, and does not);
#   - in the same chain, a noncontextual subtable turning C into Y, and after
#     it an insertion subtable that inserts X at the end of the text from its
#     start state, which only C can leave. HarfBuzz runs a subtable only where
#     the run holds a glyph that can start it: for a run shorter than four
#     glyphs it asks the run as it is, in which C has become Y and nothing
#     starts the insertion; for a longer one it asks every glyph the run has
#     held, C among them, and the insertion runs;
#   - a chain whose contextual subtable substitutes the current glyph at the
#     end of the text from its second state, which D enters marking itself and
#     F enters without marking anything. HarfBuzz, like CoreText, applies no
#     substitution at the end of the text where nothing was marked: E becomes
#     Y after D, and stays E after F.
#
# It is built with fontTools and its timestamps fixed, so that building it
# again produces the same bytes and the checksum the expectations record stays
# true.
import os
import struct
import sys

from oracle import fonttools

fonttools()
from fontTools.fontBuilder import FontBuilder  # noqa: E402
from fontTools.pens.ttGlyphPen import TTGlyphPen  # noqa: E402
from fontTools.ttLib.tables.DefaultTable import DefaultTable  # noqa: E402

GLYPHS = [".notdef", "A", "B", "C", "D", "E", "F", "X", "Y"]
GID = {name: i for i, name in enumerate(GLYPHS)}
FFFF = 0xFFFF


def box(width):
    pen = TTGlyphPen(None)
    pen.moveTo((50, 0))
    pen.lineTo((50, 600))
    pen.lineTo((width - 50, 600))
    pen.lineTo((width - 50, 0))
    pen.closePath()
    return pen.glyph()


def single_lookup(pairs):
    """A lookup table in format 6, single glyphs: glyph to value, sorted,
    with the terminating unit."""
    pairs = sorted(pairs)
    units = b"".join(struct.pack(">HH", g, v) for g, v in pairs) + struct.pack(">HH", FFFF, FFFF)
    n = len(pairs) + 1
    power = 1 << (n.bit_length() - 1)
    header = struct.pack(">HHHHHH", 6, 4, n, 4 * power, power.bit_length() - 1, 4 * (n - power))
    return header + units


def state_table(classes, rows, entries, extra_offsets, extra_blobs):
    """An extended state table's body: header, class table, state array,
    entries, then the subtable's own tables, whose offsets follow the header.
    classes maps glyph to class; rows are each state's entry per class; each
    entry is its new state, flags and data words."""
    n_classes = 4 + max(classes.values()) - 3 if classes else 4
    head = 16 + 4 * len(extra_offsets)
    class_table = single_lookup(classes.items())
    state_array = b"".join(struct.pack(">" + "H" * n_classes, *row) for row in rows)
    entry_table = b"".join(struct.pack(">" + "H" * len(e), *e) for e in entries)
    parts = [class_table, state_array, entry_table] + extra_blobs
    offsets, at = [], head
    for p in parts:
        while at % 4:
            at += 1
        offsets.append(at)
        at += len(p)
    body = struct.pack(">IIII", n_classes, offsets[0], offsets[1], offsets[2])
    body += b"".join(struct.pack(">I", o) for o in offsets[3:])
    for o, p in zip(offsets, parts):
        body += b"\0" * (o - len(body)) + p
    return body


def subtable(kind, flags, body):
    return struct.pack(">III", 12 + len(body), kind, flags) + body


def chain(default_flags, subtables):
    body = b"".join(subtables)
    return struct.pack(">IIII", default_flags, 16 + len(body), 0, len(subtables)) + body


def morx():
    # Chain one: flags 1.
    a_to_x = subtable(4, 1, single_lookup([(GID["A"], GID["X"])]))
    b_to_y = subtable(4, 2, single_lookup([(GID["B"], GID["Y"])]))
    c_to_y = subtable(4, 1, single_lookup([(GID["C"], GID["Y"])]))
    # Insertion: C is class 4 and leaves the start state; the end of the
    # text inserts X after the last glyph.
    insert_count = 1 << 5  # CurrentInsertCount
    entries = [
        (0, 0, FFFF, FFFF),  # nothing
        (0, insert_count, 0, FFFF),  # insert list[0] at the current glyph
        (1, 0, FFFF, FFFF),  # C: into state 1
    ]
    row = [1, 0, 0, 0, 2]
    insertion = subtable(5, 1, state_table({GID["C"]: 4}, [row, row], entries, [None],
                                           [struct.pack(">H", GID["X"])]))
    one = chain(1, [a_to_x, b_to_y, c_to_y, insertion])

    # Chain two: a contextual subtable acting at the end of the text.
    set_mark = 0x8000
    entries = [
        (0, 0, FFFF, FFFF),  # nothing
        (0, 0, FFFF, 0),  # substitute the current glyph through lookup 0
        (1, set_mark, FFFF, FFFF),  # D: mark it, into state 1
        (1, 0, FFFF, FFFF),  # F: into state 1, marking nothing
        (1, 0, FFFF, FFFF),  # stay in state 1
    ]
    rows = [[0, 0, 0, 0, 2, 3], [1, 4, 4, 4, 2, 3]]
    lookup = single_lookup([(GID["E"], GID["Y"])])
    subs = struct.pack(">I", 4) + lookup  # the list of lookups: one, after its offset
    contextual = subtable(1, 1, state_table({GID["D"]: 4, GID["F"]: 5}, rows, entries, [None], [subs]))
    two = chain(1, [contextual])

    return struct.pack(">HHI", 2, 0, 2) + one + two


def build(path):
    fb = FontBuilder(1000, isTTF=True)
    fb.setupGlyphOrder(GLYPHS)
    fb.setupCharacterMap({ord(n): n for n in "ABCDEF"})
    fb.setupGlyf({name: box(400 + 50 * i) for i, name in enumerate(GLYPHS)})
    fb.setupHorizontalMetrics({name: (400 + 50 * i, 50) for i, name in enumerate(GLYPHS)})
    fb.setupHorizontalHeader(ascent=800, descent=-200)
    fb.setupOS2(sTypoAscender=800, sTypoDescender=-200, usWinAscent=800, usWinDescent=200)
    fb.setupNameTable({"familyName": "MorxCases", "styleName": "Regular"})
    fb.setupPost()
    table = DefaultTable("morx")
    table.data = morx()
    fb.font["morx"] = table
    fb.font["head"].created = fb.font["head"].modified = 3660681600
    fb.font.recalcTimestamp = False
    fb.save(path)


if __name__ == "__main__":
    build(os.path.join(sys.argv[1], "MorxCases.ttf"))
