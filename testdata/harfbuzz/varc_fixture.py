# Builds the variable-composite face varc.py measures and draws, which no
# foundry made, into the directory it is given:
#
#   make hbvarc
#
# VarComposite.ttf has a weight axis and two hidden axes of its own, the
# kind a VARC font draws its components along, and a VARC table
# (OpenType 1.9.1) that says each thing HarfBuzz reads from one:
#
#   plain        a triangle turned an eighth and moved, whose box turned is not
#                the box of its points turned
#   stroke       a bar drawn at a coordinate of the hidden axis 0000, which
#                gvar widens it along; and the triangle rotated, scaled and
#                turned about a centre
#   varied       the bar at an axis value that varies with the weight, moved
#                and widened by a transform that varies with it too
#   skewed       the bar skewed both ways and scaled differently along each
#   conditional  one component for each kind of condition — an axis range,
#                its negation, a conjunction with a value varying with the
#                weight, and a disjunction — and a glyf composite leaf
#   nested       plain and stroke, composed again
#   itself       a component naming the glyph it is in, which draws that
#                glyph's own glyf outline
#   loop, loop2  each a component of the other, which HarfBuzz's decycler
#                cuts
#   reset        stroke with the axes it does not set reset, and the other
#                hidden axis set
#   resetter     reset at the lightest weight: which the reset puts back, so
#                that stroke's triangle is drawn at the face's weight
#   flat         a line with no area, which adds nothing to a box, and the
#                triangle
#   hidden       one component, whose condition never holds: nothing drawn
#   painted      not a VARC glyph but a COLR one, filled inside stroke, skewed
#                and plain: HarfBuzz draws the glyph a fill is clipped to through
#                VARC, and measures the colour glyph by the box of what it
#                draws, not by its ink
#   loosely      twice a glyph whose header states a box larger than its points:
#                once with no coordinates, where HarfBuzz measures a leaf by its
#                header, and once at coordinates that are all zero, where it
#                measures the points
#
# Every leaf is straight lines, so that the path HarfBuzz draws is the
# glyph's points in order.
#
# VarCompositeDeep64.ttf and VarCompositeDeep65.ttf draw plain only where a
# condition nested 64 and 65 deep holds, and HarfBuzz's sanitizer takes the
# first and refuses the second, table and all; VarCompositeBroken.ttf's
# coverage lies outside its table, which it refuses too. VarCompositeCFF.otf
# is CFFInk.otf (cffink_fixture.py) with a VARC table whose leaves are CFF
# glyphs, which must be in the directory first.
#
# Each is built with fontTools and its timestamps fixed, so that building it
# again produces the same bytes and the checksums the expectations record
# stay true.
import io
import os
import sys

from oracle import fonttools

fonttools()
from fontTools.colorLib.builder import buildCOLR, buildCPAL  # noqa: E402
from fontTools.fontBuilder import FontBuilder  # noqa: E402
from fontTools.misc import xmlReader  # noqa: E402
from fontTools.pens.ttGlyphPen import TTGlyphPen  # noqa: E402
from fontTools.ttLib import TTFont  # noqa: E402
from fontTools.ttLib.tables import otTables as ot  # noqa: E402
from fontTools.ttLib.tables.DefaultTable import DefaultTable  # noqa: E402
from fontTools.ttLib.tables.TupleVariation import TupleVariation  # noqa: E402
from fontTools.ttLib.tables._g_l_y_f import Glyph, GlyphComponent  # noqa: E402


def lines(*pts):
    pen = TTGlyphPen(None)
    pen.moveTo(pts[0])
    for p in pts[1:]:
        pen.lineTo(p)
    pen.closePath()
    return pen.glyph()


VARC_GLYPHS = ["plain", "stroke", "varied", "skewed", "conditional", "nested", "itself",
               "loop", "loop2", "reset", "flat", "hidden", "loosely", "resetter"]
ORDER = [".notdef", "space", "bar", "tri", "hline", "comp", "loose", "painted"] + VARC_GLYPHS

# The VARC table, as fontTools reads it from TTX. Axis 0 is wght, 1 and 2 the
# hidden axes 0000 and 0001. The store's first data set varies with the
# weight — region 0 the heavier half, region 1 the lighter — and its second
# with axis 0000. An item's values are each region's in turn.
VARC = """
<VARC>
  <Version value="0x00010000"/>
  <Coverage>
    %(coverage)s
  </Coverage>
  <MultiVarStore Format="1">
    <Format value="1"/>
    <SparseVarRegionList>
      <Region index="0">
        <SparseVarRegionAxis index="0">
          <AxisIndex value="0"/><StartCoord value="0.0"/><PeakCoord value="1.0"/><EndCoord value="1.0"/>
        </SparseVarRegionAxis>
      </Region>
      <Region index="1">
        <SparseVarRegionAxis index="0">
          <AxisIndex value="0"/><StartCoord value="-1.0"/><PeakCoord value="-1.0"/><EndCoord value="0.0"/>
        </SparseVarRegionAxis>
      </Region>
      <Region index="2">
        <SparseVarRegionAxis index="0">
          <AxisIndex value="1"/><StartCoord value="0.0"/><PeakCoord value="1.0"/><EndCoord value="1.0"/>
        </SparseVarRegionAxis>
      </Region>
    </SparseVarRegionList>
    <MultiVarData index="0" Format="1">
      <Format value="1"/>
      <VarRegionIndex index="0" value="0"/>
      <VarRegionIndex index="1" value="1"/>
      <Item index="0" value="[4096, -8192]"/>
      <Item index="1" value="[40, 256, -30, -128]"/>
      <Item index="2" value="[100, -100]"/>
    </MultiVarData>
    <MultiVarData index="1" Format="1">
      <Format value="1"/>
      <VarRegionIndex index="0" value="2"/>
      <Item index="0" value="[60]"/>
    </MultiVarData>
  </MultiVarStore>
  <ConditionList>
    <ConditionTable index="0" Format="1">
      <AxisIndex value="0"/><FilterRangeMinValue value="0.5"/><FilterRangeMaxValue value="1.0"/>
    </ConditionTable>
    <ConditionTable index="1" Format="5">
      <ConditionTable Format="1">
        <AxisIndex value="0"/><FilterRangeMinValue value="0.5"/><FilterRangeMaxValue value="1.0"/>
      </ConditionTable>
    </ConditionTable>
    <ConditionTable index="2" Format="3">
      <ConditionTable index="0" Format="1">
        <AxisIndex value="1"/><FilterRangeMinValue value="-1.0"/><FilterRangeMaxValue value="0.0"/>
      </ConditionTable>
      <ConditionTable index="1" Format="2">
        <DefaultValue value="-50"/><VarIdx value="2"/>
      </ConditionTable>
    </ConditionTable>
    <ConditionTable index="3" Format="4">
      <ConditionTable index="0" Format="1">
        <AxisIndex value="0"/><FilterRangeMinValue value="-1.0"/><FilterRangeMaxValue value="-0.5"/>
      </ConditionTable>
      <ConditionTable index="1" Format="1">
        <AxisIndex value="0"/><FilterRangeMinValue value="0.9"/><FilterRangeMaxValue value="1.0"/>
      </ConditionTable>
    </ConditionTable>
    <ConditionTable index="4" Format="1">
      <AxisIndex value="2"/><FilterRangeMinValue value="0.9"/><FilterRangeMaxValue value="1.0"/>
    </ConditionTable>
  </ConditionList>
  <AxisIndicesList>
    <Item index="0" value="[1]"/>
    <Item index="1" value="[2]"/>
    <Item index="2" value="[0]"/>
  </AxisIndicesList>
  <VarCompositeGlyphs>
    %(glyphs)s
  </VarCompositeGlyphs>
</VARC>
"""

# Each VARC glyph's components, in TTX. Transform values are in font units
# and degrees, as fontTools writes them.
COMPONENTS = {
    "plain": ["""<glyphName value="tri"/><rotation value="45.0"/><translateX value="100"/>
                 <translateY value="50"/>"""],
    "stroke": [
        """<glyphName value="bar"/><axisIndicesIndex value="0"/><axisValues value="[0.5]"/>""",
        """<glyphName value="tri"/><rotation value="30.0"/><scaleX value="0.75"/>
           <tCenterX value="200"/><tCenterY value="100"/><translateX value="300"/>""",
    ],
    "varied": ["""<glyphName value="bar"/><axisIndicesIndex value="0"/><axisValues value="[0.25]"/>
                  <axisValuesVarIndex value="0"/><transformVarIndex value="1"/>
                  <translateX value="20"/><scaleX value="1.0"/>"""],
    "skewed": ["""<glyphName value="bar"/><skewX value="10.0"/><skewY value="-5.0"/>
                  <scaleX value="1.25"/><scaleY value="0.75"/>"""],
    "conditional": [
        """<glyphName value="tri"/><conditionIndex value="0"/>""",
        """<glyphName value="bar"/><conditionIndex value="1"/><translateY value="-300"/>""",
        """<glyphName value="comp"/><conditionIndex value="2"/><translateX value="500"/>""",
        """<glyphName value="tri"/><conditionIndex value="3"/><translateY value="600"/>""",
    ],
    "nested": [
        """<glyphName value="plain"/><translateX value="-50"/>""",
        """<glyphName value="stroke"/><scaleX value="0.5"/><translateY value="200"/>""",
    ],
    "itself": [
        """<glyphName value="itself"/>""",
        """<glyphName value="tri"/><translateX value="400"/>""",
    ],
    "loop": ["""<glyphName value="loop2"/>""", """<glyphName value="bar"/>"""],
    "loop2": ["""<glyphName value="loop"/>""", """<glyphName value="tri"/><translateY value="100"/>"""],
    "reset": ["""<glyphName value="stroke"/><axisIndicesIndex value="1"/>
                 <axisValues value="[0.75]" resetUnspecifiedAxes="1"/><transformVarIndex value="65536"/>
                 <translateY value="10"/>"""],
    "flat": ["""<glyphName value="hline"/>""", """<glyphName value="tri"/><translateX value="-40"/>"""],
    "hidden": ["""<glyphName value="tri"/><conditionIndex value="4"/>"""],
    "resetter": ["""<glyphName value="reset"/><axisIndicesIndex value="2"/><axisValues value="[-1.0]"/>"""],
    "loosely": [
        """<glyphName value="loose"/><translateX value="10"/>""",
        """<glyphName value="loose"/><axisIndicesIndex value="0"/><axisValues value="[0.0]"/>
           <translateY value="400"/>""",
    ],
}


def chain(n):
    """A condition n deep: n-1 negations of an axis range that always holds."""
    inner = ('<ConditionTable Format="1"><AxisIndex value="0"/><FilterRangeMinValue value="-1.0"/>'
             '<FilterRangeMaxValue value="1.0"/></ConditionTable>')
    for _ in range(n - 1):
        inner = '<ConditionTable Format="5">%s</ConditionTable>' % inner
    return inner.replace("<ConditionTable ", '<ConditionTable index="5" ', 1)


def varc_xml(deep=None):
    """The table; with deep, 'plain' drawn only where a condition that deep
    holds."""
    components = dict(COMPONENTS)
    xml = VARC
    if deep:
        components["plain"] = [COMPONENTS["plain"][0] + '<conditionIndex value="5"/>']
        xml = xml.replace("  </ConditionList>", chain(deep) + "\n  </ConditionList>")
    coverage = "\n".join('<Glyph value="%s"/>' % g for g in VARC_GLYPHS)
    glyphs = []
    for i, name in enumerate(VARC_GLYPHS):
        comps = "".join('<VarComponent index="%d">%s</VarComponent>' % (k, c)
                        for k, c in enumerate(components[name]))
        glyphs.append('<VarCompositeGlyph index="%d">%s</VarCompositeGlyph>' % (i, comps))
    return xml % {"coverage": coverage, "glyphs": "\n".join(glyphs)}


def build(path, deep=None, broken=False, static=False):
    fb = FontBuilder(1000, isTTF=True)
    fb.setupGlyphOrder(ORDER)
    fb.setupCharacterMap({0x20: "space", 0x41: "bar", 0x42: "tri"} |
                         {0x61 + i: g for i, g in enumerate(VARC_GLYPHS)})
    comp = Glyph()
    comp.numberOfContours = -1
    a, b = GlyphComponent(), GlyphComponent()
    a.glyphName, a.x, a.y, a.flags = "bar", 0, 0, 0
    b.glyphName, b.x, b.y, b.flags = "tri", 150, -100, 0
    b.transform = [[0.5, 0], [0, 0.5]]
    comp.components = [a, b]
    glyf = {
        ".notdef": lines((50, 0), (50, 700), (450, 700), (450, 0)),
        "space": Glyph(),
        "bar": lines((0, 0), (0, 600), (100, 600), (100, 0)),
        "tri": lines((0, 0), (200, 400), (400, 0)),
        "hline": lines((0, 300), (500, 300)),
        "comp": comp,
        "loose": lines((100, 0), (100, 300), (300, 300), (300, 0)),
        "painted": lines((0, 0), (0, 50), (50, 50), (50, 0)),
    }
    # A VARC glyph's own glyf entry is what a reader without VARC draws: an
    # empty one for most, a square for 'itself', which draws it.
    for g in VARC_GLYPHS:
        glyf[g] = Glyph()
    glyf["itself"] = lines((10, 10), (10, 90), (90, 90), (90, 10))
    fb.setupGlyf(glyf)
    # Every box measured but loose's, which states more than its points.
    loose = fb.font["glyf"]["loose"]
    loose.xMin, loose.yMin, loose.xMax, loose.yMax = 50, -50, 350, 350
    fb.font.recalcBBoxes = False
    head, table = fb.font["head"], fb.font["glyf"]
    boxes = [table[g] for g in ORDER if table[g].numberOfContours]
    head.xMin, head.yMin = min(g.xMin for g in boxes), min(g.yMin for g in boxes)
    head.xMax, head.yMax = max(g.xMax for g in boxes), max(g.yMax for g in boxes)
    fb.setupHorizontalMetrics({g: (600, 0) for g in ORDER})
    fb.setupHorizontalHeader(ascent=900, descent=-300)
    fb.setupOS2(sTypoAscender=900, sTypoDescender=-300, usWinAscent=900, usWinDescent=300)
    fb.setupNameTable({"familyName": "VarComposite", "styleName": "Regular"})
    fb.setupPost()
    fb.setupFvar([("wght", 100, 400, 900, "Weight"), ("0000", -1, 0, 1, "Width of bars"),
                  ("0001", -1, 0, 1, "Hidden")], [])
    for axis in fb.font["fvar"].axes[1:]:
        axis.flags = 0x0001  # HIDDEN_AXIS
    heavy, light = {"wght": (0.0, 1.0, 1.0)}, {"wght": (-1.0, -1.0, 0.0)}
    wide = {"0000": (0.0, 1.0, 1.0)}
    fb.setupGvar({
        # The bar widens along 0000, and its right phantom point with it.
        "bar": [TupleVariation(wide, [(0, 0), (0, 0), (150, 0), (150, 0), (0, 0), (150, 0), (0, 0), (0, 0)])],
        # The triangle's apex rises with the weight, and falls below it.
        "tri": [TupleVariation(heavy, [(0, 0), (30, 120), (0, 0), (0, 0), (0, 0), (0, 0), (0, 0)]),
                TupleVariation(light, [(10, 0), (0, -80), (-10, 0), (0, 0), (0, 0), (0, 0), (0, 0)])],
        # The composite's second component moves with the weight.
        "comp": [TupleVariation(heavy, [(0, 0), (40, 25), (0, 0), (0, 0), (0, 0), (0, 0)])],
    })
    reader = xmlReader.XMLReader(io.BytesIO(("<ttFont>" + varc_xml(deep) + "</ttFont>").encode()), fb.font)
    reader.read()
    solid = {"Format": ot.PaintFormat.PaintSolid, "PaletteIndex": 0, "Alpha": 1.0}
    fb.font["COLR"] = buildCOLR({"painted": {"Format": ot.PaintFormat.PaintColrLayers, "Layers": [
        {"Format": ot.PaintFormat.PaintGlyph, "Glyph": "stroke", "Paint": solid},
        {"Format": ot.PaintFormat.PaintGlyph, "Glyph": "skewed", "Paint": solid},
        {"Format": ot.PaintFormat.PaintGlyph, "Glyph": "plain", "Paint": solid},
    ]}}, glyphMap=fb.font.getReverseGlyphMap())
    fb.font["CPAL"] = buildCPAL([[(1.0, 0.0, 0.0, 1.0)]])
    if broken:
        # The coverage's offset past the end of the table, which HarfBuzz's
        # sanitizer refuses, and with it the whole table.
        data = bytearray(fb.font["VARC"].compile(fb.font))
        data[4:8] = (len(data) + 1).to_bytes(4, "big")
        raw = DefaultTable("VARC")
        raw.data = bytes(data)
        fb.font["VARC"] = raw
    # maxp's counts, which a save with the boxes left alone does not count.
    fb.font["maxp"].recalc(fb.font)
    fb.font["head"].created = fb.font["head"].modified = 3660681600
    fb.font.recalcTimestamp = False
    if not static:
        fb.save(path)
        return
    # No fvar, and gvar as it was, read at its own count of axes: a face with
    # no design space, whose components set coordinates of their own.
    b = io.BytesIO()
    fb.save(b)
    font = TTFont(io.BytesIO(b.getvalue()))
    gvar = DefaultTable("gvar")
    gvar.data = font.getTableData("gvar")
    del font["fvar"]
    font["gvar"] = gvar
    font.recalcTimestamp = False
    font.save(path)


build(os.path.join(sys.argv[1], "VarComposite.ttf"))
# Conditions nest as deep as HarfBuzz's sanitizer takes, 64, and one deeper,
# which it refuses along with the table; and a table it refuses for a part
# outside it.
build(os.path.join(sys.argv[1], "VarCompositeDeep64.ttf"), deep=64)
build(os.path.join(sys.argv[1], "VarCompositeDeep65.ttf"), deep=65)
build(os.path.join(sys.argv[1], "VarCompositeBroken.ttf"), broken=True)
# And the face with no design space, where HarfBuzz hands VARC no coordinates
# and measures a leaf that gets none by its glyph header.
build(os.path.join(sys.argv[1], "VarCompositeStatic.ttf"), static=True)


def cff(directory):
    """VarCompositeCFF.otf: CFFInk.otf with a VARC table whose leaves are CFF
    glyphs, which HarfBuzz measures by their charstrings' boxes turned."""
    font = TTFont(os.path.join(directory, "CFFInk.otf"))
    xml = """
<VARC>
  <Version value="0x00010000"/>
  <Coverage><Glyph value="rlineto"/><Glyph value="hlineto.even"/></Coverage>
  <VarCompositeGlyphs>
    <VarCompositeGlyph index="0">
      <VarComponent index="0"><glyphName value="rrcurveto"/><rotation value="20.0"/>
        <translateX value="30"/></VarComponent>
      <VarComponent index="1"><glyphName value="rcurveline"/><scaleX value="0.5"/>
        <scaleY value="1.5"/><translateY value="-40"/></VarComponent>
    </VarCompositeGlyph>
    <VarCompositeGlyph index="1">
      <VarComponent index="0"><glyphName value="hlineto.even"/><skewX value="15.0"/></VarComponent>
      <VarComponent index="1"><glyphName value="rlineto"/><translateX value="200"/></VarComponent>
    </VarCompositeGlyph>
  </VarCompositeGlyphs>
</VARC>
"""
    xmlReader.XMLReader(io.BytesIO(("<ttFont>" + xml + "</ttFont>").encode()), font).read()
    font["head"].created = font["head"].modified = 3660681600
    font.recalcTimestamp = False
    font.save(os.path.join(directory, "VarCompositeCFF.otf"))


cff(sys.argv[1])
