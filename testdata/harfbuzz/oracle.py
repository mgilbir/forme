# The HarfBuzz every oracle here asks, and the refusal to ask any other.
#
# The expectation files are HarfBuzz's answers at one release. Written by
# whatever `pip install uharfbuzz` happened to fetch, they were answers at three:
# 14.3.0, 14.4.0 and 14.5.0 sat side by side, and a regeneration of one of them
# would have moved it to a fourth without anyone deciding that. A difference
# between two HarfBuzz releases is a change of HarfBuzz's mind, not a change to
# this package, and the files cannot tell the two apart unless they are all the
# same release.
#
# So the release is pinned in two places that have to agree: requirements.txt
# beside this file names the uharfbuzz release, by digest, and the Makefile's
# HARFBUZZ_VERSION names the HarfBuzz release — the one HarfBuzz's own data
# files are fetched at. harfbuzz() refuses to hand back a uharfbuzz that is not
# the first or does not carry the second, so a generator stops before it writes
# anything; and shape/oraclepin_test.go refuses a file that records another.
import os
import re
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(os.path.dirname(HERE))


def pinned():
    """The uharfbuzz release requirements.txt pins, and the HarfBuzz release the
    Makefile names."""
    req = open(os.path.join(HERE, "requirements.txt"), encoding="utf-8").read()
    m = re.search(r"^uharfbuzz==(\S+)", req, re.M)
    if not m:
        sys.exit("requirements.txt pins no uharfbuzz release")
    mk = open(os.path.join(ROOT, "Makefile"), encoding="utf-8").read()
    n = re.search(r"^HARFBUZZ_VERSION := (\S+)$", mk, re.M)
    if not n:
        sys.exit("the Makefile names no HARFBUZZ_VERSION")
    return m.group(1), n.group(1)


def harfbuzz():
    """uharfbuzz, once it is known to be the pinned release."""
    import uharfbuzz as hb

    want_py, want_hb = pinned()
    if hb.__version__ != want_py or hb.version_string() != want_hb:
        sys.exit(
            "this is uharfbuzz %s carrying HarfBuzz %s, and the oracle is pinned to "
            "uharfbuzz %s carrying HarfBuzz %s.\nInstall the pinned one with "
            "`make hbenv` and run with PYTHON=.hbenv/bin/python."
            % (hb.__version__, hb.version_string(), want_py, want_hb))
    return hb


def fonttools():
    """fontTools, once it is known to be the release requirements.txt pins."""
    import fontTools

    req = open(os.path.join(HERE, "requirements.txt"), encoding="utf-8").read()
    m = re.search(r"^fonttools==(\S+)", req, re.M)
    if not m:
        sys.exit("requirements.txt pins no fontTools release")
    if fontTools.version != m.group(1):
        sys.exit(
            "this is fontTools %s, and the oracle is pinned to %s.\nInstall the "
            "pinned one with `make hbenv` and run with PYTHON=.hbenv/bin/python."
            % (fontTools.version, m.group(1)))
    return fontTools
