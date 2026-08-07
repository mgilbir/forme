# Instances each font at a point in its design space and writes what two other
# implementations produce there.
#
# The output is checked in and is what shape/varinstance_test.go compares this
# package against. Regenerating it needs Python, fontTools and uharfbuzz;
# running the test needs none of them, which is the point — an oracle only a
# machine with the right Python on it can consult is an oracle nobody consults.
#
#   make varinstance
#
# # Two oracles, because neither answers the whole question
#
# fontTools' varLib.instancer cuts a static instance the way a foundry does, and
# is where the outlines, the bounding boxes and the side bearings come from.
# HarfBuzz is what a browser renders with, and is where the advances come from:
# where a font states its advances twice — in HVAR and in gvar's phantom points —
# they can disagree, and what a reader draws is HVAR's answer. Noto Sans states
# thirteen of them differently at weight 700.
#
# So each glyph line carries HarfBuzz's advance beside fontTools' geometry, and
# a case with 'nohvar' in its name is the same font with HVAR taken out, where
# the phantom points are all there is and fontTools' own advance is the answer.
#
# # The noise floor, and why it is written into the header
#
# The three implementations do not agree to the last unit, and cannot: each
# arrives at a scalar by its own arithmetic. HarfBuzz quantizes the location to
# the fourteen fractional bits the format stores, fontTools' instancer routes a
# pinned axis through its partial-instancing solver, and both land a whole font
# unit from the other on a few points in a hundred.
#
# The 'noise' header is that floor, measured: how far fontTools' instancer is
# from fontTools' *own* supportScalar and IUP applied directly, over the whole
# font. It is the amount of disagreement the oracle has with itself, and it is
# what the Go test's allowance for each case is set from. A case whose noise is
# zero is one where every implementation agrees exactly, and the test demands
# exactly that.
import hashlib
import io
import os
import sys

import uharfbuzz as hb
from fontTools.ttLib import TTFont
from fontTools.varLib import instancer
from fontTools.varLib.iup import iup_delta
from fontTools.varLib.models import normalizeLocation, piecewiseLinearMap, supportScalar
from fontTools.varLib.varStore import VarStoreInstancer
from fontTools.ttLib.tables._g_l_y_f import GlyphCoordinates
from fontTools.misc.fixedTools import otRound

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(os.path.dirname(HERE))

# Each case names a font, a location in user coordinates, and whether HVAR is
# taken out first. Between them they cover: the default instance, both ends of
# an axis, a location between two of avar's own segments, two axes at once, a
# font with no avar at all, and a font with no HVAR.
CASES = [
    ("noto-default", "fonts/notosans/NotoSans-Variable.ttf", {}, False),
    ("noto-thin", "fonts/notosans/NotoSans-Variable.ttf", {"wght": 100}, False),
    ("noto-bold", "fonts/notosans/NotoSans-Variable.ttf", {"wght": 700}, False),
    ("noto-thin-condensed", "fonts/notosans/NotoSans-Variable.ttf", {"wght": 100, "wdth": 75}, False),
    ("noto-bold-nohvar", "fonts/notosans/NotoSans-Variable.ttf", {"wght": 700}, True),
    ("arabic-black", "testdata/harfbuzz/fonts/NotoSansArabic.ttf", {"wght": 900}, False),
    ("tibetan-light", "testdata/harfbuzz/fonts/NotoSerifTibetan.ttf", {"wght": 250}, False),
    ("khmer-light-condensed", "testdata/harfbuzz/fonts/NotoSansKhmer.ttf", {"wght": 250, "wdth": 90}, False),
]


def normalized(ft, loc):
    axes = {a.axisTag: (a.minValue, a.defaultValue, a.maxValue) for a in ft["fvar"].axes}
    n = normalizeLocation(loc, axes)
    if "avar" in ft:
        for tag, seg in ft["avar"].segments.items():
            if tag in n and seg:
                n[tag] = piecewiseLinearMap(n[tag], seg)
    return [n[a.axisTag] for a in ft["fvar"].axes]


def reference(ft, loc):
    """Where every point lands when fontTools' supportScalar and IUP are applied
    directly, with the partial-instancing solver out of the way.

    It is a second assembly of fontTools' own parts, and it exists to measure the
    oracle's disagreement with itself. Nothing is written out from it."""
    n = dict(zip([a.axisTag for a in ft["fvar"].axes], normalized(ft, loc)))
    glyf, gvar, hmtx = ft["glyf"], ft["gvar"], ft["hmtx"]
    out = {}
    for gid, name in enumerate(ft.getGlyphOrder()):
        coords, ctrl = glyf._getCoordinatesAndControls(name, hmtx.metrics)
        total = [(0.0, 0.0)] * len(coords)
        for var in gvar.variations.get(name, []):
            scalar = supportScalar(n, var.axes)
            if not scalar:
                continue
            delta = [None if d is None else (d[0] * scalar, d[1] * scalar) for d in var.coordinates]
            if None in delta:
                delta = iup_delta(delta, coords, ctrl.endPts)
            total = [(a[0] + b[0], a[1] + b[1]) for a, b in zip(total, delta)]
        out[name] = [(c[0] + d[0], c[1] + d[1]) for c, d in zip(coords, total)]
    return out


def sample(ft, gvar):
    """The glyphs written out in full.

    Writing every glyph of every case would be several megabytes, so the sample
    is picked to be where instancing goes wrong rather than at random: the
    glyphs with the most variation tuples, the ones with the most points, the
    composites with the most components, the first few glyph indices, and a
    spread over the rest so that nothing is systematically left out."""
    order = ft.getGlyphOrder()
    glyf = ft["glyf"]
    chosen = set(range(min(40, len(order))))
    chosen.update(range(0, len(order), max(1, len(order) // 40)))

    def top(key, n):
        return [gid for gid, _ in sorted(
            ((gid, key(gid, name)) for gid, name in enumerate(order)),
            key=lambda p: (-p[1], p[0]))[:n]]

    chosen.update(top(lambda gid, name: len(gvar.variations.get(name, [])), 40))
    chosen.update(top(lambda gid, name: len(getattr(glyf[name], "coordinates", ())), 40))
    chosen.update(top(lambda gid, name: len(getattr(glyf[name], "components", ())), 40))
    return sorted(chosen)


def reference_font(raw, src, ref, coords):
    """A font carrying the reference's coordinates, so that its bounding boxes
    and side bearings are recomputed by fontTools from those rather than from the
    instancer's — using fontTools' own _setCoordinates, which is what the
    instancer itself calls.

    Composites are set after the glyphs they are built from, because a
    composite's box is measured from its components as they now stand."""
    ft = TTFont(io.BytesIO(raw))
    glyf, hmtx = ft["glyf"], ft["hmtx"]
    order = sorted(
        ft.getGlyphOrder(),
        key=lambda n: (glyf[n].getCompositeMaxpValues(glyf).maxComponentDepth
                       if glyf[n].isComposite() else 0, n),
    )
    for name in order:
        glyf._setCoordinates(name, GlyphCoordinates(ref[name]), hmtx.metrics)
    if "HVAR" in src:
        # Where the font states advances twice, the advance written out is the
        # one a renderer uses, so the reference has to take the same one.
        hvar = src["HVAR"].table
        base = TTFont(io.BytesIO(raw))["hmtx"]
        inst = VarStoreInstancer(hvar.VarStore, src["fvar"].axes,
                                 dict(zip([a.axisTag for a in src["fvar"].axes], coords)))
        for gid, name in enumerate(src.getGlyphOrder()):
            outer, inner = 0, gid
            if hvar.AdvWidthMap:
                idx = hvar.AdvWidthMap.mapping[name]
                outer, inner = idx >> 16, idx & 0xFFFF
            delta = inst[(outer << 16) | inner] if hvar.AdvWidthMap else inst[gid]
            hmtx.metrics[name] = (max(0, base[name][0] + otRound(delta)), hmtx.metrics[name][1])
    return glyf, hmtx.metrics


def write_case(name, rel, loc, nohvar, out_path):
    raw = open(os.path.join(ROOT, rel), "rb").read()
    font_sha = hashlib.sha256(raw).hexdigest()
    if nohvar:
        f = TTFont(io.BytesIO(raw))
        del f["HVAR"]
        b = io.BytesIO()
        f.save(b)
        raw = b.getvalue()

    ft = TTFont(io.BytesIO(raw))
    order = ft.getGlyphOrder()
    coords = normalized(ft, loc)

    advances = None
    if not nohvar:
        face = hb.Face(raw)
        hbfont = hb.Font(face)
        hbfont.scale = (face.upem, face.upem)
        hbfont.set_variations(loc)
        advances = [hbfont.get_glyph_h_advance(gid) for gid in range(len(order))]

    ref = reference(ft, loc)
    ref_glyf, ref_hmtx = reference_font(raw, ft, ref, coords)
    gids = sample(ft, ft["gvar"])
    instancer.instantiateVariableFont(ft, loc, inplace=True, optimize=False, updateFontNames=False)
    glyf, hmtx = ft["glyf"], ft["hmtx"]

    # The noise floor: every value this file states, against the same value
    # reached the other way. It is what the Go test's allowance is judged
    # against, so it has to be measured over exactly the values the Go test
    # compares — a bounding box is not a point, and one point out by a unit can
    # put a composite's box out by two.
    def values(g, adv, lsb, gl):
        out = [adv, lsb]
        if g.numberOfContours == 0:
            return out
        out += [g.xMin, g.yMin, g.xMax, g.yMax]
        if g.isComposite():
            for c in g.components:
                out += [otRound(c.x), otRound(c.y)]
        else:
            for x, y in g.coordinates:
                out += [otRound(x), otRound(y)]
        return out

    noise = noise_worst = noise_sampled = 0
    for gid, gname in enumerate(order):
        a = values(glyf[gname], advances[gid] if advances is not None else hmtx[gname][0],
                   hmtx[gname][1], glyf)
        b = values(ref_glyf[gname], ref_hmtx[gname][0], ref_hmtx[gname][1], ref_glyf)
        if len(a) != len(b):
            raise SystemExit("glyph %s has %d values one way and %d the other" % (gname, len(a), len(b)))
        for x, y in zip(a, b):
            if x != y:
                noise += 1
                noise_worst = max(noise_worst, abs(x - y))
                if gid in set(gids):
                    noise_sampled += 1

    lines = [
        "# Generated by testdata/varinstance/instance.py. DO NOT EDIT.",
        "#",
        "# One line per sampled glyph: kind, glyph index, advance, left side",
        "# bearing, bounding box, then the points — or, for a composite, each",
        "# component's glyph index and offset. The advance is HarfBuzz's and the",
        "# rest is fontTools'; see instance.py for why they come from different",
        "# places.",
        "#",
        "font %s" % rel,
        "font-sha256 %s" % font_sha,
        "strip-hvar %s" % ("yes" if nohvar else "no"),
        "location %s" % (" ".join("%s=%g" % kv for kv in sorted(loc.items())) or "default"),
        "axes %s" % " ".join(a.axisTag for a in TTFont(io.BytesIO(raw))["fvar"].axes),
        "normalized %s" % " ".join(repr(c) for c in coords),
        "glyphs %d" % len(order),
        "noise %d" % noise,
        "noise-sampled %d" % noise_sampled,
        "noise-worst %d" % noise_worst,
    ]
    for gid in gids:
        gname = order[gid]
        g = glyf[gname]
        adv, lsb = hmtx[gname]
        if advances is not None:
            adv = advances[gid]
        if g.numberOfContours == 0:
            lines.append("e %d %d %d" % (gid, adv, lsb))
            continue
        box = "%d %d %d %d" % (g.xMin, g.yMin, g.xMax, g.yMax)
        if g.isComposite():
            parts = " ".join("%d:%d,%d" % (ft.getGlyphID(c.glyphName), otRound(c.x), otRound(c.y))
                             for c in g.components)
            lines.append("c %d %d %d %s %s" % (gid, adv, lsb, box, parts))
        else:
            parts = " ".join("%d,%d" % (otRound(x), otRound(y)) for x, y in g.coordinates)
            lines.append("s %d %d %d %s %s" % (gid, adv, lsb, box, parts))
    with open(out_path, "w") as w:
        w.write("\n".join(lines) + "\n")
    print("%-22s %4d glyphs sampled of %d, noise %d values (%d sampled, worst %d)"
          % (name, len(gids), len(order), noise, noise_sampled, noise_worst))


def main():
    for name, rel, loc, nohvar in CASES:
        write_case(name, rel, loc, nohvar, os.path.join(HERE, name + ".txt"))


if __name__ == "__main__":
    main()
