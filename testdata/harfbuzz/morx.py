# Shapes every case of the text-rendering tests' AAT morx suite with HarfBuzz,
# and strings in the faces morx_fixture.py builds, and writes what it
# produces, so that shape/morx_test.go can hold the morx reader
# (shape/morx.go) and the features it takes (shape/aatfeatures.go) to it.
#
#   make hbmorx
#
# The fonts and the cases are the Unicode Consortium's text-rendering-tests,
# MORX-1 to MORX-42, as HarfBuzz keeps them under test/shape/data/
# text-rendering-tests at the release the oracle is pinned to, and they are
# kept in aat/ (see aat/README). Each case names a font, a string of code
# points and the glyphs the suite expects, by name and position; HarfBuzz
# runs the suite as part of its own, and this checks that it still sets every
# case as the suite says before writing anything, so that what is written is
# both HarfBuzz's answer and the suite's. A case the suite expects only to
# finish (*) is written as HarfBuzz sets it, or as "fails" where HarfBuzz
# gives up on it.
#
# The fixture's cases are shaped with the features each names, as a caller
# hands them to hb_shape: those turned off first and those turned on after,
# each sorted, which is the order a run asks for them in from Features'
# TagsOff and Tags (shape/plan.go's requested), or — for a case that only
# turns features on — in the order named, as ShapeGlyphsWith asks for them.
#
# What is written is each case's features, "." for none, and glyphs as
# HarfBuzz states them: index, cluster (a byte offset into the UTF-8 string),
# x advance, x offset and y offset, in font units.
import hashlib
import os
import sys

from oracle import harfbuzz

hb = harfbuzz()

HERE = os.path.dirname(os.path.abspath(__file__))
AAT = os.path.join(HERE, "aat")
out_path = sys.argv[1]

# The strings shaped in MorxCases.ttf (morx_fixture.py), for what the suite
# does not reach: the chain's flags (A, and not B), the insertion only a C
# can start, in a run of under four glyphs and of four or more, after C has
# become Y; and the end-of-text substitution after a marked D and an
# unmarked F. And two asked with features, which a face with no feat table
# runs with its chains' default flags whatever is asked.
FIXTURE = [("MorxCases.ttf", t, ".") for t in
           ["AB", "B", "CA", "CAAA", "AAAA", "AC", "DE", "FE", "DEF", "FDE", "EDE"]]
FIXTURE += [("MorxCases.ttf", "AB", "-liga,-calt"), ("MorxCases.ttf", "AB", "+smcp,+liga")]

# MorxFeatures.ttf: each feature the chain names, alone and together, on and
# off; the two exclusive figure and text spacing settings in both orders, of
# which the first asked for is kept; and features the face offers no type for.
# Its deprecated twin, whose feat offers small capitals under their old type,
# and its twin with no feat at all, which takes nothing asked.
FEATURE_SETS = [".", "-calt", "+calt", "+liga", "-liga", "+smcp", "+aalt", "+lnum", "+onum",
                "+lnum,+onum", "+onum,+lnum", "+fwid,+hwid", "+hwid,+fwid", "-calt,+liga,+smcp", "+kern",
                "-kern", "+ss01", "-aalt", "-smcp,+liga"]
for face in ("MorxFeatures.ttf", "MorxFeaturesDeprecated.ttf", "MorxFeaturesNoFeat.ttf"):
    FIXTURE += [(face, "GHIJKLMNO", f) for f in FEATURE_SETS]

FIXTURE_FONTS = {"MorxCases.ttf", "MorxFeatures.ttf", "MorxFeaturesDeprecated.ttf", "MorxFeaturesNoFeat.ttf"}


def cases():
    for font, text, features in FIXTURE:
        yield "fixture", font, text, "*", features
    for name in sorted(os.listdir(os.path.join(AAT, "tests")),
                       key=lambda n: int(n.split("-")[1].split(".")[0])):
        for line in open(os.path.join(AAT, "tests", name), encoding="utf-8"):
            line = line.strip()
            if not line or line.startswith("@") or line.startswith("#"):
                continue
            font, options, codepoints, expected = line.split(";")
            # A case expecting * asks only that shaping finish; it states no
            # options, and is shaped as the rest are.
            if expected != "*" and options.split() != ["--font-size=1000", "--ned", "--remove-default-ignorables"]:
                sys.exit(f"{name}: options this oracle does not ask with: {options}")
            text = "".join(chr(int(c[2:], 16)) for c in codepoints.split(","))
            yield name, os.path.basename(font), text, expected, "."


def hb_features(spec):
    """The features a spec names, as hb_shape is handed them: in the order
    named where all are turned on, and otherwise those turned off, sorted,
    then those turned on, sorted."""
    if spec == ".":
        return {}
    named = [(f[1:], f[0] == "+") for f in spec.split(",")]
    if all(on for _, on in named):
        return dict(named)
    return dict(sorted((t, v) for t, v in named if not v)) | dict(sorted((t, v) for t, v in named if v))


def names(font, buf):
    """The suite's own notation: glyph names, each after the first at its
    pen position, as hb-shape --ned writes them."""
    out, x, y = [], 0, 0
    for i, (info, pos) in enumerate(zip(buf.glyph_infos, buf.glyph_positions)):
        name = font.glyph_to_string(info.codepoint)
        gx, gy = x + pos.x_offset, y + pos.y_offset
        out.append(name if i == 0 and gx == 0 and gy == 0 else f"{name}@{gx},{gy}")
        x += pos.x_advance
        y += pos.y_advance
    return "[" + "|".join(out) + "]"


fonts = {}
out = []
for test, name, text, expected, features in cases():
    if name not in fonts:
        folder = HERE if name in FIXTURE_FONTS else AAT
        data = open(os.path.join(folder, "fonts", name), "rb").read()
        face = hb.Face(data)
        font = hb.Font(face)
        font.scale = (face.upem, face.upem)
        fonts[name] = (font, hashlib.sha256(data).hexdigest())
    font, digest = fonts[name]
    buf = hb.Buffer()
    buf.add_str(text)
    buf.guess_segment_properties()
    buf.flags = hb.BufferFlags.REMOVE_DEFAULT_IGNORABLES
    codes = ",".join(f"{ord(c):04X}" for c in text)
    try:
        hb.shape(font, buf, hb_features(features))
    except MemoryError:
        # HarfBuzz gave up: the case's state machine ran past the buffer's
        # allowance, which uharfbuzz reports as running out of memory.
        if expected != "*":
            sys.exit(f"{test}: HarfBuzz could not set {text!r} in {name}")
        out.append(f"{test} {name} {digest} {codes} {features} fails")
        continue
    got = names(font, buf)
    if expected != "*" and got != expected:
        sys.exit(f"{test}: HarfBuzz sets {text!r} in {name} as {got}, and the suite expects {expected}")
    # uharfbuzz numbers clusters by code point; a byte offset into the UTF-8
    # string is what a Glyph's Cluster is.
    at = [len(text[:k].encode("utf-8")) for k in range(len(text) + 1)]
    glyphs = " ".join(f"{i.codepoint},{at[i.cluster]},{p.x_advance},{p.x_offset},{p.y_offset}"
                      for i, p in zip(buf.glyph_infos, buf.glyph_positions))
    out.append(f"{test} {name} {digest} {codes} {features} {glyphs}")

with open(out_path, "w", encoding="utf-8") as w:
    w.write("# Generated by testdata/harfbuzz/morx.py. DO NOT EDIT.\n")
    w.write("#\n")
    w.write("# Each case of the text-rendering tests' morx suite and of the fixture\n")
    w.write("# faces: the test, the font, its SHA-256, the code points, the features\n")
    w.write("# asked for (+ on, - off, . none), and the glyphs HarfBuzz sets them as —\n")
    w.write("# index, cluster (a byte offset into the UTF-8 string), x advance,\n")
    w.write("# x offset and y offset, in font units — which for the suite are the\n")
    w.write("# glyphs it expects, by name and position; or fails, where HarfBuzz\n")
    w.write("# gives up.\n")
    w.write(f"# harfbuzz {hb.version_string()}\n")
    w.write(f"# uharfbuzz {hb.__version__}\n")
    w.write(f"# cases {len(out)}\n")
    for line in out:
        w.write(line + "\n")
