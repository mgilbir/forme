# Builds the math face mathtable.py reads that no foundry made, into the directory
# it is given:
#
#   make hbmath
#
# shape/math.go reads a font's MATH table, and the math fonts in the corpora —
# the suite's own test fonts, Noto Sans Math and STIX Two Math — each state it
# the one way their compiler writes it. This face states every part of it,
# including the parts a reader is most likely to get subtly wrong, so that no
# rule is held to HarfBuzz only by this package's own reading of the
# specification:
#
#   - all fifty-six constants, each a different value, so that one read from
#     the wrong slot cannot pass for another; negative values where the field
#     is signed and the two unsigned heights past 32767;
#   - a constant and an italics correction with a hinting device table, and a
#     top accent attachment with a variation device table, which must not
#     change the value read at no particular size and the default instance;
#   - italics corrections and top accent attachments on glyphs out of order
#     in the glyph list, so that a coverage index cannot pass for a glyph;
#   - extended shapes;
#   - kerning at all four corners, with no heights, one height, several, and
#     negative ones, asked at every height, one either side of each, and far
#     beyond both ends;
#   - vertical and horizontal size variants, and assemblies: a three-part one
#     with an extender between two ends, one of extenders only, one with no
#     extender, one whose connectors are shorter than the minimum overlap, a
#     glyph with variants and no assembly, and one with an assembly and no
#     variants.
#
# Its glyphs are rectangles of chosen sizes, so that the metrics an assembly
# is built from are known exactly.
#
# It is built with fontTools and its timestamps fixed, so that building it
# again produces the same bytes and the checksum the expectations record stays
# true.
import os
import sys

from oracle import fonttools

fonttools()
from fontTools.fontBuilder import FontBuilder  # noqa: E402
from fontTools.otlLib.builder import buildMathTable  # noqa: E402
from fontTools.pens.ttGlyphPen import TTGlyphPen  # noqa: E402
from fontTools.ttLib.tables import otTables  # noqa: E402

out_dir = sys.argv[1]

# name: (character or None, advance, (xMin, yMin, xMax, yMax) or None)
GLYPHS = [
    (".notdef", None, 500, None),
    ("space", 0x20, 250, None),
    ("A", 0x41, 600, (20, 0, 580, 700)),
    ("f", 0x66, 300, (10, -200, 400, 750)),
    ("x", 0x78, 500, (20, 0, 480, 450)),
    ("tilde", 0x2DC, 400, (30, 560, 370, 660)),
    ("parenleft", 0x28, 333, (60, -250, 300, 750)),
    ("paren.v1", None, 400, (60, -500, 360, 1000)),
    ("paren.v2", None, 480, (60, -800, 420, 1300)),
    ("paren.bot", None, 500, (60, 0, 450, 800)),
    ("paren.ext", None, 350, (60, 0, 200, 600)),
    ("paren.top", None, 500, (60, 0, 450, 800)),
    ("radical", 0x221A, 600, (20, -100, 580, 900)),
    ("radical.v1", None, 620, (20, -200, 600, 1300)),
    ("radical.bot", None, 640, (20, 0, 620, 700)),
    ("radical.ext", None, 300, (200, 0, 260, 500)),
    ("radical.top", None, 300, (200, 0, 260, 400)),
    ("arrowright", 0x2192, 1000, (0, 200, 1000, 400)),
    ("arrow.h1", None, 1500, (0, 180, 1500, 420)),
    ("arrow.left", None, 400, (0, 250, 400, 350)),
    ("arrow.ext", None, 600, (0, 250, 600, 350)),
    ("arrow.right", None, 500, (0, 150, 500, 450)),
    ("overline", 0x305, 500, (0, 700, 500, 750)),
    ("overline.ext", None, 500, (0, 700, 500, 750)),
    ("integral", 0x222B, 500, (-50, -300, 600, 900)),
    ("integral.display", None, 700, (-80, -600, 900, 1400)),
    ("brace", 0x7B, 500, (50, -250, 450, 750)),
    ("brace.part", None, 500, (50, 0, 450, 500)),
    ("bar", 0x7C, 200, (80, -250, 120, 750)),
    ("bar.part", None, 200, (80, 0, 120, 400)),
    ("sum", 0x2211, 900, (50, -250, 850, 750)),
    ("sum.display", None, 1300, (50, -450, 1250, 1050)),
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
fb.setupNameTable({"familyName": "MathTable", "styleName": "Regular"})
fb.setupOS2(sTypoAscender=1000, sTypoDescender=-300, usWinAscent=1000, usWinDescent=300,
            sxHeight=450, sCapHeight=700, ySubscriptYOffset=150, ySuperscriptYOffset=350)
fb.setupPost()

# Every constant a different value, in order, so that a slot read for another
# answers wrongly. The four that are not MathValueRecords come first and last.
NAMES = [
    "ScriptPercentScaleDown", "ScriptScriptPercentScaleDown",
    "DelimitedSubFormulaMinHeight", "DisplayOperatorMinHeight",
    "MathLeading", "AxisHeight", "AccentBaseHeight", "FlattenedAccentBaseHeight",
    "SubscriptShiftDown", "SubscriptTopMax", "SubscriptBaselineDropMin",
    "SuperscriptShiftUp", "SuperscriptShiftUpCramped", "SuperscriptBottomMin",
    "SuperscriptBaselineDropMax", "SubSuperscriptGapMin",
    "SuperscriptBottomMaxWithSubscript", "SpaceAfterScript",
    "UpperLimitGapMin", "UpperLimitBaselineRiseMin", "LowerLimitGapMin",
    "LowerLimitBaselineDropMin", "StackTopShiftUp", "StackTopDisplayStyleShiftUp",
    "StackBottomShiftDown", "StackBottomDisplayStyleShiftDown", "StackGapMin",
    "StackDisplayStyleGapMin", "StretchStackTopShiftUp",
    "StretchStackBottomShiftDown", "StretchStackGapAboveMin",
    "StretchStackGapBelowMin", "FractionNumeratorShiftUp",
    "FractionNumeratorDisplayStyleShiftUp", "FractionDenominatorShiftDown",
    "FractionDenominatorDisplayStyleShiftDown", "FractionNumeratorGapMin",
    "FractionNumDisplayStyleGapMin", "FractionRuleThickness",
    "FractionDenominatorGapMin", "FractionDenomDisplayStyleGapMin",
    "SkewedFractionHorizontalGap", "SkewedFractionVerticalGap",
    "OverbarVerticalGap", "OverbarRuleThickness", "OverbarExtraAscender",
    "UnderbarVerticalGap", "UnderbarRuleThickness", "UnderbarExtraDescender",
    "RadicalVerticalGap", "RadicalDisplayStyleVerticalGap",
    "RadicalRuleThickness", "RadicalExtraAscender", "RadicalKernBeforeDegree",
    "RadicalKernAfterDegree", "RadicalDegreeBottomRaisePercent",
]
constants = {n: 100 + 7 * i for i, n in enumerate(NAMES)}
constants["ScriptPercentScaleDown"] = 73
constants["ScriptScriptPercentScaleDown"] = -1  # signed, and nonsense
constants["DelimitedSubFormulaMinHeight"] = 40000  # unsigned, past 32767
constants["DisplayOperatorMinHeight"] = 65000
constants["RadicalKernAfterDegree"] = -555
constants["SubscriptBaselineDropMin"] = -120
constants["RadicalDegreeBottomRaisePercent"] = 61

buildMathTable(
    fb.font,
    constants=constants,
    # Out of glyph order, and the top accents on a different set.
    italicsCorrections={"integral": 120, "f": 90, "A": -15, "sum.display": 40},
    topAccentAttachments={"x": 250, "tilde": 200, "A": 330, "integral.display": 510},
    extendedShapes={"paren.v2", "radical.top", "integral.display"},
    mathKerns={
        "f": {
            "TopRight": ([100, 300, 500], [10, 20, 30, 40]),
            "BottomLeft": ([-200], [-5, -15]),
        },
        "A": {
            "TopLeft": ([], [7]),
            "BottomRight": ([-300, -100, 250], [1, 2, 3, 4]),
        },
    },
    minConnectorOverlap=50,
    vertGlyphVariants={
        "parenleft": [("parenleft", 1000), ("paren.v1", 1500), ("paren.v2", 2100)],
        "radical": [("radical", 1000), ("radical.v1", 1500)],
        "integral": [("integral", 1200), ("integral.display", 2000)],
        "sum": [("sum", 1000), ("sum.display", 1500)],
    },
    horizGlyphVariants={
        "arrowright": [("arrowright", 1000), ("arrow.h1", 1500)],
    },
    vertGlyphAssembly={
        # Bottom, extender, top.
        "parenleft": [
            (
                ("paren.bot", 0, 0, 200, 800),
                ("paren.ext", 1, 300, 300, 600),
                ("paren.top", 0, 200, 0, 800),
            ),
            25,
        ],
        # Two different extenders with an end between them.
        "radical": [
            (
                ("radical.bot", 0, 0, 100, 700),
                ("radical.ext", 1, 100, 100, 500),
                ("radical.top", 0, 100, 100, 400),
                ("radical.ext", 1, 100, 100, 500),
            ),
            0,
        ],
        # No extender: not an assembly MathML Core will build.
        "brace": [
            (("brace.part", 0, 0, 100, 500), ("brace.part", 0, 100, 0, 500)),
            0,
        ],
        # A connector shorter than the minimum overlap.
        "bar": [
            (("bar.part", 0, 0, 30, 400), ("bar.part", 1, 30, 30, 400)),
            0,
        ],
    },
    horizGlyphAssembly={
        "arrowright": [
            (
                ("arrow.left", 0, 0, 100, 400),
                ("arrow.ext", 1, 150, 150, 600),
                ("arrow.right", 0, 100, 0, 500),
            ),
            -10,
        ],
        # Extenders only.
        "overline": [(("overline.ext", 1, 200, 200, 500),), 0],
    },
)

math = fb.font["MATH"].table


def hinting():
    d = otTables.Device()
    d.StartSize, d.EndSize, d.DeltaFormat = 10, 12, 2
    d.DeltaValue = [1, -1, 2]
    return d


def variation():
    d = otTables.Device()
    d.StartSize, d.EndSize, d.DeltaFormat = 0, 0, 0x8000
    return d


math.MathConstants.AxisHeight.DeviceTable = hinting()
italics = math.MathGlyphInfo.MathItalicsCorrectionInfo
italics.ItalicsCorrection[italics.Coverage.glyphs.index("integral")].DeviceTable = hinting()
accents = math.MathGlyphInfo.MathTopAccentAttachment
accents.TopAccentAttachment[accents.TopAccentCoverage.glyphs.index("x")].DeviceTable = variation()

fb.font["head"].created = fb.font["head"].modified = 3660681600
os.makedirs(out_dir, exist_ok=True)
fb.save(os.path.join(out_dir, "MathTable.ttf"))
