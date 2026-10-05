# Builds the bitmap-strike faces shape/strikes_test.go paints and strikes.py
# has FreeType load:
#
#   python3 strikes_fixture.py DIR
#
# Strikes.ttf has no outlines, only monochrome and greyscale bitmap strikes in
# EBLC and EBDT: one strike for each bit depth, 1, 2, 4 and 8, at 12, 16, 20 and
# 24 pixels per em, and a fifth at 32, of one bit, holding a few glyphs only.
# Each of the first four holds every glyph, in index subtables of every format,
# 1 to 5, and in images of every format, 1, 2, 5, 6 and 7, and the composites
# of 8 and 9, which place other glyphs of the strike, one of them a composite
# itself. A format 4 and a format 5 subtable each leave a glyph of their range
# out, and a format 1 subtable states a glyph of its range with no image.
#
# StrikesApple.ttf is the same strikes as Apple's tables name them: bloc and
# bdat for EBLC and EBDT, and bhed for head, which is how an Apple font with
# no outlines is made.
#
# The tables are written byte by byte here, rather than through fontTools'
# EBLC and EBDT, so that which format holds which glyph is this file's choice
# and not the compiler's. fontTools builds the rest.
import os
import struct
import sys

from fontTools.fontBuilder import FontBuilder
from fontTools.ttLib import TTFont
from fontTools.ttLib.tables.DefaultTable import DefaultTable

out_dir = sys.argv[1]

UPEM = 1000
NUM_GLYPHS = 26  # .notdef and 25 more


# The index subtables of a full strike: (index format, image format, first
# glyph, last glyph, glyphs of the range left out). Glyphs 20 to 22 are
# composites: 20 and 21 of simple glyphs, 22 of a composite (20) and of glyphs
# whose metrics are their subtable's.
SUBTABLES = [
    (1, 1, 1, 2, ()),
    (1, 2, 3, 4, ()),
    (3, 6, 5, 6, ()),
    (3, 7, 7, 8, ()),
    (2, 5, 9, 10, ()),
    (4, 1, 11, 13, (12,)),
    (5, 5, 14, 16, (15,)),
    (4, 7, 17, 18, ()),
    (1, 2, 19, 19, (19,)),  # a glyph stated with no image
    (1, 8, 20, 21, ()),
    (3, 9, 22, 22, ()),
    (2, 5, 23, 25, ()),
]

# The components of each composite: (glyph, x offset, y offset), from the
# composite's top left.
COMPONENTS = {
    20: [(1, 0, 0), (3, 2, 1)],
    21: [(5, 1, 0), (7, 0, 2), (11, 3, 3)],
    22: [(20, 0, 0), (14, 4, 2), (9, 1, 5)],
}

# The strikes: (ppem, bit depth, which subtables). The last holds only the
# first two subtables, so that a glyph of the others is not in it.
STRIKES = [
    (12, 1, SUBTABLES),
    (16, 2, SUBTABLES),
    (20, 4, SUBTABLES),
    (24, 8, SUBTABLES),
    (32, 1, SUBTABLES[:2]),
]


def metrics(strike, gid):
    """A glyph's own size and placement in a strike: width, height, bearing
    across, bearing up and advance, in pixels. Odd widths, so that rows end
    inside a byte, and some bearings below zero."""
    w = 1 + (gid * 5 + strike * 3) % 11
    h = 2 + (gid * 3 + strike) % 10
    bx = gid % 5 - 2
    by = h - gid % 4 - 1
    return w, h, bx, by, w + 1


class Rng:
    def __init__(self, seed):
        self.s = seed

    def next(self):
        self.s = (self.s * 1103515245 + 12345) & 0x7FFFFFFF
        return self.s >> 16


def pixels(strike, gid, w, h, depth):
    """A glyph's samples, row by row, each from 0 to 2^depth-1: random, with
    the corners set to the extremes so that each depth's full range is
    used."""
    top = (1 << depth) - 1
    r = Rng(strike * 1000 + gid)
    px = [r.next() % (top + 1) for _ in range(w * h)]
    px[0] = top
    px[-1] = 0 if w * h > 1 else top
    return px


def byte_aligned(px, w, h, depth):
    out = bytearray()
    for y in range(h):
        bits, n = 0, 0
        for x in range(w):
            bits = bits << depth | px[y * w + x]
            n += depth
            while n >= 8:
                out.append(bits >> (n - 8) & 0xFF)
                n -= 8
        if n:
            out.append(bits << (8 - n) & 0xFF)
    return bytes(out)


def bit_aligned(px, depth):
    out = bytearray()
    bits, n = 0, 0
    for v in px:
        bits = bits << depth | v
        n += depth
        while n >= 8:
            out.append(bits >> (n - 8) & 0xFF)
            n -= 8
    if n:
        out.append(bits << (8 - n) & 0xFF)
    return bytes(out)


def small(w, h, bx, by, adv):
    return struct.pack(">BBbbB", h, w, bx, by, adv)


def big(w, h, bx, by, adv):
    return struct.pack(">BBbbBbbB", h, w, bx, by, adv, -(w // 2), 0, h + 1)


def composite_size(strike, gid, sizes):
    w = h = 0
    for c, dx, dy in COMPONENTS[gid]:
        cw, ch = sizes[c]
        w, h = max(w, dx + cw), max(h, dy + ch)
    return w, h


def strike_tables(si, depth, subtables):
    """One strike's index subtables, offsets from the start of the strike's
    IndexSubTableArray, and the EBDT bytes they name, offsets from base."""
    # The size of each glyph, which composites are made to hold, and the
    # metrics a format 5 subtable shares, which are its first glyph's.
    sizes, shared = {}, {}
    for index, image, first, last, _ in subtables:
        for gid in range(first, last + 1):
            if gid in COMPONENTS:
                continue
            m = metrics(si, gid if index not in (2, 5) else first)
            sizes[gid] = m[:2]
    for gid in sorted(COMPONENTS):
        if any(first <= gid <= last for _, _, first, last, _ in subtables):
            sizes[gid] = composite_size(si, gid, sizes)
    images = {}

    def image_of(index, image, first, gid):
        if gid in COMPONENTS:
            w, h = sizes[gid]
            bx, by, adv = gid % 3 - 1, h - 2, w + 1
            comps = b"".join(struct.pack(">Hbb", c, dx, dy) for c, dx, dy in COMPONENTS[gid])
            n = struct.pack(">H", len(COMPONENTS[gid]))
            if image == 8:
                return small(w, h, bx, by, adv) + b"\0" + n + comps
            return big(w, h, bx, by, adv) + n + comps
        w, h, bx, by, adv = metrics(si, gid if index not in (2, 5) else first)
        px = pixels(si, gid, w, h, depth)
        if image == 1:
            return small(w, h, bx, by, adv) + byte_aligned(px, w, h, depth)
        if image == 2:
            return small(w, h, bx, by, adv) + bit_aligned(px, depth)
        if image == 5:
            return bit_aligned(px, depth)
        if image == 6:
            return big(w, h, bx, by, adv) + byte_aligned(px, w, h, depth)
        if image == 7:
            return big(w, h, bx, by, adv) + bit_aligned(px, depth)
        raise ValueError(image)

    return sizes, image_of


def build_tables():
    eblc_sizes, eblc_rest = [], bytearray()
    ebdt = bytearray(struct.pack(">L", 0x00020000))
    # Where in EBLC what follows the size tables starts.
    head = 8 + 48 * len(STRIKES)
    for si, (ppem, depth, subtables) in enumerate(STRIKES):
        sizes, image_of = strike_tables(si, depth, subtables)
        array_at = head + len(eblc_rest)
        array = bytearray()
        bodies = bytearray()
        body_at = 8 * len(subtables)
        for index, image, first, last, missing in subtables:
            base = len(ebdt)
            sub = struct.pack(">HHL", index, image, base)
            present = [g for g in range(first, last + 1) if g not in missing]
            if index in (1, 3):
                offs, data = [], bytearray()
                for gid in range(first, last + 1):
                    offs.append(len(data))
                    if gid not in missing:
                        data += image_of(index, image, first, gid)
                    if index == 3 and len(data) % 2:
                        data += b"\0"
                offs.append(len(data))
                fmt = ">L" if index == 1 else ">H"
                sub += b"".join(struct.pack(fmt, o) for o in offs)
                ebdt += data
            elif index in (2, 5):
                imgs = [image_of(index, image, first, g) for g in present]
                size = max(len(b) for b in imgs)
                w, h, bx, by, adv = metrics(si, first)
                sub += struct.pack(">L", size) + big(w, h, bx, by, adv)
                if index == 5:
                    sub += struct.pack(">L", len(present))
                    sub += b"".join(struct.pack(">H", g) for g in present)
                for b in imgs:
                    ebdt += b + b"\0" * (size - len(b))
            elif index == 4:
                pairs, data = [], bytearray()
                for gid in present:
                    pairs.append((gid, len(data)))
                    data += image_of(index, image, first, gid)
                pairs.append((0, len(data)))
                sub += struct.pack(">L", len(present))
                sub += b"".join(struct.pack(">HH", g, o) for g, o in pairs)
                ebdt += data
            while len(sub) % 4:
                sub += b"\0"
            while len(ebdt) % 4:
                ebdt += b"\0"
            array += struct.pack(">HHL", first, last, body_at + len(bodies))
            bodies += sub
        eblc_rest += array + bodies
        line = struct.pack(">bbBbbbbbbbbb", round(ppem * 0.8), -round(ppem * 0.2), 12, 1, 0, 0, 0, 0, 0, 0, 0, 0)
        first = min(s[2] for s in subtables)
        last = max(s[3] for s in subtables)
        eblc_sizes.append(struct.pack(">LLLL", array_at, len(array) + len(bodies), len(subtables), 0)
                          + line + line + struct.pack(">HHBBBb", first, last, ppem, ppem, depth, 1))
    eblc = struct.pack(">LL", 0x00020000, len(STRIKES)) + b"".join(eblc_sizes) + eblc_rest
    return bytes(eblc), bytes(ebdt)


def raw(tag, data):
    t = DefaultTable(tag)
    t.data = data
    return t


def build(path, apple):
    names = [".notdef"] + [f"g{i}" for i in range(1, NUM_GLYPHS)]
    fb = FontBuilder(UPEM, isTTF=True)
    fb.setupGlyphOrder(names)
    fb.setupCharacterMap({0x40 + i: f"g{i}" for i in range(1, NUM_GLYPHS)})
    fb.setupHorizontalMetrics({n: (600, 0) for n in names})
    fb.setupHorizontalHeader(ascent=800, descent=-200)
    fb.setupNameTable({"familyName": "Strikes", "styleName": "Regular"})
    fb.setupOS2(sTypoAscender=800, sTypoDescender=-200, usWinAscent=800, usWinDescent=200)
    fb.setupPost()
    fb.setupMaxp()
    fb.setupHead(unitsPerEm=UPEM)
    font = fb.font
    eblc, ebdt = build_tables()
    font["EBLC" if not apple else "bloc"] = raw("EBLC" if not apple else "bloc", eblc)
    font["EBDT" if not apple else "bdat"] = raw("EBDT" if not apple else "bdat", ebdt)
    font.save(path)
    if apple:
        # head renamed bhed, as fontTools has no word for it.
        f = TTFont(path)
        f["bhed"] = raw("bhed", f.getTableData("head"))
        del f["head"]
        f.save(path, reorderTables=True)


os.makedirs(out_dir, exist_ok=True)
build(os.path.join(out_dir, "Strikes.ttf"), False)
build(os.path.join(out_dir, "StrikesApple.ttf"), True)
