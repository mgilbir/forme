.PHONY: ucd ms-use-sources clean-ms-use-sources verify-fonts test-corpora charprops linebreak vertical dictionaries casing eastasian phrases hyphens widths shapetables bidi-tables grapheme-tables stdfonts brotli-tables glyphlist dictionary-sources phrase-sources hyphen-sources afm brotli-sources agl css-color-spec test bidi-tests test-bidi clean-bidi-tests hbshaping hbvertical test-hbshaping hbfuzz test-difffuzz useable clean-ucd stdfonts grapheme-tests test-grapheme clean-grapheme-tests normalization-tests test-normalization clean-normalization-tests css-tests test-css clean-css-tests html-entities clean-html-entities css-colors clean-css-colors language-tags clean-language-tags notice-sources clean-notice-sources noto-fonts clean-noto-fonts wpt test-wpt wpt-breakdown clean-wpt varinstance test-varinstance hbenv hboracles hblanguages

# Every go test in this file names its -timeout, and these are the two it names.
#
# go test's own default is ten minutes for each package's test binary, and that
# stopped being a bound with room in it: layout's tests, with every corpus in
# the environment, take about 95 seconds here and about 500 under the race
# detector, which is most of the ten minutes on this machine and more than all
# of it on a runner two or three times slower. A default that trips on a
# slower machine is a flaky gate, and one nobody chose is one nobody knows the
# reason for. So each is written down with the measurement it was chosen from,
# at about ten times what it covers here for the ordinary run and five for the
# race run, which is five times slower to begin with. A package that outgrows
# them is a fact worth hearing about rather than a limit to raise quietly:
# measure it again, and move the number with the reason.
#
# cmd/gotesttimeout_test.go holds every go test here and in .github/workflows
# to naming one.
TEST_TIMEOUT = 15m
RACE_TIMEOUT = 45m

test:
	gofmt -l . | grep -v '^testdata/' && exit 1 || true
	go vet ./...
	go test -count=1 -timeout $(TEST_TIMEOUT) ./...

# The same suite with every corpus in the environment, which is the only way most
# of it runs at all.
#
# "test" above hands `go test` an empty environment, and a test that needs a Noto
# face or the reftest checkout answers that by skipping. Most of the suite does —
# every test that loads a fallback face, every reftest, the two colour oracles —
# and only two were reached by anything else the gate ran. They passed; nothing
# was checking that they still did, which is the same as not having them.
#
# The count that stood here was written once and never remeasured, which is the
# fault this whole paragraph is about; it is left out rather than replaced with a
# number that will be wrong again.
#
# So this is where they run. It fetches what each corpus needs first, because a
# target that quietly skips is the thing it was written to stop — and it names
# every one of them, because a variable that is set and wrong is now a failure
# rather than a skip, which is only worth having if the variable is set.
#
# notocjk and ucd were missing from both lists. The CID-keyed CFF tests read a
# real CJK face and nothing fetched one, so they skipped here and in CI under a
# step whose own comment says a skip there is worse than a failure; and the
# generator tables are checked by regenerating them, which needs the database.
CORPUS_ENV = \
	WPT_TESTS="$(abspath $(WPT_DIR))" \
	NOTO_FONTS="$(abspath $(NOTO_DIR))" \
	NOTO_CJK="$(abspath $(CJK_DIR))" \
	CSS_PARSING_TESTS="$(abspath $(CSS_TESTS_DIR))" \
	UNICODE_BIDI_TESTS="$(abspath $(BIDI_DIR))" \
	UNICODE_GRAPHEME_TESTS="$(abspath $(GRAPHEME_DIR))" \
	UNICODE_NORMALIZATION_TESTS="$(abspath $(NORMALIZATION_DIR))" \
	TABLE_INPUTS=required

# The inputs of every generated table are corpora too: cmd/regenerate_test.go
# regenerates each table from them and compares, and with TABLE_INPUTS=required
# above, a table whose inputs are not here is a failure rather than a skip.
CORPORA = wpt noto-fonts notocjk ucd css-tests bidi-tests grapheme-tests \
	normalization-tests $(HTML_ENTITIES) $(TABLE_SOURCES) notice-sources

test-corpora:
	$(MAKE) verify-fonts
	$(CORPUS_ENV) go test -count=1 -timeout $(TEST_TIMEOUT) ./...

# The same suite under the race detector.
#
# Separate from "test" because it is five times slower and the thing it looks for
# does not change with a line of layout code — what it watches is whether two
# documents laid out at once share anything, and the answer only moves when
# something becomes shared on purpose. Worth running when that might have
# happened: a new memo, a package-level var, a font set that caches.
.PHONY: race

# With every corpus in the environment, for the same reason test-corpora has
# them: an empty environment leaves the font sets, the shared block-glyph
# registry and every document that loads an @font-face out of the run, and those
# are the shared things the detector is here to watch. It ran `go test -race`
# over the tests that need nothing fetched, which are the ones that share
# nothing.
race:
	$(CORPUS_ENV) go test -count=1 -race -timeout $(RACE_TIMEOUT) ./...

# Every fetch in this file goes through FETCH rather than through a bare curl.
#
# The corpora come from a dozen hosts and one of them is always the slow one.
# unifoundry.com served GNU Unifont from a single machine with no CDN in front
# of it, and a CI run failed on it with "Failed to connect after 132634 ms" —
# which stopped the build before a line of the engine had run, and said nothing
# whatever about the change under test. A run that could not reach a web server
# is not a result. Unifont now comes from GNU's own mirror network; the retries
# below are still what every other host gets.
#
# --retry-all-errors rather than --retry, because the two are not the same:
# plain --retry covers a transient HTTP status and a handful of network errors,
# and the one that actually happened — a connection that never opened — is not
# among them. --connect-timeout bounds the wait before the first retry, since
# the default is over two minutes and five of those is longer than the job.
#
# -f so that an HTTP error is a failure rather than an error page written into
# the output file, -sS so that a normal run is quiet and a broken one is not,
# and -L because some of the sources below redirect.
FETCH := curl -fsSL --connect-timeout 20 --retry 5 --retry-delay 3 --retry-all-errors

# Every fetched set below is marked done by a stamp file, and the stamp is named
# for a digest of everything the set is fetched from: the release or commit it
# is pinned to, the URL, and the list of files. make fetches a set when its
# stamp does not exist, so a stamp named for less than all of that is a set
# that is not fetched again when the rest of it changes.
#
# The Unicode database's stamp was ".ok" and nothing more. ScriptExtensions.txt
# was added to UCD_FILES, and a checkout that already had the stamp never
# fetched it: `make ucd` had nothing to do, and the generator that reads the
# file failed on a file that was not there. The same was true of the Unicode
# version the three conformance sets and the database are fetched at, of the
# reftest corpus's commit and of its directory list, of the CSS parsing tests'
# commit, and of the files of the word lists, the hyphenation patterns and the
# AFM set, whose stamps were named for their commit but not their list.
#
# So a change to any of them names a stamp that does not exist yet. What a
# recipe fetches has to be named by the variables in its key: a file written
# into a recipe rather than into its list is the same fault again.
# cmd/makefile_test.go asks make that every stamp moves when each of its
# variables does, and that every stamp in this file is one it asks about.
#
#	$(call stamp,<directory>,<everything the set is fetched from>)
stamp = $(1)/.ok-$(call digest,$(2))
digest = $(or $(shell printf '%s' '$(strip $(1))' | sha256sum | cut -c1-16),\
	$(error sha256sum is needed to name the fetch stamps))

# Unicode's own bidirectional conformance suites, which bidi_conformance_test.go
# runs in full. Fetched rather than vendored: 15 MB, versioned by Unicode, and
# pinned to the release the tables were generated from — a character whose class
# changed between releases is a stale table rather than a defect.
BIDI_DIR := testdata/unicode-bidi
UNICODE_VERSION ?= 17.0.0
UCD_URL         := https://www.unicode.org/Public/$(UNICODE_VERSION)/ucd
BIDI_FILES      := BidiTest.txt BidiCharacterTest.txt
BIDI_STAMP      := $(call stamp,$(BIDI_DIR),$(UCD_URL) $(BIDI_FILES))

bidi-tests: $(BIDI_STAMP)

$(BIDI_STAMP):
	mkdir -p $(BIDI_DIR)
	for f in $(BIDI_FILES); do \
	  $(call fetch,$(BIDI_DIR)/$$f,$(UCD_URL)/$$f) || exit 1; \
	done
	touch $@

test-bidi: bidi-tests
	UNICODE_BIDI_TESTS=$(abspath $(BIDI_DIR)) go test -v -run TestBidiConformance -count=1 -timeout $(TEST_TIMEOUT) ./bidi

clean-bidi-tests:
	rm -rf $(BIDI_DIR)

# Shaping checked against HarfBuzz, over six fonts. See testdata/harfbuzz.
#
#	make hbenv
#	PYTHON=.hbenv/bin/python make hboracles
#
# The oracle is one HarfBuzz release, HARFBUZZ_VERSION, taken through the
# uharfbuzz release that carries it: testdata/harfbuzz/requirements.txt pins it
# by version and by digest, and pip refuses anything else. An unpinned
# `pip install uharfbuzz` is whatever PyPI serves that day, and it had left the
# expectation files at three releases. The generators refuse another release
# (testdata/harfbuzz/oracle.py), and shape/oraclepin_test.go refuses a file
# that records one.
HARFBUZZ_DIR := testdata/harfbuzz
PYTHON ?= python3
HBENV := .hbenv

hbenv:
	rm -rf $(HBENV)
	python3 -m venv $(HBENV)
	$(HBENV)/bin/pip install --require-hashes --no-deps -r $(HARFBUZZ_DIR)/requirements.txt
	$(HBENV)/bin/python -c 'import sys; sys.path.insert(0, "$(HARFBUZZ_DIR)"); \
		import oracle; oracle.harfbuzz(); oracle.fonttools()'

# Every file the oracles write through uharfbuzz. hblanguages is below, where
# the language-tag header it reads has been defined. The oracle files this
# leaves out — usecategories.expected.txt, usescripts.expected.txt and
# indiccategories.expected.txt — are read from a source checkout of the same
# release: HarfBuzz's own generators, and its own source; see
# usecategories.py, usescripts.py and indiccategories.py.
hboracles: hbshaping hbvertical hblanguages varinstance

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
	$(PYTHON) $(HARFBUZZ_DIR)/shapefeatures.py fonts/notosans/NotoSans-Variable.ttf \
		$(HARFBUZZ_DIR)/features.txt $(HARFBUZZ_DIR)/features.expected.txt
	$(PYTHON) $(HARFBUZZ_DIR)/shape.py $(HARFBUZZ_DIR)/fonts/NotoSansJavanese.ttf \
		$(HARFBUZZ_DIR)/javanese.txt $(HARFBUZZ_DIR)/javanese.dflt.expected.txt und-x-hbscdflt
	$(PYTHON) $(HARFBUZZ_DIR)/shape.py $(HARFBUZZ_DIR)/fonts/NotoSansBalinese.ttf \
		$(HARFBUZZ_DIR)/balinese.txt $(HARFBUZZ_DIR)/balinese.dflt.expected.txt und-x-hbscdflt
	$(PYTHON) $(HARFBUZZ_DIR)/shape.py $(HARFBUZZ_DIR)/fonts/NotoSerifTibetan.ttf \
		$(HARFBUZZ_DIR)/tibetan.txt $(HARFBUZZ_DIR)/tibetan.dflt.expected.txt und-x-hbscdflt

# A run set upright, over nine faces that answer where a glyph is hung and
# how far it moves the pen in different ways — see vertical.py for which is
# which, and vertical_fixture.py for the three that are built here. Four of them are in the corpora, so this needs `make notocjk
# noto-fonts` first; the expectations are checked in, and the Go test reads
# the faces from the same place.
hbvertical:
	$(PYTHON) $(HARFBUZZ_DIR)/vertical_fixture.py $(HARFBUZZ_DIR)/fonts
	$(PYTHON) $(HARFBUZZ_DIR)/vertical.py $(HARFBUZZ_DIR)/vertical.txt $(HARFBUZZ_DIR)/vertical_features.txt \
		$(HARFBUZZ_DIR)/vertical.expected.txt \
		NotoSans-Variable.ttf=fonts/notosans/NotoSans-Variable.ttf \
		NotoSansArabic.ttf=$(HARFBUZZ_DIR)/fonts/NotoSansArabic.ttf \
		NotoSansJP-Regular.otf=$(CJK_DIR)/NotoSansJP-Regular.otf \
		NotoSansJP-VF.ttf=$(NOTO_DIR)/NotoSansJP-VF.ttf \
		ipag.ttf=$(NOTO_DIR)/ipag.ttf \
		Unifont-Regular.otf=$(NOTO_DIR)/Unifont-Regular.otf \
		VerticalComposites.ttf=$(HARFBUZZ_DIR)/fonts/VerticalComposites.ttf \
		VerticalFallbacks.ttf=$(HARFBUZZ_DIR)/fonts/VerticalFallbacks.ttf \
		VerticalHhea.ttf=$(HARFBUZZ_DIR)/fonts/VerticalHhea.ttf

test-hbshaping:
	go test -v -run 'TestShapingAgreesWithHarfBuzz|TestTheHarfBuzzOracleHasTeeth|TestFeatureShapingAgreesWithHarfBuzz|TestTheFeatureOracleHasTeeth|TestTheDefaultModelAgreesWithHarfBuzz|TestUprightShapingAgreesWithHarfBuzz|TestSidewaysRunsAreShapedAsBefore|TestVerticalMetricsAgreeWithHarfBuzz|TestTheVerticalOracleHasTeeth' -count=1 -timeout $(TEST_TIMEOUT) ./shape

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
	go test -v -run 'TestInstancingAgreesWithFontToolsAndHarfBuzz|TestTheInstancingOracleHasTeeth' -count=1 -timeout $(TEST_TIMEOUT) ./shape

# Differential fuzzing against HarfBuzz. Needs the same Python as hbshaping.
hbfuzz:
	go build -o $(HARFBUZZ_DIR)/.shapetext ./cmd/shapetext
	SHAPETEXT=$(abspath $(HARFBUZZ_DIR)/.shapetext) $(PYTHON) $(HARFBUZZ_DIR)/difffuzz.py 60

# The fuzzer's classifier, on its own.
#
# It is the part of difffuzz with a decision in it: a difference it names is a
# difference nobody is ever shown, so a class that quietly grew to cover
# something new would hide the next defect rather than report it. This needs
# neither uharfbuzz nor fontTools — the imports for those are where they are used
# — so it runs anywhere python3 does, and it runs in CI.
test-difffuzz:
	$(PYTHON) $(HARFBUZZ_DIR)/difffuzz.py --self-test

# The Universal Shaping Engine's category table, derived from Unicode's own
# property files plus the engine's corrections. See cmd/genuse.
#
#	make useable UCD=/path/to/unpacked/ucd
#
# UCD_DIR is where this file would put one; UCD is where a generator reads one
# from, and is the same place unless a caller says otherwise. The two are
# separate so that clean-ucd can refuse to remove a directory it did not make.
UCD_DIR := testdata/ucd
UCD ?= $(UCD_DIR)

# The database itself, which every generator below reads and nothing fetched.
#
# .gitignore said testdata/ucd was "fetched into place for `make useable`" and
# no target put anything there: nine generator targets each documented as
# `make thing UCD=/path/to/unpacked/ucd`, and finding unicode.org and unpacking
# a zip was left to the reader. It was the one corpus in this file that had to
# be done by hand, and the reason clean-ucd guarded a directory nothing made.
#
# Three of those nine could not have run against a database at all — the
# argument lists had drifted, and nothing was in a position to notice. See
# cmd/regenerate_test.go, which now runs every one of them.
#
# The twenty-one files that are read, rather than UCD.zip: the archive is an
# order of magnitude larger than the files taken from it, unzip is one more
# thing to have installed, and a file that moves in a new release fails here by
# name instead of as a "no such file" from inside a generator.
#
# One of them is read by a test rather than a generator: LineBreakTest.txt is
# UAX #14's conformance suite, and paragraph/linebreakconformance_test.go runs
# every case of it through the line breaker. It is here rather than in a set of
# its own, as GraphemeBreakTest.txt is, because it is held to the release
# linebreaktable.go was generated from, and that release is this set's.
#
# The layout is the database's own, subdirectories and all, so that a caller who
# already has one unpacked can point UCD at it and every target works.
UCD_FILES := \
	ArabicShaping.txt \
	BidiBrackets.txt \
	BidiMirroring.txt \
	CaseFolding.txt \
	CompositionExclusions.txt \
	DerivedCoreProperties.txt \
	EastAsianWidth.txt \
	IndicPositionalCategory.txt \
	IndicSyllabicCategory.txt \
	LineBreak.txt \
	PropList.txt \
	PropertyValueAliases.txt \
	ScriptExtensions.txt \
	Scripts.txt \
	SpecialCasing.txt \
	UnicodeData.txt \
	VerticalOrientation.txt \
	auxiliary/GraphemeBreakProperty.txt \
	auxiliary/LineBreakTest.txt \
	emoji/emoji-data.txt \
	extracted/DerivedBidiClass.txt

UCD_STAMP := $(call stamp,$(UCD_DIR),$(UCD_URL) $(UCD_FILES))

ucd: $(UCD_STAMP)

$(UCD_STAMP):
	@for f in $(UCD_FILES); do \
	  mkdir -p $(UCD_DIR)/$$(dirname $$f); \
	  $(call fetch,$(UCD_DIR)/$$f,$(UCD_URL)/$$f) || exit 1; \
	done
	@echo "Unicode $(UNICODE_VERSION) in $(UCD_DIR)"
	touch $@

# A generator reads the fetched database only when it is the fetched one. A
# caller who passed UCD= has their own, and fetching over the top of it would be
# this file taking a decision that is theirs.
ifeq ($(UCD),$(UCD_DIR))
UCD_DEP := $(UCD_STAMP)
else
UCD_DEP :=
endif

# The Universal Shaping Engine's corrections to two of the database's
# properties, and the script development specifications' list of invalid vowel
# clusters: three files HarfBuzz keeps in src/ms-use, which cmd/genuse and
# cmd/genvowel read. See testdata/ms-use/NOTICE.md.
#
# They were committed, taken from HarfBuzz at a commit nobody recorded, and two
# of the three had drifted from any release anyone could name. They are
# fetched now at HARFBUZZ_VERSION — the release the shaping oracle runs and the
# language-tag table is taken from — and each is held to its SHA-256, which is
# part of the stamp's key, so a new digest fetches again.
#
#	<file>:<sha256>
HARFBUZZ_VERSION := 14.5.0
MSUSE_URL := https://raw.githubusercontent.com/harfbuzz/harfbuzz/$(HARFBUZZ_VERSION)/src/ms-use
MSUSE_DIR := testdata/ms-use
MSUSE_FILES := \
	IndicPositionalCategory-Additional.txt:2baa1c1efe5a5f108c304b1e27d0d97864c806764eb2b0a1bd91db80ae26b5b8 \
	IndicShapingInvalidCluster.txt:02024d4289864665721e14ec99eb320ce187f289514b793449b4f6a8ddaf5944 \
	IndicSyllabicCategory-Additional.txt:b9472e3e72d5fba8cb3f2e0578e73012aaae786db25779de0f1a5a5ab69b84a6
MSUSE_STAMP := $(call stamp,$(MSUSE_DIR),$(MSUSE_URL) $(MSUSE_FILES))

ms-use-sources: $(MSUSE_STAMP)

$(MSUSE_STAMP):
	mkdir -p $(MSUSE_DIR)
	for e in $(foreach f,$(MSUSE_FILES),'$(f)'); do \
	  f=$${e%%:*}; sum=$${e#*:}; \
	  $(FETCH) -o $(MSUSE_DIR)/$$f.part $(MSUSE_URL)/$$f || exit 1; \
	  echo "$$sum  $(MSUSE_DIR)/$$f.part" | sha256sum -c --quiet - || { \
	    rm -f $(MSUSE_DIR)/$$f.part; \
	    echo "$(MSUSE_URL)/$$f is not the file MSUSE_FILES pins" >&2; \
	    exit 1; \
	  }; \
	  mv $(MSUSE_DIR)/$$f.part $(MSUSE_DIR)/$$f; \
	done
	touch $@

clean-ms-use-sources:
	rm -f $(MSUSE_DIR)/*.txt $(MSUSE_DIR)/.ok-*

# Every generated table in this repository, and how a target regenerates one.
#
# A recipe here used to be "go run ./cmd/genX ... > table.go", and that shape
# had three faults. The redirection emptied the committed table before the
# generator ran, so a generator that failed left a file under version control
# empty and the build broken. The loops over several tables had no "set -e",
# so a failed fetch truncated one table, went on to the next, and reported
# success. And cmd/regenerate_test.go found the recipes by matching these
# lines, so a recipe it could not expand dropped out of the check in silence.
#
# So what each table is made from lives in one place, cmd/internal/tables, and
# every target below runs it through cmd/maketables, which generates every
# table the target names before writing any, replaces each by renaming a whole
# file over it, and exits non-zero having written nothing when anything fails.
# The drift test runs the same list through the same code.
#
# TABLE_VARS are the variables the list refers to, and they are passed by name:
# a variable the list names and this does not pass is a failure in both places,
# not an empty argument.
TABLE_VARS := UCD UNICODE_VERSION \
	ICU_DICTS DICT_DIR BUDOUX BUDOUX_DIR HYPHEN_URL HYPHEN_DIR \
	AFM_URL AFM_DIR BROTLI_URL BROTLI_DIR AGL_URL AGL_DIR \
	HTML_ENTITIES HTML_ENTITIES_URL HTML_ENTITIES_SHA256 CSS_COLOR_URL CSS_COLOR_SPEC \
	HB_LANGTAGS HB_LANGTAGS_URL HB_LANGTAGS_SHA256 HB_COPYING HB_COPYING_URL HB_COPYING_SHA256 \
	MSUSE_URL MSUSE_DIR MPL MPL_URL MPL_SHA256
MAKETABLES = go run ./cmd/maketables $(foreach v,$(TABLE_VARS),-D '$(v)=$($(v))')

# Every input a generator reads that is fetched rather than committed. Each is
# taken at a pinned commit or digest, named below beside the URL it pins, and
# each generated table records the pin — so the table can be made again next
# year from what it was made from, rather than from what an upstream branch
# happens to hold. Moving a pin is a change of its own, which regenerates the
# tables it feeds and says what changed in them.
#
# Each fetch lands in a ".part" file that is renamed into place only when it is
# whole, and each set is marked done by a stamp named for its pin and its files
# (see stamp), so a new pin or a new file fetches again rather than finding the
# old files and calling them current.
TABLE_SOURCES = $(UCD_DEP) ms-use-sources dictionary-sources phrase-sources hyphen-sources $(MPL) \
	afm brotli-sources agl css-color-spec $(HTML_ENTITIES) $(HB_LANGTAGS) $(HB_COPYING)

# One file, whole or not at all.
#
#	$(call fetch,<destination>,<url>)
define fetch
	$(FETCH) -o $(1).part $(2) && mv $(1).part $(1)
endef

# One file, whole, and the file its pin names or not at all. What a URL serves
# is checked against the SHA-256 written beside it before it is renamed into
# place, so a source that moves under its URL is a fetch that fails rather than
# a corpus that changed. It is a shell command, usable inside a loop with shell
# variables for its arguments, and it fails as one.
#
#	$(call pinned,<destination>,<url>,<sha256>)
define pinned
	{ $(FETCH) -o $(1).part $(2) && \
	  { echo "$(3)  $(1).part" | sha256sum -c --quiet - || \
	    { rm -f $(1).part; echo "$(2) is not the file its pin names, SHA-256 $(3)" >&2; false; }; } && \
	  mv $(1).part $(1); }
endef

# The tables the shaper derives from Unicode, which cmd/genuse's table above is
# only one of. Each was runnable and none was wired up, so the only way to
# regenerate one was to read its usage line — which named a directory this
# repository did not have until `make ucd` fetched one.
#
#	make shapetables                              # against the fetched database
#	make shapetables UCD=/path/to/unpacked/ucd    # against one you already have
shapetables: $(UCD_DEP) ms-use-sources
	$(MAKETABLES) shapetables

# The bidirectional character properties, UAX #9. See cmd/genbidi.
#
#	make bidi-tables UCD=/path/to/unpacked/ucd
bidi-tables: $(UCD_DEP)
	$(MAKETABLES) bidi-tables

# The grapheme cluster properties, UAX #29. See cmd/gensegment.
#
#	make grapheme-tables UCD=/path/to/unpacked/ucd
grapheme-tables: $(UCD_DEP)
	$(MAKETABLES) grapheme-tables

# The characters a line may not begin with, from Unicode's line-breaking
# property. See cmd/genlinebreak for which of UAX #14's rules are in it.
#
#	make linebreak UCD=/path/to/unpacked/ucd
linebreak: $(UCD_DEP)
	$(MAKETABLES) linebreak

# Unicode's case mappings, simple and full, and its full case folding, from the
# release UNICODE_VERSION names — not Go's, which are the release the toolchain
# shipped. See cmd/gencasing.
#
#	make casing UCD=/path/to/unpacked/ucd
casing: $(UCD_DEP)
	$(MAKETABLES) casing

# The two properties CSS Text's segment break transformation reads: East Asian
# Width, and which characters are Hangul. See cmd/geneastasian.
#
#	make eastasian UCD=/path/to/unpacked/ucd
eastasian: $(UCD_DEP)
	$(MAKETABLES) eastasian

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
# ICU is taken at the release-78.1 tag's commit. Adding a language is an entry
# in cmd/internal/tables and its file here.
#
#	make dictionaries
ICU_COMMIT := 049e0d6a420629ac7db77256987d083a563287b5
ICU_DICTS  := https://raw.githubusercontent.com/unicode-org/icu/$(ICU_COMMIT)/icu4c/source/data/brkitr/dictionaries
DICT_DIR   := testdata/icu-dictionaries
ICU_DICT_FILES := thaidict.txt laodict.txt khmerdict.txt burmesedict.txt

DICT_STAMP := $(call stamp,$(DICT_DIR),$(ICU_DICTS) $(ICU_DICT_FILES))

dictionary-sources: $(DICT_STAMP)

$(DICT_STAMP):
	mkdir -p $(DICT_DIR)
	for f in $(ICU_DICT_FILES); do \
	  $(call fetch,$(DICT_DIR)/$$f,$(ICU_DICTS)/$$f) || exit 1; \
	done
	touch $@

dictionaries: dictionary-sources
	$(MAKETABLES) dictionaries

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
# knows. Adding one is an entry in cmd/internal/tables and its model here.
#
#	make phrases
BUDOUX_COMMIT := b02ea07cdd6622af8e3bd4da59bc5f0e60eb6bbe
BUDOUX        := https://raw.githubusercontent.com/google/budoux/$(BUDOUX_COMMIT)
BUDOUX_DIR    := testdata/budoux
# Each is saved under its base name: ja.json, not budoux/models/ja.json.
BUDOUX_FILES  := LICENSE budoux/models/ja.json
BUDOUX_STAMP  := $(call stamp,$(BUDOUX_DIR),$(BUDOUX) $(BUDOUX_FILES))

phrase-sources: $(BUDOUX_STAMP)

$(BUDOUX_STAMP):
	mkdir -p $(BUDOUX_DIR)
	for f in $(BUDOUX_FILES); do \
	  $(call fetch,$(BUDOUX_DIR)/$$(basename $$f),$(BUDOUX)/$$f) || exit 1; \
	done
	touch $@

phrases: phrase-sources
	$(MAKETABLES) phrases

# Where a word may be divided when the document has not said, which is what
# "hyphens: auto" asks for.
#
# hyph-utf8, which is TeX's, which is Liang's, which is every typesetting
# system's. As with the models above the pattern file is fetched and the
# *generated* table is what is committed, and the licence travels into it:
# cmd/genhyphen copies the file's header entire, because the terms differ from
# language to language and a table shipped without them is a table nobody may
# ship.
#
# Four languages, and each is a table checked in — Hungarian's alone is half a
# megabyte, which is what a hyphenation dictionary costs when it is patterns
# rather than words. They are the four the suite asks for by name; adding a
# fifth is an entry in cmd/internal/tables, its file here, and a line in
# paragraph/hyphenate.go's hyphenSources.
#
# Each entry carries the Go identifier and the key paragraph.HyphenationOf
# resolves a lang attribute to. They differ for pinyin, whose key carries the
# script: "zh-Latn" is Mandarin in the Latin alphabet and "zh" is Han, and only
# the first of the two has syllables to divide between.
#
#	make hyphens
TEX_HYPHEN_COMMIT := 5684c0f51c0b81133db2efbe60a408b4155a3ff5
HYPHEN_URL := https://raw.githubusercontent.com/hyphenation/tex-hyphen/$(TEX_HYPHEN_COMMIT)/hyph-utf8/tex/generic/hyph-utf8/patterns/tex
HYPHEN_DIR := testdata/hyphen
HYPHEN_FILES := hyph-en-us.tex hyph-nl.tex hyph-hu.tex hyph-zh-latn-pinyin.tex

HYPHEN_STAMP := $(call stamp,$(HYPHEN_DIR),$(HYPHEN_URL) $(HYPHEN_FILES))

hyphen-sources: $(HYPHEN_STAMP)

$(HYPHEN_STAMP):
	mkdir -p $(HYPHEN_DIR)
	for f in $(HYPHEN_FILES); do \
	  $(call fetch,$(HYPHEN_DIR)/$$f,$(HYPHEN_URL)/$$f) || exit 1; \
	done
	touch $@

# The Mozilla Public License 1.1, under which this repository takes the
# Hungarian patterns — hyph-hu.tex offers MPL 1.1, GPL 2.0 or LGPL 2.1 at the
# recipient's option. The licence asks for its Exhibit A notice in each file of
# the Covered Code, and cmd/genhyphen writes it into the table from this text.
# mozilla.org publishes it at no versioned URL, so the digest is the pin: a
# changed text is a fetch that fails, which is the moment to read it.
MPL_URL := https://www.mozilla.org/media/MPL/1.1/index.txt
MPL_SHA256 := f849fc26a7a99981611a3a370e83078deb617d12a45776d6c4cada4d338be469
MPL := testdata/notices/MPL-1.1.txt

$(MPL):
	mkdir -p $(dir $@)
	$(FETCH) -o $@.part $(MPL_URL)
	echo "$(MPL_SHA256)  $@.part" | sha256sum -c --quiet - || { \
	  rm -f $@.part; \
	  echo "$(MPL_URL) is not the file MPL_SHA256 pins" >&2; \
	  exit 1; \
	}
	mv $@.part $@

hyphens: hyphen-sources $(MPL)
	$(MAKETABLES) hyphens

# Which characters stand upright on a line of vertical text, UAX #50. It is
# what tells a block of English from a block of Japanese, and so which blocks
# this engine can turn on their side. See cmd/genvertical.
#
#	make vertical UCD=/path/to/unpacked/ucd
vertical: $(UCD_DEP)
	$(MAKETABLES) vertical

# What "text-transform: full-width" and "full-size-kana" remap, both derived
# from UnicodeData.txt. See cmd/genfullwidth and cmd/genfullsizekana.
#
#	make widths UCD=/path/to/unpacked/ucd
widths: $(UCD_DEP)
	$(MAKETABLES) widths

useable: $(UCD_DEP) ms-use-sources
	$(MAKETABLES) useable

# The character properties the engine asks of a character that no table above
# answers: General_Category, White_Space, Soft_Dotted, Cased and
# Case_Ignorable. They were Go's package unicode, which is the release the
# toolchain shipped rather than this one. See cmd/gencharprop.
#
#	make charprops UCD=/path/to/unpacked/ucd
charprops: $(UCD_DEP)
	$(MAKETABLES) charprops

# Only the directory this file fetches into. "make clean-ucd UCD=/path/to/ucd"
# is the documented way to run a generator against a copy someone already has,
# and the same variable removing it recursively is a way to lose a directory
# that was never ours to remove.
clean-ucd:
	@if [ "$(UCD)" != "$(UCD_DIR)" ]; then \
	  echo "clean-ucd removes $(UCD_DIR) and nothing else; UCD is $(UCD)" >&2; \
	  exit 1; \
	fi
	rm -rf $(UCD_DIR)

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
#
# Both are taken at a commit, like every other corpus here. They were taken
# from each repository's main branch, so what a sweep counted depended on the
# day the library was fetched: Google's families are republished every week,
# and a count of shaping differences over them is a count over whichever fonts
# the branch held that morning. GF_COMMIT is the commit the sweeps quoted in
# the shaping commits were run over. NOTO_CJK_COMMIT is noto-cjk's, which the
# fallback library below also takes a face and a licence from, and it is the
# commit whose files are byte for byte the ones those runs read.
GF_DIR := testdata/googlefonts
GF_COMMIT := 352f6b7d9d6cc4fa9e242b931291d31b21a6dc84
CJK_DIR := testdata/notocjk
NOTO_CJK_COMMIT := f8d157532fbfaeda587e826d4cd5b21a49186f7c
NOTO_CJK_URL := https://raw.githubusercontent.com/notofonts/noto-cjk/$(NOTO_CJK_COMMIT)

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
	git -C $(GF_DIR) fetch origin $(GF_COMMIT)
	git -C $(GF_DIR) sparse-checkout set --no-cone '/ofl/**/*.ttf'
	git -C $(GF_DIR) checkout -f --detach $(GF_COMMIT)

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
#
# Each face is held to its SHA-256, and the set is marked done by a stamp keyed
# on the commit and the list, as every fetched set is. It used to be marked by
# nothing: a face already on disk was kept whatever it was, so a checkout never
# learned that the branch it came from had moved.
#
#	<path in noto-cjk>:<sha256>
CJK_FACES := \
	Sans/SubsetOTF/JP/NotoSansJP-Regular.otf:dff723ba59d57d136764a04b9b2d03205544f7cd785a711442d6d2d085ac5073 \
	Sans/SubsetOTF/KR/NotoSansKR-Regular.otf:69975a0ac8472717870aefeab0a4d52739308d90856b9955313b2ad5e0148d68 \
	Sans/SubsetOTF/SC/NotoSansSC-Regular.otf:faa6c9df652116dde789d351359f3d7e5d2285a2b2a1f04a2d7244df706d5ea9 \
	Sans/SubsetOTF/TC/NotoSansTC-Regular.otf:5bab0cb3c1cf89dde07c4a95a4054b195afbcfe784d69d75c340780712237537 \
	Sans/SubsetOTF/HK/NotoSansHK-Regular.otf:8a43afea92bb58dfd9027bd7ac6f5b0b2662e2ffb3e7c1edc02c62b2b21924f1 \
	Serif/SubsetOTF/JP/NotoSerifJP-Regular.otf:2c9a12dbd4f2408c4610c7ee84a108b62d7236c3775baed618c64d9cb44b2f04
CJK_STAMP := $(call stamp,$(CJK_DIR),$(NOTO_CJK_URL) $(CJK_FACES))

notocjk: $(CJK_STAMP)

$(CJK_STAMP):
	mkdir -p $(CJK_DIR)
	for e in $(foreach f,$(CJK_FACES),'$(f)'); do \
	  p=$${e%%:*}; sum=$${e#*:}; \
	  $(call pinned,$(CJK_DIR)/$$(basename $$p),$(NOTO_CJK_URL)/$$p,$$sum) || exit 1; \
	done
	@echo "$$(ls $(CJK_DIR)/*.otf | wc -l | tr -d ' ') CJK faces in $(CJK_DIR)"
	touch $@

fontsweep:
	go run ./cmd/fontsweep $(GF_DIR)/ofl $(CJK_DIR)

clean-fonts:
	rm -rf $(GF_DIR) $(CJK_DIR)

# The metrics of the fourteen standard PDF faces, from Adobe's own AFM files.
#
# The AFM set is freely redistributable and ships with a good deal of software
# — Ghostscript, matplotlib, poppler-data — but is not vendored here, because
# only the numbers are wanted and none of the files are redistributed. This
# used to be pointed at a directory the developer had found; nothing fetched
# one. matplotlib's copies, with the readme that is their licence, are taken at
# its v3.10.0 tag's commit, and reproduce the committed table byte for byte.
#
#	make stdfonts
MATPLOTLIB_COMMIT := 8d64f03a1f501ba0019279bf2f8db3930d1fe33f
AFM_URL := https://raw.githubusercontent.com/matplotlib/matplotlib/$(MATPLOTLIB_COMMIT)/lib/matplotlib/mpl-data/fonts/pdfcorefonts
AFM_DIR := testdata/afm
AFM_FILES := Courier.afm Courier-Bold.afm Courier-Oblique.afm Courier-BoldOblique.afm \
	Helvetica.afm Helvetica-Bold.afm Helvetica-Oblique.afm Helvetica-BoldOblique.afm \
	Times-Roman.afm Times-Bold.afm Times-Italic.afm Times-BoldItalic.afm \
	Symbol.afm ZapfDingbats.afm readme.txt

AFM_STAMP := $(call stamp,$(AFM_DIR),$(AFM_URL) $(AFM_FILES))

afm: $(AFM_STAMP)

$(AFM_STAMP):
	mkdir -p $(AFM_DIR)
	for f in $(AFM_FILES); do \
	  $(call fetch,$(AFM_DIR)/$$f,$(AFM_URL)/$$f) || exit 1; \
	done
	touch $@

stdfonts: afm
	$(MAKETABLES) stdfonts

# The two tables a Brotli decoder cannot compute, read from the reference
# implementation's own source at its v1.1.0 tag's commit. See cmd/genbrotli.
#
#	make brotli-tables
BROTLI_COMMIT := ed738e842d2fbdf2d6459e39267a633c4a9b2f5d
BROTLI_URL := https://raw.githubusercontent.com/google/brotli/$(BROTLI_COMMIT)/c/common
BROTLI_DIR := testdata/brotli-source
BROTLI_FILES := context.c transform.c
BROTLI_STAMP := $(call stamp,$(BROTLI_DIR),$(BROTLI_URL) $(BROTLI_FILES))

brotli-sources: $(BROTLI_STAMP)

$(BROTLI_STAMP):
	mkdir -p $(BROTLI_DIR)
	for f in $(BROTLI_FILES); do \
	  $(call fetch,$(BROTLI_DIR)/$$f,$(BROTLI_URL)/$$f) || exit 1; \
	done
	touch $@

brotli-tables: brotli-sources
	$(MAKETABLES) brotli-tables

# The glyph names the standard Latin encodings use, from Adobe's Glyph List.
# See cmd/genglyphlist.
#
#	make glyphlist
AGL_COMMIT := 4036a9ca80a62f64f9de4f7321a9a045ad0ecfd6
AGL_URL := https://raw.githubusercontent.com/adobe-type-tools/agl-aglfn/$(AGL_COMMIT)/glyphlist.txt
AGL_DIR := testdata/agl

AGL_STAMP := $(call stamp,$(AGL_DIR),$(AGL_URL))

agl: $(AGL_STAMP)

$(AGL_STAMP):
	mkdir -p $(AGL_DIR)
	$(call fetch,$(AGL_DIR)/glyphlist.txt,$(AGL_URL))
	touch $@

glyphlist: agl
	$(MAKETABLES) glyphlist

# UAX #29's grapheme cluster boundaries, which package segment finds.
#
# GraphemeBreakTest.txt is Unicode's own statement of where every boundary falls
# in several hundred crafted strings, and GraphemeBreakProperty.txt with
# emoji-data.txt and DerivedCoreProperties.txt are what cmd/gensegment turns into
# the property table. The generated table is committed and the input is not, so a
# checkout builds with no network.
GRAPHEME_DIR := testdata/unicode-grapheme

GRAPHEME_URL := $(UCD_URL)/auxiliary/GraphemeBreakTest.txt
GRAPHEME_STAMP := $(call stamp,$(GRAPHEME_DIR),$(GRAPHEME_URL))

grapheme-tests: $(GRAPHEME_STAMP)

$(GRAPHEME_STAMP):
	mkdir -p $(GRAPHEME_DIR)
	$(call fetch,$(GRAPHEME_DIR)/GraphemeBreakTest.txt,$(GRAPHEME_URL))
	touch $@

# The whole package, because the three tests that matter here are named three
# different things and a pattern that has to list them is a pattern that will be
# wrong again. TestTheConformanceSuiteHasTeeth takes each of UAX #29's rules
# away in turn and requires the sweep to reject the result — it is the check on
# the check, and it once matched no pattern at all and so never ran.
test-grapheme: grapheme-tests
	UNICODE_GRAPHEME_TESTS=$(abspath $(GRAPHEME_DIR)) \
	  go test -v -count=1 -timeout $(TEST_TIMEOUT) ./segment

clean-grapheme-tests:
	rm -rf $(GRAPHEME_DIR)

# UAX #15's normalisation forms, which shape.ComposeCanonically produces one of.
#
# NormalizationTest.txt gives five spellings of the same text per line and states
# the invariants an implementation has to satisfy. Fetched and not committed,
# like the two suites above: twenty thousand lines of upstream data that a
# checkout does not need to build.
NORMALIZATION_DIR := testdata/unicode-normalization

NORMALIZATION_URL := $(UCD_URL)/NormalizationTest.txt
NORMALIZATION_STAMP := $(call stamp,$(NORMALIZATION_DIR),$(NORMALIZATION_URL))

normalization-tests: $(NORMALIZATION_STAMP)

$(NORMALIZATION_STAMP):
	mkdir -p $(NORMALIZATION_DIR)
	$(call fetch,$(NORMALIZATION_DIR)/NormalizationTest.txt,$(NORMALIZATION_URL))
	touch $@

# Both tests, because the second is the check on the first: the sweep is run
# again against a normaliser that returns its input unchanged, and has to reject
# it. A sweep handed no cases passes in silence.
test-normalization: normalization-tests
	UNICODE_NORMALIZATION_TESTS=$(abspath $(NORMALIZATION_DIR)) \
	  go test -v -count=1 -timeout $(TEST_TIMEOUT) -run 'NFC|Normalization' ./shape

clean-normalization-tests:
	rm -rf $(NORMALIZATION_DIR)

# shallow_at fetches exactly one commit of one repository: no history, no other
# branches. The CSS parsing tests below are what use it.
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
# This is the css package's external oracle, and the framing matters. These
# expectations were written by someone else, from the specification, and three
# independent parsers (tinycss2, rust-cssparser, Crass) are checked against
# them. So a disagreement is evidence about forme rather than a restatement of
# this engine's own reading; an oracle made from this engine's own output would
# agree with it by construction and guard nothing.
#
# Cloned under testdata (gitignored). The tests skip when CSS_PARSING_TESTS is
# unset, and fail when it names a directory with no corpus in it, which is what
# test-css and test-corpora hand them.
CSS_TESTS_DIR := testdata/css-parsing-tests

# The commit, because a corpus is only an oracle if two runs read the same one.
# This named CSS_TESTS_REF, which nothing ever defined, so the fetch asked for
# the empty string and took whatever the default branch pointed at that morning
# — and the number of cases the suite checks is quoted in the README. What held
# it still was a CI cache keyed on this file, which is to say: any edit here
# swapped the corpus. The reftest corpus was pinned for exactly that reason;
# this one was not.
CSS_TESTS_COMMIT := 203ce36bffd617db7f118c551e32794561fb273d
CSS_TESTS_URL := https://github.com/SimonSapin/css-parsing-tests
CSS_TESTS_STAMP := $(call stamp,$(CSS_TESTS_DIR),$(CSS_TESTS_URL) $(CSS_TESTS_COMMIT))

css-tests: $(CSS_TESTS_STAMP)

$(CSS_TESTS_STAMP):
	$(call shallow_at,$(CSS_TESTS_DIR),$(CSS_TESTS_URL),$(CSS_TESTS_COMMIT))
	touch $@

# The path is absolute because `go test ./css` runs with the package directory
# as its working directory, not the repository root.
# ./style as well as ./css: the colour oracle reads the same corpus, checking
# every colour the CSS Syntax tests name against what this engine parses it to,
# and no target set the variable for it — so it skipped, everywhere, always.
test-css: css-tests
	CSS_PARSING_TESTS=$(abspath $(CSS_TESTS_DIR)) go test -v -count=1 -timeout $(TEST_TIMEOUT) \
	  -run 'TestCSSOracle|TestColorOracle|TestUnsupportedColorFilesAreAccountedFor' \
	  ./css ./style

clean-css-tests:
	rm -rf $(CSS_TESTS_DIR)

# The HTML standard's own list of named character references, which
# cmd/genhtmlentities turns into html/entities.go. The *generated table* is
# committed and the input is not, as with every generated table here: the table
# is part of the source, and re-deriving it needs the network, so a checkout
# builds without one.
#
# Regenerate after the standard adds a name — which it has not done in years, so
# this is a rare errand rather than part of a build.
#
# The standard publishes the file at one URL and keeps no versions, so it is
# pinned by digest rather than by commit: the fetch refuses a file with any
# other SHA-256, and so does cmd/genhtmlentities, and the generated table
# records it. A refusal means the standard changed the file, and taking the
# change is moving HTML_ENTITIES_SHA256.
HTML_ENTITIES := testdata/html/entities.json
HTML_ENTITIES_URL := https://html.spec.whatwg.org/entities.json
HTML_ENTITIES_SHA256 := d741d877ac77c4194c4ad526b5b4a19aef8dfe411ab840a466891cdbb9f362e6

# The fetch is a target of its own so that a caller can have the standard's file
# without having the table rebuilt from it. That is the difference between
# checking the table and asserting it equals itself: TestEntityTableMatchesTheStandard
# reads this file and compares it with the committed html/entities.go, and if
# fetching it also regenerated that file the test would compare the generator's
# output with the generator's output and pass whatever either said.
$(HTML_ENTITIES):
	mkdir -p $(dir $@)
	$(FETCH) -o $@.part $(HTML_ENTITIES_URL)
	echo "$(HTML_ENTITIES_SHA256)  $@.part" | sha256sum -c --quiet - || { \
	  rm -f $@.part; \
	  echo "$(HTML_ENTITIES_URL) is not the file HTML_ENTITIES_SHA256 pins" >&2; \
	  exit 1; \
	}
	mv $@.part $@

html-entities: $(HTML_ENTITIES)
	$(MAKETABLES) html-entities

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
#
# The draft is edited every week, so it is taken at a commit of csswg-drafts
# rather than from its main branch.
CSSWG_COMMIT := cbca081425c1d488556b9ce42e69370eefe3edc4
CSS_COLOR_URL := https://raw.githubusercontent.com/w3c/csswg-drafts/$(CSSWG_COMMIT)/css-color-4/Overview.bs
CSS_COLOR_DIR := testdata/css-color-4
CSS_COLOR_SPEC := $(CSS_COLOR_DIR)/Overview.bs

CSS_COLOR_STAMP := $(call stamp,$(CSS_COLOR_DIR),$(CSS_COLOR_URL))

css-color-spec: $(CSS_COLOR_STAMP)

$(CSS_COLOR_STAMP):
	mkdir -p $(CSS_COLOR_DIR)
	$(call fetch,$(CSS_COLOR_SPEC),$(CSS_COLOR_URL))
	touch $@

css-colors: css-color-spec
	$(MAKETABLES) css-colors

clean-css-colors:
	rm -rf $(CSS_COLOR_DIR)

# Which OpenType language systems a BCP 47 language tag selects, which
# cmd/genlangtags turns into shape/langtags.go.
#
# The mapping is two registries joined — OpenType's language system tags and
# IANA's language subtags — with a long list of corrections where they
# disagree, and HarfBuzz publishes the join as a generated header. Fonts are
# tested against HarfBuzz, so the header is the input: a join made again here
# would differ from it exactly where the corrections are.
#
# Taken at a HarfBuzz release — the one the shaping oracle runs — and pinned by
# digest as well: the fetch refuses a file with any other SHA-256, so does the
# generator, and the table records it. Moving to a newer release is moving both.
HB_LANGTAGS_VERSION := $(HARFBUZZ_VERSION)
HB_LANGTAGS_URL := https://raw.githubusercontent.com/harfbuzz/harfbuzz/$(HB_LANGTAGS_VERSION)/src/hb-ot-tag-table.hh
HB_LANGTAGS_SHA256 := fe80a969cc25ddf2c4613b9ebbc1dd7e26ec105fafc892d9ff9f221a5d2355e6
HB_LANGTAGS := testdata/harfbuzz-langtags/hb-ot-tag-table.hh

$(HB_LANGTAGS):
	mkdir -p $(dir $@)
	$(FETCH) -o $@.part $(HB_LANGTAGS_URL)
	echo "$(HB_LANGTAGS_SHA256)  $@.part" | sha256sum -c --quiet - || { \
	  rm -f $@.part; \
	  echo "$(HB_LANGTAGS_URL) is not the file HB_LANGTAGS_SHA256 pins" >&2; \
	  exit 1; \
	}
	mv $@.part $@

# HarfBuzz's COPYING at the same release, whose notice the table carries.
#
# The "Old MIT" licence permits copying "provided that the above copyright
# notice and the following two paragraphs appear in all copies", and the table
# is a copy of part of HarfBuzz. It said "see its COPYING" and carried none of
# it. cmd/genlangtags writes the file whole into the table's header, and it is
# pinned by digest for the same reasons the header is.
HB_COPYING_URL := https://raw.githubusercontent.com/harfbuzz/harfbuzz/$(HB_LANGTAGS_VERSION)/COPYING
HB_COPYING_SHA256 := ba8f810f2455c2f08e2d56bb49b72f37fcf68f1f4fade38977cfd7372050ad64
HB_COPYING := testdata/harfbuzz-langtags/COPYING

$(HB_COPYING):
	mkdir -p $(dir $@)
	$(FETCH) -o $@.part $(HB_COPYING_URL)
	echo "$(HB_COPYING_SHA256)  $@.part" | sha256sum -c --quiet - || { \
	  rm -f $@.part; \
	  echo "$(HB_COPYING_URL) is not the file HB_COPYING_SHA256 pins" >&2; \
	  exit 1; \
	}
	mv $@.part $@

language-tags: $(HB_LANGTAGS) $(HB_COPYING)
	$(MAKETABLES) language-tags

# What HarfBuzz answers for the language and script tags, which
# shape/language_test.go holds the table above to. Needs the pinned oracle; see
# hbenv.
hblanguages: $(HB_LANGTAGS)
	$(PYTHON) $(HARFBUZZ_DIR)/langtags.py $(HB_LANGTAGS) $(HARFBUZZ_DIR)/langtags.expected.txt
	$(PYTHON) $(HARFBUZZ_DIR)/scripttags.py $(HARFBUZZ_DIR)/scripttags.expected.txt

# The licences THIRD_PARTY_NOTICES quotes that no generator reads.
#
# Every notice in that file is a copy of a text somebody else wrote, and a copy
# typed out is a copy with a typo in it. So each text is quoted from a file
# taken at a pin — a commit, a release, or where there is neither, the URL and
# the digest of what it served — and cmd/notices_test.go checks every quotation
# against the file it names. The licences the generators already read — BudouX's
# LICENSE, the AFM readme, the glyph list, the word lists, the hyphenation
# patterns, HarfBuzz's COPYING — are checked against their own copies.
#
# Unicode's licence and W3C's have no versioned URL. A change to either is a
# fetch that fails on its digest, which is the moment to read the new text.
#
#	<file>|<url>|<sha256>
NOTICE_DIR := testdata/notices
NOTICE_SOURCES := \
	brotli-LICENSE|https://raw.githubusercontent.com/google/brotli/$(BROTLI_COMMIT)/LICENSE|3d180008e36922a4e8daec11c34c7af264fed5962d07924aea928c38e8663c94 \
	icu-LICENSE|https://raw.githubusercontent.com/unicode-org/icu/$(ICU_COMMIT)/LICENSE|e55522d81edc687a341a4411e0776e54ca654e90147f354a90458aaced4116af \
	unicode-license.txt|https://www.unicode.org/license.txt|e7a93b009565cfce55919a381437ac4db883e9da2126fa28b91d12732bc53d96 \
	whatwg-html-LICENSE|https://raw.githubusercontent.com/whatwg/html/cd8ac6f1bbf86dd0bd09ef75d27dacaebe7b4c1d/LICENSE|85dc6f5ccb57a6fe8c33d158f9fc8fc7ee5655a5d3db2cdd131c6a3d0f48a864 \
	csswg-drafts-LICENSE.md|https://raw.githubusercontent.com/w3c/csswg-drafts/$(CSSWG_COMMIT)/LICENSE.md|232da9c6c2b9f7e19e5d85cc7cf43760d80b7c4174406ac6404fa2c1b51d531b \
	w3c-software-license-2023.html|https://www.w3.org/copyright/software-license-2023/|ec32c12624d9dc038328872f288355f9e3ff59f2c1ab575c631868eb894415c1

notice-field = $(word $(2),$(subst |, ,$(1)))
NOTICE_FILES := $(foreach n,$(NOTICE_SOURCES),$(NOTICE_DIR)/$(call notice-field,$(n),1))

define notice-rule
$(NOTICE_DIR)/$(call notice-field,$(1),1):
	mkdir -p $(NOTICE_DIR)
	$(FETCH) -o $$@.part $(call notice-field,$(1),2)
	echo "$(call notice-field,$(1),3)  $$@.part" | sha256sum -c --quiet - || { \
	  rm -f $$@.part; \
	  echo "$(call notice-field,$(1),2) is not the file its digest in NOTICE_SOURCES pins" >&2; \
	  exit 1; \
	}
	mv $$@.part $$@
endef
$(foreach n,$(NOTICE_SOURCES),$(eval $(call notice-rule,$(n))))

notice-sources: $(NOTICE_FILES)

clean-notice-sources:
	rm -rf $(NOTICE_DIR)

clean-language-tags:
	rm -rf $(dir $(HB_LANGTAGS))

# Noto, for the scripts the fourteen standard PDF faces do not have.
#
# Those fourteen cover Latin and nothing else, so a document with a Hebrew word
# or a kana in it gets a face that cannot encode the letters — and since
# shape.Face.Encode gives such a face the space's code for anything its
# encoding cannot represent, the word is absent from the page rather than
# showing as boxes anyone would notice. The
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
# Bitstream Vera licence instead. As with Ahem, forme neither vendors nor
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
# Licensing: the compiled fonts are SIL Open Font License 1.1 — the project's own
# LICENSE.txt says so in as many words, the GPL covering the build sources rather
# than the fonts — and it is committed at testdata/unifont/LICENSE.txt and copied
# in beside them.
# A fetch that succeeds and hands back something that is not a font.
#
# unifoundry.com broke CI three times, in two different ways, which is why
# Unifont is fetched from GNU now. The first was not answering at all, which the
# retries above cover. The second was answering 200 with an HTML page where a
# font should be — and that one is the worse failure,
# because curl is content and the corpus looks fetched. The run then reports a
# hundred and four reftests below the baseline, which reads as a layout
# regression right up to the last line of the message, where the harness says
# the fallback faces did not load.
#
# So a font is checked to be one where it is fetched, and the run stops there
# with the first two hundred bytes of whatever arrived. The four magic numbers
# are every sfnt wrapper the fonts here can arrive in: 0x00010000 for TrueType
# outlines, "OTTO" for CFF, "true" for the older Apple flavour, and "wOFF" for
# the one web font the suite ships.
define sfnt
	head -c 4 "$(1)" | od -An -tx1 | tr -d ' \n' | \
	  grep -Eq '^(00010000|4f54544f|74727565|774f4646)$$' || { \
	    echo "$(1) is not a font: $$(wc -c < "$(1)") bytes beginning"; \
	    head -c 200 "$(1)" | od -c | head -5; \
	    exit 1; \
	  }
endef

# Unifont comes from GNU rather than from unifoundry.com, which is the author's
# own site and the single machine the note above is about. It is a GNU package:
# ftp.gnu.org carries every release and a network of mirrors carries ftp.gnu.org,
# and ftpmirror.gnu.org is the redirector that sends a fetch to a working one.
# The two files are byte for byte the ones unifoundry serves — same SHA-256,
# checked before this was changed — so this is the same font from a source that
# is not one machine.
#
# ftp.gnu.org is named as the fallback rather than left to the redirector,
# because a redirector that is down redirects nothing.
#
# The licence is not fetched at all. It is a licence for a pinned version of a
# font, it is 24 KB, and it lives in exactly one place on the web — so making
# the build depend on that place being up, for a file that never changes, is the
# dependency this whole note is about. It is committed, and copied in beside the
# fonts it covers.
UNIFONT_VER      := 17.0.05
UNIFONT_BASE     := https://ftpmirror.gnu.org/gnu/unifont/unifont-$(UNIFONT_VER)
UNIFONT_FALLBACK := https://ftp.gnu.org/gnu/unifont/unifont-$(UNIFONT_VER)
UNIFONT_LICENSE  := testdata/unifont/LICENSE.txt

# One Unifont file, from the redirector or from ftp.gnu.org, and held to its
# SHA-256 wherever it came from. A mirror that serves something else is passed
# over for ftp.gnu.org rather than believed.
#
#	$(call unifont,<destination>,<basename>,<sha256>)
define unifont
	$(call pinned,$(1),$(UNIFONT_BASE)/$(2),$(3)) \
	  || $(call pinned,$(1),$(UNIFONT_FALLBACK)/$(2),$(3))
endef
UNIFONT_SHA256       := 85701ab9b1e251ee16f4df00b13f22eac311d72b7dab427a7d975fe7f5064702
UNIFONT_UPPER_SHA256 := f4fd6d5d752726d384feef175bb780c9f29382cd4941c9e1e6990d7c3822a090

# The library is taken at a commit of each repository it comes from, and every
# file in it is held to its SHA-256.
#
# It was taken from the main branch of both, and that made the reftest baseline
# a function of the day it was fetched. notofonts.github.io is rebuilt by a bot
# most nights, and a face that changes under a branch changes the glyphs, the
# advances and the shaping of every document that falls back to it: a fetch on
# 2026-09-24 gives a NotoSansDevanagari-Regular.ttf that is not the one the
# baseline was measured with, because upstream replaced it on 2026-09-10. Only
# the CI cache, keyed on this file, was holding the library still — which is
# the accident the reftest corpus's own pin was made to end (see WPT_COMMIT).
#
# NOTO_COMMIT is the commit of notofonts.github.io whose files are byte for byte
# the ones the baseline was measured with, and NOTO_CJK_COMMIT (above, beside
# the CJK faces) is noto-cjk's. Each digest below is part of the stamp's key, so
# moving a pin or a digest fetches the library again, and a fetch that does not
# give exactly these files fails rather than moving the baseline.
NOTO_DIR := testdata/fonts-noto
NOTO_COMMIT := 4a4f893ee29c828fb9e018b35c8eaad8aa3449e9
NOTO_URL := https://raw.githubusercontent.com/notofonts/notofonts.github.io/$(NOTO_COMMIT)

# IPAMincho and IPAGothic, which four hanging-punctuation documents ask for by
# *name*.
#
# This is not the Doulos SIL arrangement and the difference is the whole reason
# it is fetched here rather than into $(WPT_DIR)/fonts. Doulos is an
# "@font-face { src: url('/fonts/DoulosSIL-R.woff') }" — a URL the engine's own
# resolver fetches — so putting the file where the URL points answers the
# document. These four write
#
#	font-family: "IPAMincho", "IPAGothic", "IPA明朝", "IPAゴシック";
#
# and no @font-face at all: a *system* family the suite expects the platform to
# have. No file in the corpus can answer that, so the harness lends the faces by
# name instead, which is what layout/suitefonts_test.go's ipaFamilies does. They
# belong in the font library a caller supplies, and that is this directory.
#
# What it is worth, measured rather than estimated: **no clean passes**. The
# library's NotoSansJP-VF is already a fullwidth Japanese face, so these
# documents were already being set with the metrics the tests need, and
# hanging-punctuation-allow-end-001 is forty marks from its reference in either
# face. What it buys is honesty and a work queue: four documents stop reporting
# a font-fallback that was true and useless, and two of them move from unclean
# failures to clean ones — which is what puts them in the distance probe, where
# the next person looks. The ratchet does not move and is not meant to.
#
# Licensing: the IPA Font License 1.0 permits redistribution, and the licence is
# unpacked beside the fonts. No font bytes are vendored in this repository or
# shipped in anything it builds — the same arrangement as Ahem, Doulos and the
# Noto faces.
IPAFONT_URL := https://moji.or.jp/wp-content/ipafont/IPAfont/IPAfont00303.zip
IPAFONT_SHA256 := f755ed79a4b8e715bed2f05a189172138aedf93db0f465b4e20c344a02766fe5

# The hinted faces of notofonts.github.io, by family: each is
# fonts/<family>/hinted/ttf/<family>-Regular.ttf, saved under its base name.
#
#	<family>:<sha256>
NOTO_HINTED := \
	NotoSans:478c558ea716033cd60c03438f628dfa75694dcf6b5f6d505a2f05fd2b4f3823 \
	NotoSansHebrew:cdefaf8efd47045f6820928eba84db5bed7557539328952b5f828315485e02ee \
	NotoSansArabic:bdff3e5659d67e67def05b33f749683b9376ae819d65d3dd62ac4640b3aaef48 \
	NotoSansDevanagari:306b53ecfb182a504dd8a7446093c316387d2fd8dc350d0792ed1753fe0996cd \
	NotoSansArmenian:720df88c332417a235b4d6209d14ec2e2bf4bfe2a954b7453d869ea593bfce1e \
	NotoSansGeorgian:d3e33254b09e7bb2c5cf0f17e554b80462056c5a107097f258d495168c3a9346 \
	NotoSansOgham:5b3705f2dbc34a493eaa968af282456f319dd74cd230a61614d5b7f6baa31121 \
	NotoSansCoptic:e70bd535d7e6cdf2346eab36ea76441059b18ee14d3243e85240b5e65eb0ad45 \
	NotoSansDeseret:9f384e8a75a059b8efcbead73ef5aa3b504ac3e9d218be5368a20b19bfccdeec \
	NotoSansSymbols:d0e98e9a2c046594c5021437273943be7e79e0fd980fde125279e22302212595 \
	NotoSerifTibetan:ee97bf3dc56e813651db734c9f35f8f1d41e7e31acf5f7d893e64ad22b292446

# The files taken from noto-cjk by path, each saved under its base name, and
# the licence that covers them all, saved as OFL.txt. They are lists for the
# stamp's sake: a file written into the recipe and not into a list is a file a
# checkout with the stamp never fetches.
#
#	<path in noto-cjk>:<sha256>
NOTO_PATHS := \
	Sans/Variable/TTF/Subset/NotoSansJP-VF.ttf:f4b373b226668ee33a6e54b02823dcd2d1209f17159f777421ae8c2275160369
NOTO_LICENSE := Sans/LICENSE:6a73f9541c2de74158c0e7cf6b0a58ef774f5a780bf191f2d7ec9cc53efe2bf2
NOTO_STAMP := $(call stamp,$(NOTO_DIR),$(NOTO_URL) $(NOTO_HINTED) $(NOTO_CJK_URL) \
	$(NOTO_PATHS) $(NOTO_LICENSE) $(UNIFONT_BASE) $(UNIFONT_FALLBACK) \
	$(UNIFONT_SHA256) $(UNIFONT_UPPER_SHA256) $(IPAFONT_URL) $(IPAFONT_SHA256))

noto-fonts: $(NOTO_STAMP)

$(NOTO_STAMP):
	mkdir -p $(NOTO_DIR)
	for e in $(foreach f,$(NOTO_HINTED),'$(f)'); do \
	  fam=$${e%%:*}; sum=$${e#*:}; \
	  $(call pinned,$(NOTO_DIR)/$$fam-Regular.ttf,$(NOTO_URL)/fonts/$$fam/hinted/ttf/$$fam-Regular.ttf,$$sum) \
	    || exit 1; \
	done
	for e in $(foreach f,$(NOTO_PATHS),'$(f)'); do \
	  p=$${e%%:*}; sum=$${e#*:}; \
	  $(call pinned,$(NOTO_DIR)/$$(basename $$p),$(NOTO_CJK_URL)/$$p,$$sum) || exit 1; \
	done
	$(call pinned,$(NOTO_DIR)/OFL.txt,$(NOTO_CJK_URL)/$(firstword $(subst :, ,$(NOTO_LICENSE))),$(lastword $(subst :, ,$(NOTO_LICENSE))))
	$(call unifont,$(NOTO_DIR)/Unifont-Regular.otf,unifont-$(UNIFONT_VER).otf,$(UNIFONT_SHA256))
	$(call unifont,$(NOTO_DIR)/UnifontUpper-Regular.otf,unifont_upper-$(UNIFONT_VER).otf,$(UNIFONT_UPPER_SHA256))
	cp $(UNIFONT_LICENSE) $(NOTO_DIR)/UNIFONT-LICENSE.txt
	$(call pinned,$(NOTO_DIR)/ipafont.zip,$(IPAFONT_URL),$(IPAFONT_SHA256))
	unzip -o -j -d $(NOTO_DIR) $(NOTO_DIR)/ipafont.zip \
	  'IPAfont00303/ipam.ttf' \
	  'IPAfont00303/ipag.ttf' \
	  'IPAfont00303/IPA_Font_License_Agreement_v1.0.txt'
	mv $(NOTO_DIR)/IPA_Font_License_Agreement_v1.0.txt \
	  $(NOTO_DIR)/IPA-Font-License-v1.0.txt
	rm -f $(NOTO_DIR)/ipafont.zip
	$(MAKE) verify-fonts
	touch $@

# Every fetched face, checked to be a face.
#
# A target of its own, and CI runs it whether or not anything was fetched. That
# is the half the check in the recipe above cannot cover: a cache hit skips the
# fetch entirely — the stamp is restored with the fonts — so a corrupt
# file that once reached the cache would be served to every run afterwards and
# never looked at again.
verify-fonts:
	for f in $(NOTO_DIR)/*.ttf $(NOTO_DIR)/*.otf $(WPT_DIR)/fonts/*.ttf \
	         $(WPT_DIR)/fonts/*.otf $(WPT_DIR)/fonts/*.woff; do \
	  [ -e "$$f" ] || continue; \
	  $(call sfnt,$$f); \
	done
	@echo "every font in $(NOTO_DIR) and $(WPT_DIR)/fonts is one"

clean-noto-fonts:
	rm -rf $(NOTO_DIR)


# W3C Web Platform Tests: the external oracle for the layout engine.
#
# A CSS reftest is a pair of documents with the assertion *these two render
# identically*, and the pair and the claim come from the CSS Working Group. That
# is what makes it an oracle rather than a restatement of this engine's own
# reading.
# Reftests are also built so that the two documents reach the same rendering by
# *different* mechanisms, so an engine bug usually moves one and not the other.
#
# No browser is needed: forme renders both and compares its own display lists.
#
# WPT is enormous, so this is a blobless sparse clone rather than the whole of
# it. The directories are everything a page laid out *once* can be held to.
#
# What is left out is left out for a reason and not for convenience: pagination
# and page-box describe flowing content across several pages, which this engine
# does not do (it lays a document out on one sheet and scales it to fit); ui and run-in are interaction and a feature CSS removed. Floats,
# positioning and z-index are emphatically *in* — they are only dynamic in a
# viewport that resizes, and this one does not.
WPT_DIR  := testdata/wpt
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
# against and which the harness hands to the engine — see layout/suitefonts_test.go
# for why a test font is the only way those assertions can be expressed.
#
# Licensing, since it is a font and fonts often are not as free as the code
# around them: Ahem.ttf is tracked in the web-platform-tests repository, which
# is under the 3-Clause BSD licence above, and carries no separate licence of
# its own. forme neither vendors nor redistributes it — it is fetched into this
# gitignored directory exactly as the rest of the corpus is, is used only to run
# the tests, and no font bytes are shipped in this repository or in anything it
# builds. The exposure is therefore the same as depending on the suite at all,
# which the ratchet already does.

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

# Named for the directory list as well as the commit: a directory added to
# WPT_DIRS is a sparse checkout that has not been made yet, and the stamp is
# how make knows.
WPT_STAMP := $(call stamp,$(WPT_DIR),$(WPT_COMMIT) $(WPT_DIRS))

# After WPT_STAMP, which it names, because make expands a rule's prerequisites
# where it reads the rule.
wpt: $(WPT_STAMP) $(WPT_DIR)/fonts/DoulosSIL-R.woff \
     $(WPT_DIR)/fonts/NotoSansArmenian-Regular \
     $(WPT_DIR)/fonts/NotoSansGeorgian-Regular.ttf

$(WPT_STAMP):
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

$(WPT_DIR)/fonts/DoulosSIL-R.woff: $(WPT_STAMP)
	$(FETCH) -o $(WPT_DIR)/doulos-web.zip $(DOULOS_URL)
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
$(WPT_DIR)/fonts/NotoSansArmenian-Regular: $(WPT_STAMP) $(NOTO_STAMP)
	cp $(NOTO_DIR)/NotoSansArmenian-Regular.ttf $@

$(WPT_DIR)/fonts/NotoSansGeorgian-Regular.ttf: $(WPT_STAMP) $(NOTO_STAMP)
	cp $(NOTO_DIR)/NotoSansGeorgian-Regular.ttf $@

# NOTO_FONTS as well as WPT_TESTS, and noto-fonts as well as wpt. The ratchet
# counts what the engine renders with the font library a *caller* supplies, and
# without the Noto faces thirty documents that pass in CI report a missing glyph
# instead. Running this target without them measured 4,594 against a baseline of
# 4,624 and printed "this is a layout regression" — which is the one thing a
# ratchet must never say when it is wrong, because the reading it invites is to
# lower the number.
# The corpus checks run beside the ratchet and not somewhere else, because the
# number the ratchet holds means nothing without them: it is a count of *these*
# documents, and a checkout at another revision or with another sparse list is a
# measurement of a different suite. They are named here because "TestWPT" does
# not match them, so for as long as that was the whole pattern the pin was
# checked by nothing that anybody ran.
test-wpt: wpt noto-fonts
	WPT_TESTS=$(abspath $(WPT_DIR)) NOTO_FONTS=$(abspath $(NOTO_DIR)) \
	  go test -v -run 'TestWPT|TestTheCorpus|TestTheReadme' -count=1 -timeout $(TEST_TIMEOUT) ./layout/

# Where the reftests that are not clean actually are.
#
# The ratchet says how many pass; this says what the rest are, which is a
# different question and the one a person asks before deciding what to work on
# next. It groups the failures by suite and by test family, counts every
# unsupported rule by the tests it appears in, and — the part worth having —
# lists the tests that *pass* and are held back by exactly one rule, with what
# that rule named. Those are the ones a single fix moves onto the clean count.
#
# It is behind its own target rather than run with the ratchet because it is an
# analysis and not an assertion: it fails at nothing, and a sweep that always
# passes has no business in a test run that is supposed to mean something.
wpt-breakdown: wpt noto-fonts
	WPT_BREAKDOWN=1 WPT_TESTS=$(abspath $(WPT_DIR)) NOTO_FONTS=$(abspath $(NOTO_DIR)) \
	  go test -v -run TestWPTBreakdown -count=1 -timeout $(TEST_TIMEOUT) ./layout/

clean-wpt:
	rm -rf $(WPT_DIR)

# What test-corpora and race need, declared here at the end and not on their own
# rules above.
#
# make expands a rule's prerequisites when it reads the rule, and CORPORA names
# variables — TABLE_SOURCES, HTML_ENTITIES — defined further down this file.
# Written on the rules at the top, they expanded to nothing: both targets ran the
# suite with TABLE_INPUTS=required and without fetching a single table input,
# and every table whose input is fetched failed as "not here". Declared after
# every variable they use, they expand to all of them. cmd/makefile_test.go
# asks make itself what these two depend on, so the order cannot quietly come
# back.
test-corpora race: $(CORPORA)
