# Asks HarfBuzz how CFF2 variable fonts draw at several locations, and
# fontTools how it cuts a static instance of them there, and writes both, so
# that shape/cff2_test.go can hold shape/cff2.go and shape/cff2cff.go to them.
#
#   make hbcff2
#
# The same bargain as every oracle here: the output is checked in.
#
# # Two oracles, for two questions
#
# HarfBuzz is what draws a variable font on a screen. At each location it is
# asked for a sample of glyphs' extents (hb_font_get_glyph_extents), their
# outlines (its draw functions, point for point, as the floats it hands them
# over), their advances, and for a face with vertical metrics their vertical
# advances and origins; and for the normalized coordinates it reached, so that
# the reader can be run at exactly its location.
#
# fontTools' instancer is what cuts a static font from a variable one, and a
# face from LoadInstance is such a font. At each location fontTools instances
# the font and downgrades its CFF2 to CFF (instantiateVariableFont with
# downgradeCFF2), and is asked for the instance's advances, its outlines as its
# own pen draws them, each glyph's bounds, and — of HarfBuzz — the extents of
# the instance's glyphs, which is what a face cut here measures its ink from.
#
# The faces are named on the command line as NAME=PATH: the CFF2Blend.otf
# fixture (cff2_fixture.py), built for what the real fonts never do, and the
# CFF2 fonts of `make cff-fonts`. A face whose file is not there stops the run.
import hashlib
import io
import os
import sys

from oracle import fonttools, harfbuzz

hb = harfbuzz()
ft = fonttools()
from fontTools.pens.boundsPen import BoundsPen  # noqa: E402
from fontTools.pens.recordingPen import RecordingPen  # noqa: E402
from fontTools.ttLib import TTFont  # noqa: E402
from fontTools.varLib import instancer  # noqa: E402

out_path = sys.argv[1]
faces = [a.split("=", 1) for a in sys.argv[2:]]

# The locations each face is drawn at, in user coordinates: the default, both
# ends of an axis, a point between avar's segments or on a half, and two axes
# at once.
LOCATIONS = {
    # The fixture's: where its halves fall, where the order of its scalars
    # decides a rounding (see cff2_fixture.py), and both axes at their ends.
    "CFF2Blend.otf": [{}, {"wght": 8192}, {"wght": 1311, "XOPQ": 500},
                      {"wght": 16384, "XOPQ": 16384}],
    "SourceSans3VF-Upright.otf": [{}, {"wght": 900}, {"wght": 550}, {"wght": 437}],
    "SourceSerif4Variable-Roman.otf": [
        {}, {"wght": 700, "opsz": 60}, {"wght": 300, "opsz": 8}, {"wght": 650}],
    "NotoSansJP-VF.otf": [{}, {"wght": 900}, {"wght": 400}],
}

# How many glyphs of each face are sampled for their extents and advances,
# and how many of those for their outlines.
SAMPLE = 160
OUTLINES = 32
# The whole of each face is compared out of tree by setting both to a number
# past its glyph count, e.g. CFF2_SAMPLE=70000 CFF2_OUTLINES=70000; the file
# checked in holds the sample, so that it stays small.
SAMPLE = int(os.environ.get("CFF2_SAMPLE", SAMPLE))
OUTLINES = int(os.environ.get("CFF2_OUTLINES", OUTLINES))


def label(loc):
    return ",".join(f"{k}={v}" for k, v in sorted(loc.items())) or "default"


def num(v):
    v = float(v)
    return str(int(v)) if v == int(v) else repr(v)


class Pen:
    """Records what a draw call hands over, in its own order."""

    def __init__(self):
        self.ops = []

    def moveTo(self, p):
        self.ops.append("M " + " ".join(map(num, p)))

    def lineTo(self, p):
        self.ops.append("L " + " ".join(map(num, p)))

    def curveTo(self, *pts):
        self.ops.append("C " + " ".join(num(c) for p in pts for c in p))

    def qCurveTo(self, *pts):
        sys.exit("a quadratic curve from a CFF2 font")

    def closePath(self):
        self.ops.append("Z")

    def endPath(self):
        self.ops.append("Z")


def hints(inst, g):
    """A glyph's hints in fontTools' instance: its horizontal and its vertical
    stems as the edges they come to, each stem operator's first edge from
    zero, and its masks' bytes, in order."""
    from fontTools.cffLib.specializer import programToCommands
    cs = inst["CFF "].cff.topDictIndex[0].CharStrings[g]
    cs.decompile()
    h, v, masks = [], [], []
    seen_mask = False
    commands = programToCommands(cs.program)
    for i, (op, args) in enumerate(commands):
        stems = None
        if op in ("hstem", "hstemhm"):
            stems = h
        elif op in ("vstem", "vstemhm"):
            stems = v
        elif op == "" and i + 1 < len(commands) and commands[i + 1][0] in ("hintmask", "cntrmask") \
                and not seen_mask:
            stems = v  # the stems a first mask declares by its operands
        elif op in ("hintmask", "cntrmask"):
            seen_mask = True
            masks.append(commands[i + 1][1][0].hex())
        if stems is not None:
            pos = 0
            for a in args:
                pos += a
                stems.append(num(pos))
    return "h " + " ".join(h) + " v " + " ".join(v) + " m " + " ".join(masks)


def extents(font, gid):
    e = font.get_glyph_extents(gid)
    if e is None:
        return "none"
    return f"{e.x_bearing} {e.y_bearing} {e.width} {e.height}"


out = []
for name, path in faces:
    try:
        data = open(path, "rb").read()
    except FileNotFoundError:
        sys.exit(f"{name}: {path} is not there. Fetch it (make cff-fonts) and run again.")
    out.append(f"face {name} {hashlib.sha256(data).hexdigest()}")
    face = hb.Face(data)
    n = face.glyph_count
    sample = sorted(set(range(0, n, max(1, n // SAMPLE))) | {n - 1})
    outlines = set(sample[:: max(1, len(sample) // OUTLINES)])
    vertical = "vmtx" in TTFont(io.BytesIO(data))
    fixture = name == "CFF2Blend.otf"
    for loc in LOCATIONS[name]:
        out.append(f"loc {label(loc)}")
        font = hb.Font(face)
        if loc:
            font.set_variations(loc)
        coords = [round(c * 16384) for c in font.get_var_coords_normalized()]
        out.append("coords " + " ".join(map(str, coords)))
        for gid in sample:
            out.append(f"E {gid} {extents(font, gid)}")
            line = f"A {gid} {font.get_glyph_h_advance(gid)}"
            if vertical:
                x, y = font.get_glyph_v_origin(gid)
                line += f" {font.get_glyph_v_advance(gid)} {x} {y}"
            out.append(line)
            if gid in outlines:
                pen = Pen()
                font.draw_glyph_with_pen(gid, pen)
                out.append(f"P {gid} " + " ".join(pen.ops))

        # static: every axis the location does not name pinned at its
        # default, so that what comes back is a static font, as a face from
        # LoadInstance is.
        inst = instancer.instantiateVariableFont(
            TTFont(io.BytesIO(data)), loc, downgradeCFF2=True, static=True)
        buf = io.BytesIO()
        inst.save(buf)
        idata = buf.getvalue()
        inst = TTFont(io.BytesIO(idata))
        order = inst.getGlyphOrder()
        glyphs = inst.getGlyphSet()
        ifont = hb.Font(hb.Face(idata))
        for gid in sample:
            g = order[gid]
            line = f"I {gid} {inst['hmtx'][g][0]}"
            if vertical:
                line += f" {inst['vmtx'][g][0]}"
            out.append(line)
            out.append(f"IE {gid} {extents(ifont, gid)}")
            bp = BoundsPen(glyphs)
            glyphs[g].draw(bp)
            b = bp.bounds
            out.append(f"IB {gid} " + ("none" if b is None else " ".join(map(num, b))))
            if gid in outlines:
                pen = Pen()
                glyphs[g].draw(pen)
                out.append(f"IP {gid} " + " ".join(pen.ops))
            if fixture:
                out.append(f"IH {gid} " + hints(inst, g))
        if fixture:
            top = inst["CFF "].cff.topDictIndex[0]
            for i, fd in enumerate(top.FDArray):
                p = fd.Private
                for key in ("BlueValues", "OtherBlues", "StdHW", "StdVW", "StemSnapH", "StemSnapV"):
                    if hasattr(p, key):
                        v = getattr(p, key)
                        v = v if isinstance(v, list) else [v]
                        out.append(f"IV {i} {key} " + " ".join(map(num, v)))
        os2, post = inst["OS/2"], inst["post"]
        out.append(f"IM weight {os2.usWeightClass} width {os2.usWidthClass} "
                   f"italic {num(post.italicAngle)} xheight {os2.sxHeight} "
                   f"capheight {os2.sCapHeight} strikeout {os2.yStrikeoutPosition} "
                   f"{os2.yStrikeoutSize}")

with open(out_path, "w", encoding="utf-8") as w:
    w.write("# Generated by testdata/harfbuzz/cff2.py. DO NOT EDIT.\n")
    w.write("#\n")
    w.write("# For each face and location: the normalized coordinates HarfBuzz\n")
    w.write("# reached (F2Dot14); per sampled glyph, HarfBuzz's extents (E), its\n")
    w.write("# advance and, for a face with vertical metrics, its vertical advance and\n")
    w.write("# origin (A), and for some its outline as HarfBuzz draws it (P). Then\n")
    w.write("# fontTools' instance there: each glyph's advances (I), HarfBuzz's\n")
    w.write("# extents of the instance's glyph (IE), fontTools' bounds of it (IB), for\n")
    w.write("# some its outline (IP), and the instance's font-wide numbers (IM); for\n")
    w.write("# the fixture, each glyph's hints (IH: its stems' edges and its masks) and\n")
    w.write("# each Font DICT's Private DICT numbers (IV).\n")
    w.write(f"# harfbuzz {hb.version_string()}\n")
    w.write(f"# uharfbuzz {hb.__version__}\n")
    w.write(f"# fonttools {ft.version}\n")
    for line in out:
        w.write(line + "\n")
