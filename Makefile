.PHONY: test bidi-tests test-bidi clean-bidi-tests hbshaping test-hbshaping hbfuzz useable clean-ucd stdfonts grapheme-tests test-grapheme clean-grapheme-tests css-tests test-css clean-css-tests html-entities clean-html-entities css-colors clean-css-colors noto-fonts clean-noto-fonts wpt test-wpt clean-wpt varinstance test-varinstance

test:
	gofmt -l . | grep -v '^testdata/' && exit 1 || true
	go vet ./...
	go test -count=1 ./...

# The same suite under the race detector.
#
# Separate from "test" because it is five times slower and the thing it looks for
# does not change with a line of layout code — what it watches is whether two
# documents laid out at once share anything, and the answer only moves when
# something becomes shared on purpose. Worth running when that might have
# happened: a new memo, a package-level var, a font set that caches.
.PHONY: race

race:
	go test -count=1 -race ./...

# Unicode's own bidirectional conformance suites, which bidi_conformance_test.go
# runs in full. Fetched rather than vendored: 15 MB, versioned by Unicode, and
# pinned to the release the tables were generated from — a character whose class
# changed between releases is a stale table rather than a defect.
BIDI_DIR := testdata/unicode-bidi
UNICODE_VERSION ?= 17.0.0
UCD_URL         := https://www.unicode.org/Public/$(UNICODE_VERSION)/ucd

bidi-tests: $(BIDI_DIR)/.ok

$(BIDI_DIR)/.ok:
	mkdir -p $(BIDI_DIR)
	curl -fsSL -o $(BIDI_DIR)/BidiTest.txt \
		$(UCD_URL)/BidiTest.txt
	curl -fsSL -o $(BIDI_DIR)/BidiCharacterTest.txt \
		$(UCD_URL)/BidiCharacterTest.txt
	touch $@

test-bidi: bidi-tests
	UNICODE_BIDI_TESTS=$(abspath $(BIDI_DIR)) go test -v -run TestBidiConformance -count=1 ./bidi

clean-bidi-tests:
	rm -rf $(BIDI_DIR)

# Shaping checked against HarfBuzz, over six fonts. See testdata/harfbuzz.
#
#	python3 -m venv .hbenv && .hbenv/bin/pip install uharfbuzz fonttools
#	PYTHON=.hbenv/bin/python make hbshaping
HARFBUZZ_DIR := testdata/harfbuzz
PYTHON ?= python3

hbshaping:
	$(PYTHON) $(HARFBUZZ_DIR)/corpus.py
	$(PYTHON) $(HARFBUZZ_DIR)/corpus_arabic.py
	$(PYTHON) $(HARFBUZZ_DIR)/corpus_khmer.py
	$(PYTHON) $(HARFBUZZ_DIR)/corpus_javanese.py
	$(PYTHON) $(HARFBUZZ_DIR)/corpus_balinese.py
	$(PYTHON) $(HARFBUZZ_DIR)/corpus_tibetan.py
	$(PYTHON) $(HARFBUZZ_DIR)/shape.py fonts/notosans/NotoSans-Variable.ttf \
		$(HARFBUZZ_DIR)/corpus.txt $(HARFBUZZ_DIR)/expected.txt
	$(PYTHON) $(HARFBUZZ_DIR)/shape.py $(HARFBUZZ_DIR)/fonts/NotoSansArabic.ttf \
		$(HARFBUZZ_DIR)/arabic.txt $(HARFBUZZ_DIR)/arabic.expected.txt
	$(PYTHON) $(HARFBUZZ_DIR)/shape.py $(HARFBUZZ_DIR)/fonts/NotoSansKhmer.ttf \
		$(HARFBUZZ_DIR)/khmer.txt $(HARFBUZZ_DIR)/khmer.expected.txt
	$(PYTHON) $(HARFBUZZ_DIR)/shape.py $(HARFBUZZ_DIR)/fonts/NotoSansJavanese.ttf \
		$(HARFBUZZ_DIR)/javanese.txt $(HARFBUZZ_DIR)/javanese.expected.txt
	$(PYTHON) $(HARFBUZZ_DIR)/shape.py $(HARFBUZZ_DIR)/fonts/NotoSansBalinese.ttf \
		$(HARFBUZZ_DIR)/balinese.txt $(HARFBUZZ_DIR)/balinese.expected.txt
	$(PYTHON) $(HARFBUZZ_DIR)/shape.py $(HARFBUZZ_DIR)/fonts/NotoSerifTibetan.ttf \
		$(HARFBUZZ_DIR)/tibetan.txt $(HARFBUZZ_DIR)/tibetan.expected.txt

test-hbshaping:
	go test -v -run 'TestShapingAgreesWithHarfBuzz|TestTheHarfBuzzOracleHasTeeth' -count=1 ./shape

# Instancing checked against fontTools and HarfBuzz, over four faces and eight
# locations. Needs the same Python as hbshaping.
#
# The expectations are checked in, so the Go test runs with nothing but a Go
# toolchain — an oracle only a machine with the right Python on it can consult is
# an oracle nobody consults. Regenerating them is what needs the Python, and only
# a change to what is compared should change what comes out.
varinstance:
	$(PYTHON) testdata/varinstance/instance.py

test-varinstance:
	go test -v -run 'TestInstancingAgreesWithFontToolsAndHarfBuzz|TestTheInstancingOracleHasTeeth' -count=1 ./shape

# Differential fuzzing against HarfBuzz. Needs the same Python as hbshaping.
hbfuzz:
	go build -o $(HARFBUZZ_DIR)/.shapetext ./cmd/shapetext
	SHAPETEXT=$(abspath $(HARFBUZZ_DIR)/.shapetext) $(PYTHON) $(HARFBUZZ_DIR)/difffuzz.py 60

# The Universal Shaping Engine's category table, derived from Unicode's own
# property files plus the engine's corrections. See cmd/genuse.
#
#	make useable UCD=/path/to/unpacked/ucd
UCD ?= testdata/ucd

useable:
	go run ./cmd/genuse \
		$(UCD)/IndicSyllabicCategory.txt \
		$(UCD)/IndicPositionalCategory.txt \
		$(UCD)/UnicodeData.txt \
		$(UCD)/DerivedCoreProperties.txt \
		$(UCD)/ArabicShaping.txt \
		testdata/ms-use/IndicSyllabicCategory-Additional.txt \
		testdata/ms-use/IndicPositionalCategory-Additional.txt \
		> shape/usetable.go
	gofmt -w shape/usetable.go

clean-ucd:
	rm -rf $(UCD)

# The broad font sweeps, over two libraries far too large to vendor: every OFL
# family Google publishes, and Noto's CJK faces.
#
# Both are fetched blobless and sparse, because only the faces are wanted.
# google/fonts is three gigabytes of which a fifth is screenshots and video, and
# noto-cjk is seven of which the subset OTFs are a few hundred megabytes. Taking
# the whole of either would cost several times what the fonts do.
#
# They are two libraries rather than one because they answer different
# questions. The OFL set is TrueType throughout — 3,795 faces and not one CFF —
# so it says a great deal about shaping and nothing whatever about the CFF
# reader. The CJK faces are CID-keyed CFF, which is the format this module
# refuses, so they are the ones that size that refusal.
#
#	make fonts       # fetch, or bring up to date if already fetched
#	make fontsweep   # read every face in both and report what happened
#	make clean-fonts # remove them
GF_DIR := testdata/googlefonts
CJK_DIR := testdata/notocjk

.PHONY: fonts googlefonts notocjk fontsweep clean-fonts

fonts: googlefonts notocjk

# Each target is written to be run twice. Fetching a couple of gigabytes over a
# promisor remote fails in the middle often enough that repairing it has to be
# ordinary rather than an incident: a clone that dies after the objects arrive
# but before the checkout leaves a directory with a .git in it and no fonts, and
# "pull if it exists" cannot mend that. So the clone is conditional and
# everything after it is not.
googlefonts:
	@test -d $(GF_DIR)/.git || git clone --filter=blob:none --no-checkout --sparse \
		https://github.com/google/fonts.git $(GF_DIR)
	git -C $(GF_DIR) fetch origin main
	git -C $(GF_DIR) sparse-checkout set --no-cone '/ofl/**/*.ttf'
	git -C $(GF_DIR) checkout -f -B main origin/main

# The CJK faces are fetched file by file rather than cloned.
#
# noto-cjk is seven gigabytes, and a blobless sparse clone of it failed twice in
# the same place — the pack for the subset directory is large enough that the
# connection dropped mid-sideband both times, leaving a .git with no fonts in it.
# The faces themselves are four megabytes each over plain HTTP and have never
# failed, so they are taken that way, which is what bidi-tests already does for
# Unicode's files.
#
# One weight per region is enough for what this is for. Every static CJK face is
# CID-keyed CFF, so any one of them exercises the refusal; the other six weights
# would be six more copies of the same answer.
CJK_BASE := https://raw.githubusercontent.com/notofonts/noto-cjk/main
CJK_FACES := \
	Sans/SubsetOTF/JP/NotoSansJP-Regular.otf \
	Sans/SubsetOTF/KR/NotoSansKR-Regular.otf \
	Sans/SubsetOTF/SC/NotoSansSC-Regular.otf \
	Sans/SubsetOTF/TC/NotoSansTC-Regular.otf \
	Sans/SubsetOTF/HK/NotoSansHK-Regular.otf \
	Serif/SubsetOTF/JP/NotoSerifJP-Regular.otf

notocjk:
	@mkdir -p $(CJK_DIR)
	@for f in $(CJK_FACES); do \
		out=$(CJK_DIR)/$$(basename $$f); \
		if [ -s "$$out" ]; then echo "have $$out"; else \
			echo "fetching $$out"; \
			curl -fsSL --retry 3 -o "$$out" "$(CJK_BASE)/$$f" || exit 1; \
		fi; \
	done
	@echo "$$(ls $(CJK_DIR)/*.otf | wc -l | tr -d ' ') CJK faces in $(CJK_DIR)"

fontsweep:
	go run ./cmd/fontsweep $(GF_DIR)/ofl $(CJK_DIR)

clean-fonts:
	rm -rf $(GF_DIR) $(CJK_DIR)

# The metrics of the fourteen standard PDF faces, from Adobe's own AFM files.
#
# The AFM set is freely redistributable and ships with a good deal of software
# — Ghostscript, matplotlib, poppler-data — but is not vendored here, because
# only the numbers are wanted and none of the files are redistributed. Point
# this at a directory holding them:
#
#	make stdfonts AFM=/path/to/afm
AFM ?= testdata/afm

stdfonts:
	go run ./cmd/genstdfonts $(AFM) > shape/standard14.go
	gofmt -w shape/standard14.go

# UAX #29's grapheme cluster boundaries, which package segment finds.
#
# GraphemeBreakTest.txt is Unicode's own statement of where every boundary falls
# in several hundred crafted strings, and GraphemeBreakProperty.txt with
# emoji-data.txt and DerivedCoreProperties.txt are what cmd/gensegment turns into
# the property table. The generated table is committed and the input is not, so a
# checkout builds with no network.
GRAPHEME_DIR := testdata/unicode-grapheme

grapheme-tests: $(GRAPHEME_DIR)/.ok

$(GRAPHEME_DIR)/.ok:
	mkdir -p $(GRAPHEME_DIR)
	curl -sSf -o $(GRAPHEME_DIR)/GraphemeBreakTest.txt $(UCD_URL)/auxiliary/GraphemeBreakTest.txt
	touch $@

test-grapheme: grapheme-tests
	UNICODE_GRAPHEME_TESTS=$(abspath $(GRAPHEME_DIR)) go test -v -run TestGrapheme -count=1 ./segment

clean-grapheme-tests:
	rm -rf $(GRAPHEME_DIR)

# shallow_at fetches exactly one commit of one repository: no history, no other
# branches. It came from pdf0 with the corpora below, which are the only things
# here that need it.
define shallow_at
	rm -rf $(1)
	git init -q $(1)
	git -C $(1) remote add origin $(2)
	git -C $(1) fetch -q --depth 1 origin $(3)
	git -C $(1) checkout -q FETCH_HEAD
endef

# CSS parsing tests (CC0, Simon Sapin): implementation-independent expected
# outputs for the algorithms of CSS Syntax Level 3, one JSON file per algorithm.
#
# This is the css package's external oracle, and the framing matters — see
# docs/adr/0003-arlington-as-parser-oracle.md for the two attempts this
# repository scrapped for guarding nothing. These expectations were written by
# someone else, from the specification, and three independent parsers
# (tinycss2, rust-cssparser, Crass) are checked against them. So a disagreement
# is evidence about pdf0 rather than a restatement of pdf0's own reading.
#
# Cloned under testdata (gitignored); tests skip if absent, mirroring `make
# corpus` and `make arlington`.
CSS_TESTS_DIR := testdata/css-parsing-tests

css-tests: $(CSS_TESTS_DIR)/.ok

$(CSS_TESTS_DIR)/.ok:
	$(call shallow_at,$(CSS_TESTS_DIR),https://github.com/SimonSapin/css-parsing-tests,$(CSS_TESTS_REF))
	touch $@

# The path is absolute because `go test ./css` runs with the package directory
# as its working directory, not the repository root.
test-css: css-tests
	CSS_PARSING_TESTS=$(abspath $(CSS_TESTS_DIR)) go test -v -run TestCSSOracle -count=1 ./css

clean-css-tests:
	rm -rf $(CSS_TESTS_DIR)

# The HTML standard's own list of named character references, which
# cmd/genhtmlentities turns into html/entities.go. The *generated table* is
# committed and the input is not, on the arrangement the font tables used before
# they moved to forme: the table is part of the source, and re-deriving it needs
# the network, so a checkout builds without one.
#
# Regenerate after the standard adds a name — which it has not done in years, so
# this is a rare errand rather than part of a build.
HTML_ENTITIES := testdata/html/entities.json

html-entities:
	mkdir -p $(dir $(HTML_ENTITIES))
	curl -sSf -o $(HTML_ENTITIES) https://html.spec.whatwg.org/entities.json
	go run ./cmd/genhtmlentities -in $(HTML_ENTITIES) -out html/entities.go
	gofmt -w html/entities.go

clean-html-entities:
	rm -f $(HTML_ENTITIES)

# The CSS Color 4 specification's named-colour table, which cmd/gencolors turns
# into style/colors.go.
#
# The source is the specification's own Bikeshed document and *not* the CSS
# parsing tests, which hold the same 148 mappings: generating the table from the
# suite that checks it would make that check circular, proving only that a file
# round-trips through a generator. As with the HTML entities, the generated table
# is committed and the input is not.
CSS_COLOR_SPEC := testdata/css-color-4.bs

css-colors:
	mkdir -p $(dir $(CSS_COLOR_SPEC))
	curl -sSf -o $(CSS_COLOR_SPEC) https://raw.githubusercontent.com/w3c/csswg-drafts/main/css-color-4/Overview.bs
	go run ./cmd/gencolors -in $(CSS_COLOR_SPEC) -out style/colors.go
	gofmt -w style/colors.go

clean-css-colors:
	rm -f $(CSS_COLOR_SPEC)

# Noto, for the scripts the fourteen standard PDF faces do not have.
#
# Those fourteen cover Latin and nothing else, so a document with a Hebrew word
# or a kana in it gets a face that cannot encode the letters — and since the
# encoder substitutes a space for anything it cannot represent, the word is
# absent from the page rather than showing as boxes anyone would notice. The
# reftest harness hands these to the engine through FallbackFontSet.
#
# Measured against the suite: the three between them cover 81% of the characters
# the standard faces are missing and clear 64% of the documents that report one,
# against 50% for the best single font tried (DejaVu Sans) and 32% for a
# monospaced one (Cascadia Mono). Coverage per character is a poor guide —
# a document stops reporting only when *every* character it uses is covered, so
# the two commonest characters decide more than the long tail does.
#
# Licensing: all three are SIL Open Font License 1.1, which is why they were
# chosen over DejaVu Sans — it scores better on characters and is under the
# Bitstream Vera licence instead. As with Ahem, pdf0 neither vendors nor
# redistributes them: they are fetched into this gitignored directory, used only
# to run the tests, and no font bytes ship in this repository or anything it
# builds. The licence text is fetched alongside them.
#
# The Japanese face is the variable TTF and not one of the static OTFs, because
# those are CID-keyed CFF and forme does not read them. forme instantiates it at
# the font's default, which its name table reports as Thin — so CJK set through
# this fallback is lighter than it should be. It is a fallback for text that
# would otherwise be invisible, and the weight being wrong is worth saying out
# loud rather than leaving to be discovered.
NOTO_DIR := testdata/fonts-noto
NOTO_BASE := https://raw.githubusercontent.com/notofonts
NOTO_HINTED := NotoSans NotoSansHebrew NotoSansArabic NotoSansDevanagari \
               NotoSansArmenian NotoSansGeorgian

noto-fonts: $(NOTO_DIR)/.ok

$(NOTO_DIR)/.ok:
	mkdir -p $(NOTO_DIR)
	for fam in $(NOTO_HINTED); do \
	  curl -sSf -o $(NOTO_DIR)/$$fam-Regular.ttf \
	    $(NOTO_BASE)/notofonts.github.io/main/fonts/$$fam/hinted/ttf/$$fam-Regular.ttf; \
	done
	curl -sSf -o $(NOTO_DIR)/NotoSerifTibetan-Regular.ttf \
	  $(NOTO_BASE)/notofonts.github.io/main/fonts/NotoSerifTibetan/hinted/ttf/NotoSerifTibetan-Regular.ttf
	curl -sSf -o $(NOTO_DIR)/NotoSansJP-VF.ttf \
	  $(NOTO_BASE)/noto-cjk/main/Sans/Variable/TTF/Subset/NotoSansJP-VF.ttf
	curl -sSf -o $(NOTO_DIR)/OFL.txt \
	  $(NOTO_BASE)/noto-cjk/main/Sans/LICENSE
	touch $@

clean-noto-fonts:
	rm -rf $(NOTO_DIR)


# W3C Web Platform Tests: the external oracle for the layout engine.
#
# A CSS reftest is a pair of documents with the assertion *these two render
# identically*, and the pair and the claim come from the CSS Working Group. That
# is what makes it an oracle rather than a restatement of pdf0's own reading —
# ADR 0003 records what this repository already learned about the difference.
# Reftests are also built so that the two documents reach the same rendering by
# *different* mechanisms, so an engine bug usually moves one and not the other.
#
# No browser is needed: pdf0 renders both and compares its own display lists.
#
# WPT is enormous, so this is a blobless sparse clone rather than the whole of
# it. The directories are everything a page laid out *once* can be held to.
#
# What is left out is left out for a reason and not for convenience: pagination
# and page-box describe flowing content across several pages, which §2.2 decides
# against; ui and run-in are interaction and a feature CSS removed. Floats,
# positioning and z-index are emphatically *in* — they are only dynamic in a
# viewport that resizes, and this one does not.
WPT_DIR  := testdata/wpt
WPT_REF  ?= master
WPT_DIRS := css/CSS2/normal-flow css/CSS2/box-display css/CSS2/margin-padding-clear \
            css/CSS2/abspos css/CSS2/positioning css/CSS2/visuren css/CSS2/visudet \
            css/CSS2/visufx css/CSS2/floats css/CSS2/floats-clear css/CSS2/tables \
            css/CSS2/zindex css/CSS2/zorder css/CSS2/stacking-context \
            css/CSS2/linebox css/CSS2/text css/CSS2/bidi-text css/CSS2/lists \
            css/CSS2/generated-content css/CSS2/borders css/CSS2/backgrounds \
            css/CSS2/box css/CSS2/colors css/CSS2/values \
            css/CSS2/support css/CSS2/reference css/css-text/white-space css/reference \
            fonts

# "fonts" is there for Ahem.ttf, which a quarter of the suite is written
# against and which the harness hands to the engine — see render/ahem_test.go
# for why a test font is the only way those assertions can be expressed.
#
# Licensing, since it is a font and fonts often are not as free as the code
# around them: Ahem.ttf is tracked in the web-platform-tests repository, which
# is under the 3-Clause BSD licence above, and carries no separate licence of
# its own. pdf0 neither vendors nor redistributes it — it is fetched into this
# gitignored directory exactly as the rest of the corpus is, is used only to run
# the tests, and no font bytes are shipped in this repository or in anything it
# builds. The exposure is therefore the same as depending on the suite at all,
# which the ratchet already does.

wpt: $(WPT_DIR)/.ok

$(WPT_DIR)/.ok:
	rm -rf $(WPT_DIR)
	git clone --filter=blob:none --sparse --depth 1 \
		https://github.com/web-platform-tests/wpt.git $(WPT_DIR)
	git -C $(WPT_DIR) sparse-checkout set $(WPT_DIRS)
	touch $@

test-wpt: wpt
	WPT_TESTS=$(abspath $(WPT_DIR)) go test -v -run TestWPT -count=1 ./layout/

clean-wpt:
	rm -rf $(WPT_DIR)