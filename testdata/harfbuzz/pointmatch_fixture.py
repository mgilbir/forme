# Builds the variable faces pointmatch.py instances that no foundry made, into
# the directory it is given:
#
#   make hbpointmatch
#
# A composite glyph may place a component by matching points rather than at an
# offset: ARGS_ARE_XY_VALUES clear, its two arguments are a point already
# gathered for the composite and a point of the component, and the component is
# moved so that the second lands on the first. In a variable font both points
# move with gvar, so where the component goes is decided at the location, from
# the points as they are there. gvar's own delta for such a component — the
# format gives every component one — moves nothing, since the match is made
# after it.
#
# No face in the corpora places a component this way and varies, so these do.
# Every outline is straight lines, so that the path HarfBuzz draws is the
# glyph's points in order and the two can be compared point for point.
#
# # PointMatch.ttf
#
#   bowl, mark   simple glyphs whose points gvar moves, each differently, at
#                both ends of the weight axis
#   joined       bowl at an offset, and mark placed by matching its point 1 to
#                bowl's point 2; gvar states a delta for the matched component
#                too, which nothing honours
#   turned       bowl, and mark through a 2x2 with scaled offsets, matched after
#                the transform
#   nested       mark, and then joined inside it at an offset: joined's match
#                names point 2, which HarfBuzz counts from the start of the
#                outermost glyph — nested's mark — and fontTools from the start
#                of joined's own points
#   metrics      joined, taking its metrics from mark (USE_MY_METRICS), which is
#                the matched component
#   scaled       bowl halved, its offset halved with it (SCALED_COMPONENT_OFFSET),
#                and mark matched to one of its points
#
# # PointMatchPhantom.ttf
#
# The matches fontTools' instancer cannot make, since it counts no phantom
# points and indexes past the end of its list where a match does:
#
#   right        mark matched by its right phantom point, which moves with the
#                advance
#   bottom       mark matched by its bottom phantom point: the face has no
#                vertical metrics, so it is an em below the top of mark's box,
#                and gvar moves it
#   self         mark, through a 2x2 with scaled offsets, matched to one of
#                its own points, which HarfBuzz counts among those gathered
#                once the component's are; gvar's delta for it, which that
#                match does not cancel, goes through the 2x2
#   carriedright a composite of mark that takes mark's metrics, matched by the
#                right phantom point it takes from mark
#   outside      a match naming a point nobody has, which HarfBuzz leaves
#                where the transform put it
#
# Each is built with fontTools and its timestamps fixed, so that building it
# again produces the same bytes and the checksums the expectations record stay
# true.
import os
import sys

from oracle import fonttools

fonttools()
from fontTools.fontBuilder import FontBuilder  # noqa: E402
from fontTools.pens.ttGlyphPen import TTGlyphPen  # noqa: E402
from fontTools.ttLib.tables.TupleVariation import TupleVariation  # noqa: E402
from fontTools.ttLib.tables._g_l_y_f import Glyph, GlyphComponent  # noqa: E402

USE_MY_METRICS = 0x0200
SCALED_COMPONENT_OFFSET = 0x0800

MAX = {"wght": (0.0, 1.0, 1.0)}
TURN = [[0.5, 0.25], [-0.25, 0.75]]
MIN = {"wght": (-1.0, -1.0, 0.0)}


def lines(*pts):
    pen = TTGlyphPen(None)
    pen.moveTo(pts[0])
    for p in pts[1:]:
        pen.lineTo(p)
    pen.closePath()
    return pen.glyph()


def at(name, x, y, flags=0, transform=None):
    c = GlyphComponent()
    c.glyphName, c.x, c.y, c.flags = name, x, y, flags
    if transform is not None:
        c.transform = transform
    return c


def matched(name, first, second, flags=0, transform=None):
    c = GlyphComponent()
    c.glyphName, c.firstPt, c.secondPt, c.flags = name, first, second, flags
    if transform is not None:
        c.transform = transform
    return c


def composite(*components):
    g = Glyph()
    g.numberOfContours = -1
    g.components = list(components)
    return g


# The outline points, then the four phantom points: left, right, top, bottom.
BOWL = lines((100, 0), (500, -20), (520, 300), (480, 640), (110, 600), (60, 300))
BOWL_MAX = [(20, 0), (30, -10), (40, 5), (30, 20), (-10, 15), (-20, 0), (0, 0), (40, 0), (0, 0), (0, 0)]
BOWL_MIN = [(-10, 5), (-15, 10), (-25, -5), (-10, -10), (5, -5), (10, 0), (0, 0), (-30, 0), (0, 0), (0, 0)]
MARK = lines((0, 0), (0, 80), (60, 90), (70, 10))
MARK_MAX = [(5, 10), (5, 15), (-5, 15), (-5, 10), (0, 0), (10, 0), (0, 12), (0, -18)]
MARK_MIN = [(-3, -4), (-2, -8), (4, -6), (1, -2), (0, 0), (-6, 0), (0, -5), (0, 9)]


def variations(glyphs):
    """gvar: each glyph's deltas at the heaviest weight and at the lightest."""
    return {name: [TupleVariation(MAX, list(hi)), TupleVariation(MIN, list(lo))]
            for name, (hi, lo) in glyphs.items()}


def face(path, family, glyphs, deltas, metrics, boxes=None):
    """A face of the glyphs. boxes are the bounding boxes of the composites
    fontTools cannot measure, whose matches name phantom points it does not
    count; every other glyph's is measured."""
    boxes = boxes or {}
    order = [".notdef"] + list(glyphs)
    fb = FontBuilder(1000, isTTF=True)
    fb.setupGlyphOrder(order)
    fb.setupCharacterMap({0x41 + i: g for i, g in enumerate(order[1:])})
    glyf = {".notdef": lines((50, 0), (50, 700), (450, 700), (450, 0))}
    glyf.update(glyphs)
    fb.setupGlyf(glyf, calcGlyphBounds=False)
    table = fb.font["glyf"]
    for name in order:
        if name in boxes:
            g = table[name]
            g.xMin, g.yMin, g.xMax, g.yMax = boxes[name]
        else:
            table[name].recalcBounds(table)
    head = fb.font["head"]
    head.xMin = min(table[n].xMin for n in order)
    head.yMin = min(table[n].yMin for n in order)
    head.xMax = max(table[n].xMax for n in order)
    head.yMax = max(table[n].yMax for n in order)
    fb.font.recalcBBoxes = False
    hmtx = {".notdef": (500, 50)}
    hmtx.update(metrics)
    fb.setupHorizontalMetrics(hmtx)
    fb.setupHorizontalHeader(ascent=880, descent=-120)
    fb.setupOS2(sTypoAscender=880, sTypoDescender=-120, usWinAscent=880, usWinDescent=120)
    fb.setupNameTable({"familyName": family, "styleName": "Regular"})
    fb.setupPost()
    fb.setupFvar([("wght", 100, 400, 900, "Weight")], [])
    fb.setupGvar(variations(deltas))
    # maxp's counts, which a save with the boxes left alone does not count.
    fb.font["maxp"].recalc(fb.font)
    fb.font["head"].created = fb.font["head"].modified = 3660681600
    fb.font.recalcTimestamp = False
    fb.save(path)


def point_match(directory):
    glyphs = {
        "bowl": BOWL,
        "mark": MARK,
        "joined": composite(at("bowl", 10, 0), matched("mark", 2, 1)),
        "turned": composite(at("bowl", 0, 0), matched("mark", 4, 3, SCALED_COMPONENT_OFFSET, TURN)),
        "nested": composite(at("mark", -100, 20), at("joined", 30, 40)),
        "metrics": composite(at("bowl", 0, 0), matched("mark", 3, 0, USE_MY_METRICS)),
        "scaled": composite(at("bowl", 40, 20, SCALED_COMPONENT_OFFSET, [[0.5, 0], [0, 0.5]]),
                            matched("mark", 2, 1)),
    }
    deltas = {
        "bowl": (BOWL_MAX, BOWL_MIN),
        "mark": (MARK_MAX, MARK_MIN),
        # A composite's points are its components' offsets and then its phantom
        # points. The matched component's delta is stated and has no effect.
        "joined": ([(7, 3), (50, -40), (0, 0), (30, 0), (0, 0), (0, 0)],
                   [(-4, 2), (-25, 30), (0, 0), (-20, 0), (0, 0), (0, 0)]),
        "turned": ([(0, 0), (60, 60), (0, 0), (25, 0), (0, 0), (0, 0)],
                   [(0, 0), (-60, 10), (0, 0), (-15, 0), (0, 0), (0, 0)]),
        "nested": ([(-6, 4), (9, -3), (0, 0), (20, 0), (0, 0), (0, 0)],
                   [(3, -2), (-5, 6), (0, 0), (-10, 0), (0, 0), (0, 0)]),
        "metrics": ([(5, 5), (-40, 40), (0, 0), (0, 0), (0, 0), (0, 0)],
                    [(0, 0), (0, 0), (0, 0), (0, 0), (0, 0), (0, 0)]),
        "scaled": ([(6, -2), (0, 0), (0, 0), (0, 0), (0, 0), (0, 0)],
                   [(-4, 8), (0, 0), (0, 0), (0, 0), (0, 0), (0, 0)]),
    }
    metrics = {"bowl": (600, 100), "mark": (100, 0), "joined": (640, 110), "turned": (600, 100),
               "nested": (700, 0), "metrics": (100, 100), "scaled": (400, 50)}
    face(os.path.join(directory, "PointMatch.ttf"), "PointMatch", glyphs, deltas, metrics)


def phantom(directory):
    glyphs = {
        "bowl": BOWL,
        "mark": MARK,
        # Once mark is gathered, bowl's six points are 0 to 5 and mark's four
        # are 6 to 9, followed by its phantom points; of mark's own, 0 to 3 are
        # its outline and 4 to 7 its phantom points: left, right, top and
        # bottom.
        "right": composite(at("bowl", 200, 0), matched("mark", 3, 5)),
        "bottom": composite(at("bowl", 200, 0), matched("mark", 0, 7)),
        "self": composite(at("bowl", 200, 0), matched("mark", 9, 0, SCALED_COMPONENT_OFFSET, TURN)),
        "outside": composite(at("bowl", 200, 0), matched("mark", 200, 0)),
        # carried takes its metrics from mark, and so its phantom points, as
        # they were before mark was moved into it; carriedright matches the
        # right one of those.
        "carried": composite(at("mark", 30, 0, USE_MY_METRICS)),
        "carriedright": composite(at("bowl", 200, 0), matched("carried", 3, 5)),
    }
    deltas = {
        "bowl": (BOWL_MAX, BOWL_MIN),
        "mark": (MARK_MAX, MARK_MIN),
        "right": ([(0, 0), (11, 11), (0, 0), (0, 0), (0, 0), (0, 0)],
                  [(0, 0), (-7, 7), (0, 0), (0, 0), (0, 0), (0, 0)]),
        "bottom": ([(0, 0), (13, -13), (0, 0), (0, 0), (0, 0), (0, 0)],
                   [(0, 0), (5, 5), (0, 0), (0, 0), (0, 0), (0, 0)]),
        "self": ([(0, 0), (17, 3), (0, 0), (0, 0), (0, 0), (0, 0)],
                 [(0, 0), (2, -9), (0, 0), (0, 0), (0, 0), (0, 0)]),
        "outside": ([(0, 0), (8, 8), (0, 0), (0, 0), (0, 0), (0, 0)],
                    [(0, 0), (-8, -8), (0, 0), (0, 0), (0, 0), (0, 0)]),
        "carried": ([(4, -4), (0, 0), (-9, 0), (0, 0), (0, 0)],
                    [(-2, 3), (0, 0), (6, 0), (0, 0), (0, 0)]),
        "carriedright": ([(0, 0), (19, -6), (0, 0), (0, 0), (0, 0), (0, 0)],
                         [(0, 0), (-3, 21), (0, 0), (0, 0), (0, 0), (0, 0)]),
    }
    metrics = {"bowl": (600, 100), "mark": (100, 0), "right": (600, 100), "bottom": (600, 100),
               "self": (600, 100), "outside": (600, 100), "carried": (300, 30),
               "carriedright": (600, 100)}
    # The boxes, measured by matching as HarfBuzz matches: mark's phantom
    # points are its origin, its advance along, and — with no vertical
    # metrics — the top of its box and an em below that.
    bowl = [(x + 200, y) for x, y in BOWL.coordinates]
    mark = list(MARK.coordinates)
    mark_phantoms = [(0, 0), (100, 0), (0, 90), (0, 90 - 1000)]

    def box(first, second, points=mark, matrix=None):
        own = list(points) + mark_phantoms
        if matrix:
            (a, b), (c, d) = matrix
            own = [(a * x + c * y, b * x + d * y) for x, y in own]
        gathered = list(bowl) + own
        if first >= len(gathered):
            dx = dy = 0
        else:
            dx, dy = gathered[first][0] - own[second][0], gathered[first][1] - own[second][1]
        pts = list(bowl) + [(x + dx, y + dy) for x, y in own[:len(points)]]
        return (min(p[0] for p in pts), min(p[1] for p in pts),
                max(p[0] for p in pts), max(p[1] for p in pts))
    carried = [(x + 30, y) for x, y in mark]
    boxes = {"right": box(3, 5), "bottom": box(0, 7), "self": box(9, 0, matrix=TURN),
             "outside": box(200, 0), "carriedright": box(3, 5, carried)}
    boxes = {k: tuple(round(v) for v in b) for k, b in boxes.items()}
    face(os.path.join(directory, "PointMatchPhantom.ttf"), "PointMatchPhantom", glyphs, deltas, metrics,
         boxes)


point_match(sys.argv[1])
phantom(sys.argv[1])
