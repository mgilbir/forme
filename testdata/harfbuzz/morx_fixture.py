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


# MortCases.ttf: a mort, the morx's predecessor, which HarfBuzz reads where a
# face has no morx: sixteen-bit fields, a class table of a first glyph and a
# byte a glyph, state cells of a byte, and states, actions, components,
# ligatures and substitutions named by their byte offsets from the state
# table. One chain, a subtable of each kind, each on letters of its own:
#
#   G H I      rearranged: G marks the first, I the last, and AxD => DxA
#   J K        contextual: J is marked; at K, the mark becomes Z and K
#              becomes Y
#   L M        a ligature, N
#   O          X inserted after it
#   P          noncontextual, P becomes Y
MORT_GLYPHS = [".notdef", "G", "H", "I", "J", "K", "L", "M", "N", "O", "P", "X", "Y", "Z"]
MGID = {name: i for i, name in enumerate(MORT_GLYPHS)}


def obsolete_table(first, classes, rows, entry_bytes, extra_count, extra_blobs):
    """An obsolete state table's body: its header of nClasses and the three
    offsets, then extra_count offsets of its own tables; the class table, a
    first glyph and a class a byte; the state array, a byte a cell; the
    entries; then the extra tables. rows are each state's entry indices, and
    entry_bytes a function of the state array's offset, since an entry names
    its next state by the byte offset of its row."""
    n_classes = len(rows[0])
    head = 8 + 2 * extra_count
    class_table = struct.pack(">HH", first, len(classes)) + bytes(classes)
    if len(class_table) % 2:
        class_table += b"\0"
    state_at = head + len(class_table)
    state_array = b"".join(bytes(r) for r in rows)
    while len(state_array) % 2:
        state_array += b"\0"
    entries_at = state_at + len(state_array)
    entries = entry_bytes(state_at, n_classes)
    at = entries_at + len(entries)
    offsets, blobs = [], b""
    for blob in extra_blobs:
        offsets.append(at + len(blobs))
        blobs += blob
        while len(blobs) % 2:
            blobs += b"\0"
    body = struct.pack(">HHHH", n_classes, head, state_at, entries_at)
    body += b"".join(struct.pack(">H", o) for o in offsets)
    return body + class_table + state_array + entries + blobs


def mort_subtable(kind, flags, body):
    return struct.pack(">HHI", 8 + len(body), kind, flags) + body


def mort():
    FIRST = MGID["G"]

    def classes(mapping):
        """Classes for G to P: each glyph named in mapping, the rest out of
        bounds."""
        return [mapping.get(g, 1) for g in range(FIRST, MGID["P"] + 1)]

    def row_state(state_at, n_classes, row):
        return state_at + row * n_classes

    # Rearrangement: classes 4 = G, 5 = H, 6 = I.
    def rearrangement_entries(state_at, n):
        mark_first, mark_last, verb = 0x8000, 0x2000, 3
        return b"".join(struct.pack(">HH", row_state(state_at, n, r), f) for r, f in [
            (0, 0),  # 0: nothing, to the start
            (2, mark_first),  # 1: G marks the first, into state 2
            (2, 0),  # 2: stay in state 2
            (0, mark_last | verb),  # 3: I marks the last and rearranges
        ])
    rows = [[0, 0, 0, 0, 1, 0, 0], [0, 0, 0, 0, 1, 0, 0], [0, 0, 0, 0, 1, 2, 3]]
    rearrangement = mort_subtable(0, 1, obsolete_table(
        FIRST, classes({MGID["G"]: 4, MGID["H"]: 5, MGID["I"]: 6}), rows, rearrangement_entries, 0, []))

    # Contextual: classes 4 = J, 5 = K. A substitution is the glyph at twice
    # (index + glyph) bytes from the state table: the table below is laid out
    # so that index 0 at J is Z and index 1 at K is Y, past its first word.
    def contextual_entries(state_at, n):
        sub_at = contextual_entries.sub_at
        mark = sub_at // 2 - MGID["J"]
        current = sub_at // 2 + 1 - MGID["K"]
        return b"".join(struct.pack(">HHhh", row_state(state_at, n, r), f, m, c) for r, f, m, c in [
            (0, 0, 0, 0),
            (2, 0x8000, 0, 0),  # J: marked, into state 2
            (0, 0, mark, current),  # K: the mark and K substituted
        ])
    subs = struct.pack(">HH", MGID["Z"], MGID["Y"])
    rows = [[0, 0, 0, 0, 1, 0], [0, 0, 0, 0, 1, 0], [0, 0, 0, 0, 1, 2]]
    # The offsets depend on the table's own layout: build it once to learn
    # where the substitutions land, then again with them.
    contextual_entries.sub_at = 0
    probe = obsolete_table(FIRST, classes({MGID["J"]: 4, MGID["K"]: 5}), rows, contextual_entries, 1, [subs])
    contextual_entries.sub_at = struct.unpack(">H", probe[8:10])[0]
    contextual = mort_subtable(1, 1, obsolete_table(
        FIRST, classes({MGID["J"]: 4, MGID["K"]: 5}), rows, contextual_entries, 1, [subs]))

    # Ligature: classes 4 = L, 5 = M. L pushes itself; M pushes itself and runs
    # the actions at the offset its flags hold: pop M, then pop L and store the
    # ligature the two components' values sum to — the byte offset of N in the
    # ligature table.
    def ligature_entries(actions_at):
        def entries(state_at, n):
            return b"".join(struct.pack(">HH", row_state(state_at, n, r), f) for r, f in [
                (0, 0),
                (2, 0x8000),  # L: a component, into state 2
                (0, 0x8000 | actions_at),  # M: a component, and the actions
            ])
        return entries

    rows = [[0, 0, 0, 0, 1, 0], [0, 0, 0, 0, 1, 0], [0, 0, 0, 0, 1, 2]]
    lig_classes = classes({MGID["L"]: 4, MGID["M"]: 5})
    # Laid out once with the three tables at their sizes, to learn where they
    # land, since what they hold is their offsets.
    probe = obsolete_table(FIRST, lig_classes, rows, ligature_entries(0), 3,
                           [b"\0" * 8, b"\0" * 4, b"\0" * 2])
    actions_at, components_at, ligatures_at = struct.unpack(">HHH", probe[8:14])

    def component_offset(glyph, index):
        """The action's offset that names the index-th word of the component
        table at a glyph: a word offset from the state table, less the glyph."""
        return components_at // 2 + index - glyph

    actions = struct.pack(">II", component_offset(MGID["M"], 0) & 0x3FFFFFFF,
                          0xC0000000 | (component_offset(MGID["L"], 1) & 0x3FFFFFFF))
    components = struct.pack(">HH", 0, ligatures_at)
    ligatures = struct.pack(">H", MGID["N"])
    ligature = mort_subtable(2, 1, obsolete_table(FIRST, lig_classes, rows, ligature_entries(actions_at), 3,
                                                  [actions, components, ligatures]))

    # Insertion: class 4 = O, after which X is inserted.
    def insertion_entries(state_at, n):
        current_count = 1 << 5
        return b"".join(struct.pack(">HHHH", row_state(state_at, n, r), f, c, m) for r, f, c, m in [
            (0, 0, FFFF, FFFF),
            (0, current_count, 0, FFFF),  # O: X after it
        ])
    rows = [[0, 0, 0, 0, 1], [0, 0, 0, 0, 1]]
    insertion = mort_subtable(5, 1, obsolete_table(
        FIRST, classes({MGID["O"]: 4}), rows, insertion_entries, 1, [struct.pack(">H", MGID["X"])]))

    noncontextual = mort_subtable(4, 1, single_lookup([(MGID["P"], MGID["Y"])]))

    subtables = [rearrangement, contextual, ligature, insertion, noncontextual]
    body = b"".join(subtables)
    chain = struct.pack(">IIHH", 1, 12 + len(body), 0, len(subtables)) + body
    return struct.pack(">HHI", 1, 0, 1) + chain


# MorxLanguage.ttf: a morx chain whose features follow the run's language,
# type 39, each setting one more than the index of a tag in the ltag table,
# which states "tr", "ZH_Hant" and "sr-Latn". Each feature turns on a subtable
# turning its letter into X:
#
#   G  setting 1, "tr", which "tr", "TR" and "tr-TR" are, and "trk" is not
#   H  setting 2, "ZH_Hant", which HarfBuzz reads as "zh-hant": "zh-Hant"
#      and "zh_hant_TW" are, and "zh" is not
#   K  setting 3, "sr-Latn": "sr-Latn-RS" is, "sr" is not
#   I  setting 9, which names no tag: no language, which only a run with no
#      language matches
#   J  setting 0, which names nothing and is never on
LANGUAGE_GLYPHS = [".notdef", "G", "H", "I", "J", "K", "X"]
LANGUAGE_GID = {name: i for i, name in enumerate(LANGUAGE_GLYPHS)}


def ltag(tags):
    """An ltag table: version 1, no flags, and each tag's range."""
    at = 12 + 4 * len(tags)
    ranges, strings = b"", b""
    for t in tags:
        ranges += struct.pack(">HH", at + len(strings), len(t))
        strings += t.encode("ascii")
    return struct.pack(">III", 1, 0, len(tags)) + ranges + strings


def language_chain():
    letters = [("G", 0x02), ("H", 0x04), ("K", 0x08), ("I", 0x10), ("J", 0x20)]
    subtables = [subtable(4, bit, single_lookup([(LANGUAGE_GID[g], LANGUAGE_GID["X"])])) for g, bit in letters]
    features = [(39, 1, 0x02, ALL), (39, 2, 0x04, ALL), (39, 3, 0x08, ALL), (39, 9, 0x10, ALL), (39, 0, 0x20, ALL)]
    body = b"".join(struct.pack(">HHII", *f) for f in features) + b"".join(subtables)
    head = struct.pack(">IIII", 0x01, 16 + len(body), len(features), len(subtables))
    return struct.pack(">HHI", 2, 0, 1) + head + body


def build_language(directory):
    save(os.path.join(directory, "MorxLanguage.ttf"), "MorxLanguage", LANGUAGE_GLYPHS, "GHIJK",
         {"morx": language_chain(), "ltag": ltag(["tr", "ZH_Hant", "sr-Latn"])})


def build_mort(directory):
    save(os.path.join(directory, "MortCases.ttf"), "MortCases", MORT_GLYPHS, "GHIJKLMNOP", {"mort": mort()})


# MorxRunaway.ttf: two machines that never advance, each spending the run's
# allowance (max_ops) where HarfBuzz charges for marking glyphs it may not
# break between, so that where HarfBuzz gives up on the run depends on those
# charges being made as it makes them:
#
#   - a contextual subtable: B marks itself; A, after it, substitutes the
#     marked glyph (B, C, D and round again) and does not advance, which
#     HarfBuzz charges as unsafe to break from the mark to past A;
#   - an insertion subtable: E marks itself; F, after it, inserts X after the
#     marked glyph and does not advance, which HarfBuzz charges as unsafe to
#     break from the mark, in the output, to past F.
#
# A alone, before any B, stays on itself doing nothing, which spends the
# allowance without anything charged for marking; and B and E alone do
# nothing at all.
RUNAWAY_GLYPHS = [".notdef", "A", "B", "C", "D", "E", "F", "X"]
RUNAWAY_GID = {name: i for i, name in enumerate(RUNAWAY_GLYPHS)}
DONT_ADVANCE = 0x4000
SET_MARK = 0x8000


def runaway():
    g = RUNAWAY_GID
    row = [0, 0, 0, 0, 1, 2]
    entries = [
        (0, 0, FFFF, FFFF),  # nothing
        (0, SET_MARK, FFFF, FFFF),  # B: mark it
        (0, DONT_ADVANCE, 0, FFFF),  # A: substitute the mark through lookup 0, and stay
    ]
    lookup = single_lookup([(g["B"], g["C"]), (g["C"], g["D"]), (g["D"], g["B"])])
    contextual = subtable(1, 1, state_table({g["B"]: 4, g["A"]: 5}, [row, row], entries, [None],
                                            [struct.pack(">I", 4) + lookup]))
    entries = [
        (0, 0, FFFF, FFFF),  # nothing
        (0, SET_MARK, FFFF, FFFF),  # E: mark it
        (0, DONT_ADVANCE | 1, FFFF, 0),  # F: insert list[0] after the mark, and stay
    ]
    insertion = subtable(5, 1, state_table({g["E"]: 4, g["F"]: 5}, [row, row], entries, [None],
                                           [struct.pack(">H", g["X"])]))
    return struct.pack(">HHI", 2, 0, 2) + chain(1, [contextual]) + chain(1, [insertion])


def build_runaway(directory):
    save(os.path.join(directory, "MorxRunaway.ttf"), "MorxRunaway", RUNAWAY_GLYPHS, "ABEF",
         {"morx": runaway()})


if __name__ == "__main__":
    build(os.path.join(sys.argv[1], "MorxCases.ttf"))
    build_features(sys.argv[1])
    build_mort(sys.argv[1])
    build_language(sys.argv[1])
    build_runaway(sys.argv[1])
