# Instances the faces pointmatch_fixture.py builds, whose composites place
# components by matching points, and writes what three other implementations
# say of them, so that shape/pointmatch_test.go can hold LoadInstance to it.
#
#   make hbpointmatch
#
# Faces are named on the command line as NAME=PATH, and each is asked at the
# weights in WEIGHTS:
#
#   H     HarfBuzz drawing the variable face at the weight: a glyph's path, each
#         point of it as HarfBuzz hands it to a pen, M for the start of a
#         contour and L for a line; the face's outlines are all lines
#   I     HarfBuzz's own instancer (hb-subset with the axis pinned): each
#         glyph's bounding box, its advance and left side bearing, and for a
#         composite each component — glyph, flags, and either its offset or
#         its two matched points, then its 2x2 in F2Dot14
#   F, P  fontTools' instancer: the same as I, and each glyph's points
#         flattened by fontTools, for a face fontTools can instance
#   S     HarfBuzz drawing fontTools' instance: each glyph's path as H, with
#         every left side bearing first set to the glyph's xMin, so that
#         nothing is shifted and the points are the instance's own. It is how
#         HarfBuzz reads an instance whose matches were kept: where fontTools
#         and HarfBuzz count a nested composite's points differently (see
#         pointmatch_fixture.py's 'nested'), P is fontTools' count and S is
#         HarfBuzz's
#
# fontTools' instancer cannot instance PointMatchPhantom.ttf — it counts no
# phantom points, and a match naming one indexes past the end of its list —
# and says so in a 'fonttools' line rather than an F.
import hashlib
import io
import sys

from oracle import fonttools, harfbuzz

hb = harfbuzz()
fonttools()
from fontTools.pens.recordingPen import RecordingPen  # noqa: E402
from fontTools.ttLib import TTFont  # noqa: E402
from fontTools.varLib import instancer  # noqa: E402
from fontTools.varLib.instancer import OverlapMode  # noqa: E402

out_path = sys.argv[1]
faces = [a.split("=", 1) for a in sys.argv[2:]]

WEIGHTS = [100, 250, 400, 650, 900]

# The component flags an instancer decides for itself — how the arguments are
# stored, whether more follow, whether instructions do — and which the
# comparison leaves out.
DECIDED = 0x0001 | 0x0020 | 0x0100


def num(v):
    return repr(float(v))


def path(font, gid):
    pen = RecordingPen()
    font.draw_glyph_with_pen(gid, pen)
    out = []
    for op, args in pen.value:
        if op == "moveTo":
            out.append("M %s %s" % (num(args[0][0]), num(args[0][1])))
        elif op == "lineTo":
            out.append("L %s %s" % (num(args[0][0]), num(args[0][1])))
        elif op == "closePath":
            out.append("Z")
        else:
            sys.exit("glyph %d draws a %s, and the fixture is lines only" % (gid, op))
    return " ".join(out)


def f2(v):
    return round(v * 16384)


def records(ft):
    """Each glyph's box, advance, left side bearing and components."""
    glyf, hmtx = ft["glyf"], ft["hmtx"]
    out = []
    for gid, name in enumerate(ft.getGlyphOrder()):
        g = glyf[name]
        adv, lsb = hmtx[name]
        box = "%d %d %d %d" % (g.xMin, g.yMin, g.xMax, g.yMax) if g.numberOfContours else "empty"
        line = "%d %d %d %s" % (gid, adv, lsb, box)
        if g.isComposite():
            for c in g.components:
                t = getattr(c, "transform", [[1, 0], [0, 1]])
                matrix = "%d,%d,%d,%d" % (f2(t[0][0]), f2(t[0][1]), f2(t[1][0]), f2(t[1][1]))
                if hasattr(c, "firstPt"):
                    args = "match %d %d" % (c.firstPt, c.secondPt)
                else:
                    args = "at %d %d" % (c.x, c.y)
                line += " | %d %d %s %s" % (ft.getGlyphID(c.glyphName), c.flags & ~DECIDED & 0xFFFF,
                                           args, matrix)
        out.append(line)
    return out


out = []
for name, file in faces:
    data = open(file, "rb").read()
    digest = hashlib.sha256(data).hexdigest()
    face = hb.Face(data)
    for w in WEIGHTS:
        out.append(f"face {name}@wght={w} {digest}")
        font = hb.Font(face)
        font.set_variations({"wght": w})
        for gid in range(face.glyph_count):
            out.append(f"H {gid} {path(font, gid)}")
        inp = hb.SubsetInput()
        inp.sets(hb.SubsetInputSets.GLYPH_INDEX).set(set(range(face.glyph_count)))
        inp.flags = hb.SubsetFlags.RETAIN_GIDS | hb.SubsetFlags.NO_HINTING
        inp.pin_axis_location(face, "wght", w)
        pinned = hb.subset(face, inp)
        if pinned is None:
            out.append("harfbuzz cannot instance it")
        else:
            ft = TTFont(io.BytesIO(pinned.blob.data), recalcBBoxes=False)
            for line in records(ft):
                out.append("I " + line)
        try:
            ft = TTFont(io.BytesIO(data))
            # The overlap flags left as they were, which the instancer would
            # otherwise set on every glyph and HarfBuzz's does not.
            instancer.instantiateVariableFont(ft, {"wght": w}, inplace=True, optimize=False,
                                              updateFontNames=False,
                                              overlap=OverlapMode.KEEP_AND_DONT_SET_FLAGS)
            b = io.BytesIO()
            ft.save(b)
            ft = TTFont(io.BytesIO(b.getvalue()), recalcBBoxes=False)
        except IndexError as e:
            out.append(f"fonttools cannot instance it: {type(e).__name__}")
            continue
        for line in records(ft):
            out.append("F " + line)
        glyf = ft["glyf"]
        for gid, g in enumerate(ft.getGlyphOrder()):
            coords, _, _ = glyf[g].getCoordinates(glyf)
            out.append(f"P {gid} " + " ".join("%s,%s" % (num(x), num(y)) for x, y in coords))
        hmtx = ft["hmtx"]
        for g in ft.getGlyphOrder():
            box = glyf[g]
            hmtx[g] = (hmtx[g][0], box.xMin if box.numberOfContours else 0)
        b = io.BytesIO()
        ft.save(b)
        static = hb.Font(hb.Face(b.getvalue()))
        for gid in range(face.glyph_count):
            out.append(f"S {gid} {path(static, gid)}")

with open(out_path, "w", encoding="utf-8") as w:
    w.write("# Generated by testdata/harfbuzz/pointmatch.py. DO NOT EDIT.\n")
    w.write("#\n")
    w.write("# For each face at each weight: H is HarfBuzz's path of a glyph drawn\n")
    w.write("# at that weight; I is HarfBuzz's instancer's glyph, and F fontTools',\n")
    w.write("# each as glyph index, advance, left side bearing and box, then each\n")
    w.write("# component after a bar; P is fontTools' instance's points, flattened.\n")
    w.write(f"# harfbuzz {hb.version_string()}\n")
    w.write(f"# uharfbuzz {hb.__version__}\n")
    for line in out:
        w.write(line + "\n")
