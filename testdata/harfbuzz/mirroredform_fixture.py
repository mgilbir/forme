# Builds the face mirroredform.py reads that no foundry made, into the
# directory it is given:
#
#   make hbmirroredform
#
# shape/mirroredform.go answers what a font's 'rtlm' feature makes of one
# glyph, which is how MathML Core draws a right-to-left operator or radical
# (§5.3.2). The math fonts in the corpora state 'rtlm' one way each — Noto
# Sans Math as one single substitution under every script, radical-rtlm.woff
# as one for the square root under 'latn' alone, STIX Two Math not at all.
# This face states it every way the answer depends on:
#
#   - a single substitution in each of its two formats (a constant delta for
#     the square root and the summation, a list for the parenthesis and the
#     integral), so that neither is read as the other;
#   - a second lookup that substitutes the first one's output again, which
#     HarfBuzz applies in turn: the summation's form is the second glyph;
#   - a lookup that takes a glyph apart into two, which is not a mirrored
#     form of it;
#   - a form stated under 'grek' only, which a symbol, read under 'DFLT',
#     does not see, and one under 'latn' only, which a Latin letter does;
#   - a parenthesis with a form of its own though Unicode gives it a mirror
#     character, which MathML Core asks for before the mirror.
#
# Its glyphs are rectangles, and it is built with fontTools and its
# timestamps fixed, so that building it again produces the same bytes and the
# checksum the expectations record stays true.
import os
import sys

from oracle import fonttools

fonttools()
from fontTools.feaLib.builder import addOpenTypeFeaturesFromString  # noqa: E402
from fontTools.fontBuilder import FontBuilder  # noqa: E402
from fontTools.pens.ttGlyphPen import TTGlyphPen  # noqa: E402
from fontTools.ttLib import TTFont  # noqa: E402

out_dir = sys.argv[1]

# name: (character or None, advance, (xMin, yMin, xMax, yMax) or None)
GLYPHS = [
    (".notdef", None, 500, None),
    ("space", 0x20, 250, None),
    ("A", 0x41, 600, (20, 0, 580, 700)),
    ("A.rtlm", None, 610, (30, 0, 590, 700)),
    ("parenleft", 0x28, 333, (60, -250, 300, 750)),
    ("parenright", 0x29, 333, (33, -250, 273, 750)),
    ("parenleft.rtlm", None, 340, (40, -250, 280, 750)),
    ("bar", 0x7C, 200, (80, -250, 120, 750)),
    ("bar.left", None, 100, (40, -250, 60, 750)),
    ("bar.right", None, 100, (40, -250, 60, 750)),
    ("alpha", 0x3B1, 500, (30, 0, 470, 450)),
    ("alpha.rtlm", None, 500, (40, 0, 460, 450)),
    ("arrowright", 0x2192, 1000, (0, 200, 1000, 400)),
    ("arrowright.rtlm", None, 1000, (0, 210, 1000, 390)),
    ("integral", 0x222B, 500, (-50, -300, 600, 900)),
    ("integral.rtlm", None, 520, (-100, -300, 550, 900)),
    ("radical", 0x221A, 600, (20, -100, 580, 900)),
    ("radical.rtlm", None, 620, (40, -100, 600, 900)),
    ("sum", 0x2211, 900, (50, -250, 850, 750)),
    ("sum.rtlm", None, 910, (60, -250, 860, 750)),
    ("sum.rtlm2", None, 920, (70, -250, 870, 750)),
]


def rect(box):
    pen = TTGlyphPen(None)
    if box is not None:
        x0, y0, x1, y1 = box
        pen.moveTo((x0, y0))
        pen.lineTo((x0, y1))
        pen.lineTo((x1, y1))
        pen.lineTo((x1, y0))
        pen.closePath()
    return pen.glyph()


order = [g[0] for g in GLYPHS]
fb = FontBuilder(1000, isTTF=True)
fb.setupGlyphOrder(order)
fb.setupCharacterMap({c: n for n, c, _, _ in GLYPHS if c is not None})
fb.setupGlyf({n: rect(box) for n, _, _, box in GLYPHS})
fb.setupHorizontalMetrics({n: (adv, box[0] if box else 0) for n, _, adv, box in GLYPHS})
fb.setupHorizontalHeader(ascent=1000, descent=-300)
fb.setupNameTable({"familyName": "MirroredForms", "styleName": "Regular"})
fb.setupOS2(sTypoAscender=1000, sTypoDescender=-300, usWinAscent=1000, usWinDescent=300)
fb.setupPost()

# radical and sum are each one glyph before their forms, so feaLib states
# their lookup as format 1, a delta; the parenthesis and the integral are
# not, so theirs is format 2, a list. The two are asserted below rather than
# assumed.
FEATURES = """
languagesystem DFLT dflt;
languagesystem latn dflt;
languagesystem grek dflt;

lookup Delta {
    sub radical by radical.rtlm;
} Delta;
lookup DeltaSum {
    sub sum by sum.rtlm;
} DeltaSum;
lookup List {
    sub parenleft by parenleft.rtlm;
    sub integral by integral.rtlm;
} List;
lookup Again {
    sub sum.rtlm by sum.rtlm2;
} Again;
lookup Apart {
    sub bar by bar.left bar.right;
} Apart;
lookup GreekOnly {
    sub arrowright by arrowright.rtlm;
    sub alpha by alpha.rtlm;
} GreekOnly;
lookup LatinOnly {
    sub A by A.rtlm;
} LatinOnly;

feature rtlm {
    script DFLT;
    language dflt;
    lookup Delta;
    lookup DeltaSum;
    lookup List;
    lookup Again;
    lookup Apart;

    script latn;
    language dflt;
    lookup Delta;
    lookup DeltaSum;
    lookup List;
    lookup Again;
    lookup Apart;
    lookup LatinOnly;

    script grek;
    language dflt;
    lookup GreekOnly;
} rtlm;
"""
addOpenTypeFeaturesFromString(fb.font, FEATURES)

fb.font["head"].created = fb.font["head"].modified = 0
path = os.path.join(out_dir, "MirroredForms.ttf")
fb.save(path)

# The formats as they are written: fontTools decides one when it compiles a
# single substitution, and forgets it again when it reads one back, so the
# file's are asked for as it would write them.
font = TTFont(path)
formats = set()
for lookup in font["GSUB"].table.LookupList.Lookup:
    if lookup.LookupType == 1:
        for sub in lookup.SubTable:
            sub.preWrite(font)
            formats.add(sub.Format)
if formats != {1, 2}:
    os.remove(path)
    sys.exit("the single substitutions were compiled as formats %s, and the face is "
             "meant to state both" % sorted(formats))
print("wrote", path)
