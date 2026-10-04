# Builds the font collections beside this file and wraps each in WOFF 2 with
# google/woff2's own encoder, then records what google/woff2's own decoder
# makes of each, so that font/woff2collection_test.go can hold DecodeWOFF2 to
# the format's reference implementation on collections it did not make:
#
#   python3 make.py WOFF2_BUILD_DIR
#
# WOFF2_BUILD_DIR holds woff2_compress and woff2_decompress, built with
# CMake from google/woff2 at the commit below, against brotli 1.2.0:
#
#   https://codeload.github.com/google/woff2/tar.gz/fb9c3379f2605b10f3e8f1d9636664ab5576775c
#   (sha256 f32f80941d621632fd668a00f3e30b00cde69df04ccb4730afb1809144bc6299)
#
# The collections are made with the pinned fontTools (testdata/harfbuzz/
# requirements.txt) from faces this repository builds itself, in
# testdata/harfbuzz/fonts, so that nothing here is anyone else's font:
#
#   shared.ttc    a face and a copy of it under another name, sharing every
#                 table but the name: one glyf, loca and hmtx for two fonts,
#                 which the encoder transforms once
#   distinct.ttc  three faces sharing nothing: composites placed by point
#                 matching, composites set upright, and a colour face
#   mixed.ttc     a TrueType face and a CFF one, whose CFF the encoder keeps
#                 untransformed
#
# Each NAME.ttc is kept beside NAME.woff2, so that the test can compare what
# each face decodes to with what the encoder was given. expected.txt is the
# reference decoder's answer for each NAME.woff2: the length of the
# collection it rebuilds, and the start of its SHA-256.
import hashlib
import os
import subprocess
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(os.path.dirname(os.path.dirname(HERE)))
sys.path.insert(0, os.path.join(ROOT, "testdata", "harfbuzz"))

from oracle import fonttools  # noqa: E402

fonttools()
from fontTools.ttLib import TTFont  # noqa: E402
from fontTools.ttLib.ttCollection import TTCollection  # noqa: E402

FONTS = os.path.join(ROOT, "testdata", "harfbuzz", "fonts")
build = sys.argv[1]


def face(name):
    # Kept as built, timestamps and all, so that making them again makes the
    # same bytes.
    f = TTFont(os.path.join(FONTS, name))
    f.recalcTimestamp = False
    return f


def renamed(name, family):
    f = face(name)
    for rec in f["name"].names:
        if rec.nameID in (1, 4, 16):
            rec.string = family
        if rec.nameID == 6:
            rec.string = family.replace(" ", "")
    return f


COLLECTIONS = {
    "shared": [face("PointMatch.ttf"), renamed("PointMatch.ttf", "PointMatch Copy")],
    "distinct": [face("PointMatch.ttf"), face("VerticalComposites.ttf"), face("ColourPaint.ttf")],
    "mixed": [face("VarComposite.ttf"), face("CFFInk.otf")],
}

lines = []
for name, fonts in COLLECTIONS.items():
    ttc = os.path.join(HERE, name + ".ttc")
    coll = TTCollection()
    coll.fonts = fonts
    coll.save(ttc, shareTables=True)
    subprocess.run([os.path.join(build, "woff2_compress"), ttc], check=True, capture_output=True)
    woff2 = os.path.join(HERE, name + ".woff2")
    # woff2_decompress writes NAME.ttf beside its input, for a collection too.
    subprocess.run([os.path.join(build, "woff2_decompress"), woff2], check=True, capture_output=True)
    rebuilt = os.path.join(HERE, name + ".ttf")
    data = open(rebuilt, "rb").read()
    os.remove(rebuilt)
    lines.append(f"{name}.woff2 {len(data)} {hashlib.sha256(data).hexdigest()[:16]}")

with open(os.path.join(HERE, "expected.txt"), "w", encoding="utf-8") as w:
    w.write("# What each WOFF 2 collection beside this decodes to, according to\n")
    w.write("# google/woff2's woff2_decompress at the commit make.py names, run on\n")
    w.write("# these bytes. Written by make.py. DO NOT EDIT.\n")
    w.write("#\n")
    w.write("# <file> <collection length in bytes> <sha256 of it, first 16 hex digits>\n")
    for line in lines:
        w.write(line + "\n")
