.PHONY: linebreak vertical dictionaries test bidi-tests test-bidi clean-bidi-tests hbshaping test-hbshaping hbfuzz useable clean-ucd stdfonts grapheme-tests test-grapheme clean-grapheme-tests css-tests test-css clean-css-tests html-entities clean-html-entities css-colors clean-css-colors noto-fonts clean-noto-fonts wpt test-wpt clean-wpt varinstance test-varinstance

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

# The characters a line may not begin with, from Unicode's line-breaking
# property. See cmd/genlinebreak for which of UAX #14's rules are in it.
#
#	make linebreak UCD=/path/to/unpacked/ucd
linebreak:
	go run ./cmd/genlinebreak $(UCD)/LineBreak.txt > paragraph/linebreaktable.go
	gofmt -w paragraph/linebreaktable.go

# Unicode's full case mappings — the ones that turn one character into more than
# one, which Go's own case functions cannot express. See cmd/gencasing.
#
#	make casing UCD=/path/to/unpacked/ucd
casing:
	go run ./cmd/gencasing $(UCD)/SpecialCasing.txt > paragraph/casingtable.go
	gofmt -w paragraph/casingtable.go

# The two properties CSS Text's segment break transformation reads: East Asian
# Width, and which characters are Hangul. See cmd/geneastasian.
#
#	make eastasian UCD=/path/to/unpacked/ucd
eastasian:
	go run ./cmd/geneastasian $(UCD)/EastAsianWidth.txt $(UCD)/Scripts.txt \
	  > paragraph/eastasiantable.go
	gofmt -w paragraph/eastasiantable.go

# The word lists CSS Text §5.1's lexical line breaking needs, for the scripts
# that write no spaces between their words.
#
# ICU's break-iterator dictionaries, which are what every browser segments these
# scripts with — so a page laid out from them breaks where a reader of the
# suite's references expects. They are fetched rather than kept in this
# repository, and the *generated* table is what is committed: the licence
# travels into it, which is the Unicode terms' own requirement and is why
# cmd/gendict copies the header rather than summarising it.
#
# The four together are 179,000 words and four megabytes of Go, which is by a
# wide margin the largest thing in this repository. That is what the feature
# costs: where one word ends and the next begins in Thai is a fact about the
# vocabulary, so an engine without the vocabulary cannot know it, and there is
# no smaller form the knowledge comes in.
#
# They are the four class SA scripts ICU publishes a list for. The rest of the
# class — Tai Tham, Tai Le, Tai Viet and their neighbours — has none to publish,
# and UnsupportedScript is what says so about them.
#
#	make dictionaries
ICU_DICTS := https://raw.githubusercontent.com/unicode-org/icu/main/icu4c/source/data/brkitr/dictionaries
DICT_DIR  := testdata/icu-dictionaries

ICU_DICT_NAMES := thai:thaidict lao:laodict khmer:khmerdict burmese:burmesedict

dictionaries:
	mkdir -p $(DICT_DIR)
	for pair in $(ICU_DICT_NAMES); do \
	  name=$${pair%%:*}; file=$${pair##*:}; \
	  curl -sSf -o $(DICT_DIR)/$$file.txt $(ICU_DICTS)/$$file.txt; \
	  go run ./cmd/gendict $$name $(DICT_DIR)/$$file.txt > paragraph/$$name'dict.go'; \
	  gofmt -w paragraph/$$name'dict.go'; \
	done

# The phrase model CSS Text §5.2's "auto-phrase" needs, for the language whose
# words run together and whose phrases do not.
#
# BudouX, which is what Chromium segments Japanese phrases with — so a page laid
# out from it breaks where a reader of the suite's references expects. As with
# the word lists above the model is fetched and the *generated* table is what is
# committed, and the licence travels into it: BudouX is Apache-2.0, whose terms
# require that recipients get them, so cmd/genphrase copies the file rather than
# naming it.
#
# It is twenty kilobytes against the word lists' four megabytes, and the
# difference is what a model is. A phrase is a content word with its particles
# stuck to it, so no list of words can say where one ends; sixteen hundred
# weights over the characters around a boundary can, and are wrong often enough
# that the suite's own tests allow more than one answer.
#
# One language. BudouX publishes Chinese and Thai as well, no document in the
# suite asks for either, and Thai already breaks at the words its ICU dictionary
# knows. Adding one is adding a pair to PHRASE_MODELS.
#
#	make phrases
BUDOUX      := https://raw.githubusercontent.com/google/budoux/main
BUDOUX_DIR  := testdata/budoux

PHRASE_MODELS := japanese:ja

phrases:
	mkdir -p $(BUDOUX_DIR)
	curl -sSf -o $(BUDOUX_DIR)/LICENSE $(BUDOUX)/LICENSE
	for pair in $(PHRASE_MODELS); do \
	  name=$${pair%%:*}; file=$${pair##*:}; \
	  curl -sSf -o $(BUDOUX_DIR)/$$file.json $(BUDOUX)/budoux/models/$$file.json; \
	  go run ./cmd/genphrase $$name $(BUDOUX_DIR)/$$file.json $(BUDOUX_DIR)/LICENSE \
	    > paragraph/$$name'phrases.go'; \
	  gofmt -w paragraph/$$name'phrases.go'; \
	done

# Which characters stand upright on a line of vertical text, UAX #50. It is
# what tells a block of English from a block of Japanese, and so which blocks
# this engine can turn on their side. See cmd/genvertical.
#
#	make vertical UCD=/path/to/unpacked/ucd
vertical:
	go run ./cmd/genvertical $(UCD)/VerticalOrientation.txt > paragraph/verticaltable.go
	gofmt -w paragraph/verticaltable.go

# What "text-transform: full-width" and "full-size-kana" remap, both derived
# from UnicodeData.txt. See cmd/genfullwidth and cmd/genfullsizekana.
#
#	make widths UCD=/path/to/unpacked/ucd
widths:
	go run ./cmd/genfullwidth $(UCD)/UnicodeData.txt > paragraph/widthtable.go
	go run ./cmd/genfullsizekana $(UCD)/UnicodeData.txt > paragraph/kanatable.go
	gofmt -w paragraph/widthtable.go paragraph/kanatable.go

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
# The last four are narrow on purpose. Ogham, Coptic, Deseret and the Number
# Forms are what the suite's remaining glyph-missing reports are *for* — eleven
# documents between them, and nothing else in this set has a glyph for any of it.
# They are small because each covers one block and nothing else: Ogham is four
# kilobytes, which is a whole script.
#
# None of them carries Latin, and that only became usable when the fallback
# started working per run rather than per box. A face with no Latin cannot set
# "the ogham space mark ᚛" as a whole, so the whole-box question had no answer
# and the box kept the family's face — which is exactly the shape the Hebrew
# sentence had. See layout/facerun.go.
#
# The Japanese face is the variable TTF and not one of the static OTFs, because
# those are CID-keyed CFF and forme does not read them. forme instantiates it at
# the font's default, which its name table reports as Thin — so CJK set through
# this fallback is lighter than it should be. It is a fallback for text that
# would otherwise be invisible, and the weight being wrong is worth saying out
# loud rather than leaving to be discovered.
# GNU Unifont is the last resort, and it is here for what it *cannot* be asked
# to do as much as for what it can.
#
# It covers fourteen of the sixteen scripts this suite writes in one file, and
# the last two in its upper-plane companion — so with it in the library there is
# no character in the whole corpus that any document sets as a space. That is the
# whole of the gain, and it is a gain in the picture rather than in the count:
# the tests that needed it were passing before, drawing nothing where a character
# belonged.
#
# It is a bitmap font grown into outlines, so its glyphs are on an 8- or 16-pixel
# grid and its advances come in two widths. That is why it is *last*: asked after
# every face that might set the text properly, and reached only where the answer
# would otherwise be a blank. A fallback list is an ordering and this is the
# bottom of it.
#
# It was also what found the fault in the per-box fallback. A face that can set
# almost anything can set almost any *whole paragraph*, so the question "can one
# face set the whole of this text" started finding an answer every time it was
# asked, and eighty-eight documents were reported as substituted that had nothing
# wrong with them. See layout/facerun.go.
#
# Licensing: the compiled fonts are SIL Open Font License 1.1 — unifoundry's
# LICENSE.txt says so in as many words, the GPL covering the build sources rather
# than the fonts — and it is fetched alongside them.
UNIFONT_VER  := 17.0.05
UNIFONT_BASE := https://unifoundry.com/pub/unifont/unifont-$(UNIFONT_VER)/font-builds

NOTO_DIR := testdata/fonts-noto
NOTO_BASE := https://raw.githubusercontent.com/notofonts
NOTO_HINTED := NotoSans NotoSansHebrew NotoSansArabic NotoSansDevanagari \
               NotoSansArmenian NotoSansGeorgian \
               NotoSansOgham NotoSansCoptic NotoSansDeseret NotoSansSymbols

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
	curl -sSf -o $(NOTO_DIR)/Unifont-Regular.otf \
	  $(UNIFONT_BASE)/unifont-$(UNIFONT_VER).otf
	curl -sSf -o $(NOTO_DIR)/UnifontUpper-Regular.otf \
	  $(UNIFONT_BASE)/unifont_upper-$(UNIFONT_VER).otf
	curl -sSf -o $(NOTO_DIR)/UNIFONT-LICENSE.txt https://unifoundry.com/LICENSE.txt
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
            css/CSS2/support css/CSS2/reference css/css-text css/reference \
            css/support \
            fonts images

# "images" is the suite's shared image directory, at the repository root rather
# than under css/. Two documents reach into it — "/images/blue.png" and
# "/images/background.png" — and without it they were laid out with the picture
# missing and reported for it. It is a third of a megabyte for ninety-one files,
# which is the cheapest entry in this list.
#
# It is also what found the corpus pin above. Adding a line here moved the CI
# cache key, which was the only thing holding the suite at a fixed revision, and
# a one-document change came back as a hundred-document regression. The pin is
# what makes this line safe to write.
#
# "css/support" is the suite's *shared* support directory, as against the
# per-chapter css/CSS2/support beside it. It was missing, and the whole of it is
# sixty-one small files, and its absence cost twenty-three clean passes: the
# tests that want it write a root-relative "/css/support/60x60-red.png", every
# one of them resolved to a file that was not there, and every one of them then
# passed *vacuously* — a background image that fails to load paints nothing, and
# the reference beside it painted nothing either.
#
# That is the fifth time a large block of this suite has turned out to be about
# the harness rather than about the engine, and it is the same shape every time:
# the tests were not failing, so nothing was red, and the cost was paid in the
# vacuous bucket where nobody looks. See wptCleanPassBaseline.

# "css/css-text" was "css/css-text/white-space" until the rest of it was
# measured. The engine implements most of what the other directories test —
# line breaking, word-break, text-align, text-transform, letter-spacing,
# text-indent, tab-size, overflow-wrap, and the bidi and shaping that i18n is
# written against — and none of it was being run.
#
# It added 1073 reftests: 327 pass cleanly, 167 pass with something unsupported
# and 579 fail. A 30% clean rate on material the engine was never measured
# against is the number worth recording, because it says the failures are a seam
# rather than a wall: the largest groups are i18n (93), word-break (71), line
# breaking (82 across two directories), text-align (45) and text-transform (41),
# and 382 of the 579 are clean failures — a real difference in the picture rather
# than a document the engine declined to render.
#
# Some of the rest is honestly out of reach and is counted here so that it is not
# rediscovered: hyphens (41) is a property this engine reports unimplemented, and
# text-autospace, hanging-punctuation, word-space-transform and text-fit are CSS
# Text 4 features it has never claimed.

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

wpt: $(WPT_DIR)/.ok $(WPT_DIR)/fonts/DoulosSIL-R.woff \
     $(WPT_DIR)/fonts/NotoSansArmenian-Regular \
     $(WPT_DIR)/fonts/NotoSansGeorgian-Regular.ttf

# The revision the corpus is taken at.
#
# It is pinned because a ratchet has to be measured against a fixed thing. The
# clone was "--depth 1" of a moving branch, so what the suite *was* depended on
# when it happened to be fetched — and the only thing holding it still was a CI
# cache keyed on this file, so any edit here silently swapped the corpus
# underneath the number. That is not a hypothetical: adding one directory to
# WPT_DIRS moved the key, re-cloned six weeks of upstream, and turned a hundred
# clean passes into "a layout regression" that was nothing of the kind.
#
# So the cache is a speed-up now and nothing rests on it: a cold run and a warm
# one check out the same commit and count the same tests.
#
# Moving it is a deliberate act with its own commit, which says what the new
# revision changed and moves the baseline to what it measures. It is not
# something another change gets to do as a side effect.
WPT_COMMIT := a1e944e7a879854494e1a041a8ad1e4a8ae28ab1

$(WPT_DIR)/.ok:
	rm -rf $(WPT_DIR)
	git clone --filter=blob:none --sparse --depth 1 \
		https://github.com/web-platform-tests/wpt.git $(WPT_DIR)
	git -C $(WPT_DIR) sparse-checkout set $(WPT_DIRS)
	git -C $(WPT_DIR) fetch --depth 1 --filter=blob:none origin $(WPT_COMMIT)
	git -C $(WPT_DIR) checkout --detach $(WPT_COMMIT)
	touch $@

# Doulos SIL, which the suite asks for and does not ship.
#
# Sixty-eight of its documents write
# "@font-face { src: url('/fonts/DoulosSIL-R.woff') }", and that file is not in
# the web-platform-tests repository: "git ls-tree HEAD fonts/" has its two
# siblings from the same foundry, GentiumPlus-R.woff and
# Scheherazade-Regular.woff, and not this one. So the tests are written against
# a font the suite lost, and every browser running them falls back exactly as
# this engine does.
#
# It is fetched because the alternative is measuring nothing. The documents are
# text-transform tests — the uppercase of the Greek Extended block, of the
# Latin Extended additions, the case pairs a general-purpose face has no glyphs
# for — and their references write the expected text out in the same font. With
# the font missing, both halves fall back to whatever the library has and
# twenty-nine of them agreed while neither drew what it was asked to. With it,
# the same twenty-nine agree on the picture the test is about. Nothing about the
# engine changed: 5784 clean passes became 5813, the failures did not move, and
# the vacuous bucket shrank by exactly the difference.
#
# Version 5.000's *web* package is fetched rather than the current release
# because that is the file the tests name: SIL ships only a TTF now, and
# "DoulosSIL-R.woff" is the name the 5.000 webfont package gives it.
#
# Licensing: Doulos SIL is under the SIL Open Font License 1.1, and the
# licence travels with it into this gitignored directory. No font bytes are
# vendored in this repository or shipped in anything it builds — the same
# arrangement as Ahem and the Noto faces above.
DOULOS_URL := https://software.sil.org/downloads/r/doulos/DoulosSIL-5.000-web.zip

$(WPT_DIR)/fonts/DoulosSIL-R.woff: $(WPT_DIR)/.ok
	curl -sSf -o $(WPT_DIR)/doulos-web.zip $(DOULOS_URL)
	unzip -o -j -d $(WPT_DIR)/fonts $(WPT_DIR)/doulos-web.zip \
	  'DoulosSIL-5.000-web/web/DoulosSIL-R.woff' \
	  'DoulosSIL-5.000-web/OFL.txt'
	mv $(WPT_DIR)/fonts/OFL.txt $(WPT_DIR)/fonts/DoulosSIL-OFL.txt
	rm -f $(WPT_DIR)/doulos-web.zip

# Two more the suite asks for and does not ship, and which this checkout already
# has: "git ls-tree HEAD fonts/" has neither NotoSansArmenian-Regular nor
# NotoSansGeorgian-Regular.ttf, and fourteen of the text-transform documents
# write an @font-face for one of them.
#
# The same story as Doulos and a shorter fix, because nothing has to be
# fetched. The faces are already in $(NOTO_DIR) — they are in NOTO_HINTED above,
# where they were put for the fallback library — and what was missing was a copy
# of each under the name the suite's @font-face asks for. So this is two "cp"s
# and not a download.
#
# It is worth seven clean passes, 5,815 to 5,822, with the failures unmoved and
# the vacuous bucket shrinking by exactly the difference: seven documents that
# agreed while neither half could set Armenian or Georgian now agree on the
# picture the test is about.
#
# The Armenian one has no extension. That is the suite's spelling — its
# @font-face writes "url('/fonts/NotoSansArmenian-Regular') format('truetype')"
# — and it is copied to the name that is asked for rather than to the name it
# had, because a font is found here by its URL and not by its suffix.
$(WPT_DIR)/fonts/NotoSansArmenian-Regular: $(WPT_DIR)/.ok $(NOTO_DIR)/.ok
	cp $(NOTO_DIR)/NotoSansArmenian-Regular.ttf $@

$(WPT_DIR)/fonts/NotoSansGeorgian-Regular.ttf: $(WPT_DIR)/.ok $(NOTO_DIR)/.ok
	cp $(NOTO_DIR)/NotoSansGeorgian-Regular.ttf $@

# NOTO_FONTS as well as WPT_TESTS, and noto-fonts as well as wpt. The ratchet
# counts what the engine renders with the font library a *caller* supplies, and
# without the Noto faces thirty documents that pass in CI report a missing glyph
# instead. Running this target without them measured 4,594 against a baseline of
# 4,624 and printed "this is a layout regression" — which is the one thing a
# ratchet must never say when it is wrong, because the reading it invites is to
# lower the number.
test-wpt: wpt noto-fonts
	WPT_TESTS=$(abspath $(WPT_DIR)) NOTO_FONTS=$(abspath $(NOTO_DIR)) \
	  go test -v -run TestWPT -count=1 ./layout/

clean-wpt:
	rm -rf $(WPT_DIR)