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


def save(path, family, glyphs, letters, tables):
    """A face of the glyphs, each a box a little wider than the one before,
    the letters mapped to the glyphs of their names, and the tables given as
    their bytes."""
    fb = FontBuilder(1000, isTTF=True)
    fb.setupGlyphOrder(glyphs)
    fb.setupCharacterMap({ord(n): n for n in letters})
    fb.setupGlyf({name: box(400 + 50 * i) for i, name in enumerate(glyphs)})
    fb.setupHorizontalMetrics({name: (400 + 50 * i, 50) for i, name in enumerate(glyphs)})
    fb.setupHorizontalHeader(ascent=800, descent=-200)
    fb.setupOS2(sTypoAscender=800, sTypoDescender=-200, usWinAscent=800, usWinDescent=200)
    fb.setupNameTable({"familyName": family, "styleName": "Regular"})
    fb.setupPost()
    for tag, data in tables.items():
        table = DefaultTable(tag)
        table.data = data
        fb.font[tag] = table
    fb.font["head"].created = fb.font["head"].modified = 3660681600
    fb.font.recalcTimestamp = False
    fb.save(path)


def build(path):
    save(path, "MorxCases", GLYPHS, "ABCDEF", {"morx": morx()})


# MorxFeatures.ttf: a morx chain whose features turn its subtables on and off,
# and a feat table offering their types, for the features a caller asks for.
# Each subtable turns one letter into X, so which ran shows letter by letter:
#
#   G  flag 0x01, on by default; contextual alternates (36) off clears it
#   H  flag 0x02, common ligatures (1) on sets it, off clears it
#   I  flag 0x04, small capitals by their deprecated type (3, setting 3),
#      which a request for small capitals (37, setting 1) reaches
#   J  flag 0x08, character alternatives (17), which 'aalt' asks for
#   K  flag 0x10, lining figures (21, setting 1), which clear oldstyle
#   L  flag 0x20, oldstyle figures (21, setting 0), which clear lining
#   M  flag 0x40, small capitals (37, setting 1) itself
#   N  flag 0x80, full-width text (22, setting 1)
#   O  flag 0x100, half-width text (22, setting 2)
#
# The figure types are exclusive, so asking for both keeps the first; so is
# text spacing, whose two settings here are not a setting and its opposite,
# and which a type that is not exclusive would keep both of.
# MorxFeaturesDeprecated.ttf is the same with a feat that offers small capitals
# under their deprecated type alone, which HarfBuzz looks for where the
# current one is not offered; MorxFeaturesNoFeat.ttf the same chain with no
# feat table at all, which HarfBuzz runs with its default flags whatever is
# asked.
FEATURE_GLYPHS = [".notdef", "G", "H", "I", "J", "K", "L", "M", "X", "N", "O"]
FEATURE_GID = {name: i for i, name in enumerate(FEATURE_GLYPHS)}
ALL = 0xFFFFFFFF


def feat(features):
    """A feat table: (type, exclusive, settings) for each feature type, sorted
    by type, each setting named by name ID 256."""
    head = struct.pack(">IHHI", 0x00010000, len(features), 0, 0)
    records, settings = b"", b""
    at = 12 + 12 * len(features)
    for typ, exclusive, values in features:
        offset = at + len(settings)
        records += struct.pack(">HHIHh", typ, len(values), offset, 0x8000 if exclusive else 0, 256)
        settings += b"".join(struct.pack(">Hh", v, 256) for v in values)
    return head + records + settings


def feature_chain():
    letters = [("G", 0x01), ("H", 0x02), ("I", 0x04), ("J", 0x08), ("K", 0x10), ("L", 0x20), ("M", 0x40),
               ("N", 0x80), ("O", 0x100)]
    subtables = [subtable(4, bit, single_lookup([(FEATURE_GID[g], FEATURE_GID["X"])])) for g, bit in letters]
    features = [
        (36, 1, 0, ALL & ~0x01),  # contextual alternates off
        (36, 0, 0x01, ALL),  # contextual alternates on
        (1, 2, 0x02, ALL),  # common ligatures on
        (1, 3, 0, ALL & ~0x02),  # common ligatures off
        (3, 3, 0x04, ALL),  # small capitals, deprecated
        (37, 1, 0x40, ALL),  # small capitals
        (17, 1, 0x08, ALL),  # character alternatives
        (21, 0, 0x20, ALL & ~0x10),  # oldstyle figures
        (21, 1, 0x10, ALL & ~0x20),  # lining figures
        (22, 1, 0x80, ALL),  # full-width text
        (22, 2, 0x100, ALL),  # half-width text
    ]
    body = b"".join(struct.pack(">HHII", *f) for f in features) + b"".join(subtables)
    head = struct.pack(">IIII", 0x01, 16 + len(body), len(features), len(subtables))
    return struct.pack(">HHI", 2, 0, 1) + head + body


FEAT_TYPES = [(1, False, [2, 3]), (17, True, [0, 1]), (21, True, [0, 1]), (22, True, [0, 1, 2]),
              (36, False, [0, 1])]


def build_features(directory):
    letters = "GHIJKLMNO"
    save(os.path.join(directory, "MorxFeatures.ttf"), "MorxFeatures", FEATURE_GLYPHS, letters,
         {"morx": feature_chain(), "feat": feat(FEAT_TYPES + [(37, True, [0, 1])])})
    save(os.path.join(directory, "MorxFeaturesDeprecated.ttf"), "MorxFeaturesDeprecated", FEATURE_GLYPHS,
         letters, {"morx": feature_chain(), "feat": feat([(1, False, [2, 3]), (3, True, [0, 3])] + FEAT_TYPES[1:])})
    save(os.path.join(directory, "MorxFeaturesNoFeat.ttf"), "MorxFeaturesNoFeat", FEATURE_GLYPHS, letters,
         {"morx": feature_chain()})


if __name__ == "__main__":
    build(os.path.join(sys.argv[1], "MorxCases.ttf"))
    build_features(sys.argv[1])
