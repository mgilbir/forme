.PHONY: test bidi-tests test-bidi clean-bidi-tests hbshaping test-hbshaping hbfuzz useable clean-ucd stdfonts

test:
	gofmt -l . | grep -v '^testdata/' && exit 1 || true
	go vet ./...
	go test -count=1 ./...

# Unicode's own bidirectional conformance suites, which bidi_conformance_test.go
# runs in full. Fetched rather than vendored: 15 MB, versioned by Unicode, and
# pinned to the release the tables were generated from — a character whose class
# changed between releases is a stale table rather than a defect.
BIDI_DIR := testdata/unicode-bidi
UNICODE_VERSION ?= 17.0.0

bidi-tests: $(BIDI_DIR)/.ok

$(BIDI_DIR)/.ok:
	mkdir -p $(BIDI_DIR)
	curl -fsSL -o $(BIDI_DIR)/BidiTest.txt \
		https://www.unicode.org/Public/$(UNICODE_VERSION)/ucd/BidiTest.txt
	curl -fsSL -o $(BIDI_DIR)/BidiCharacterTest.txt \
		https://www.unicode.org/Public/$(UNICODE_VERSION)/ucd/BidiCharacterTest.txt
	touch $@

test-bidi: bidi-tests
	UNICODE_BIDI_TESTS=$(abspath $(BIDI_DIR)) go test -v -run TestBidiConformance -count=1 ./shape

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

notocjk:
	@test -d $(CJK_DIR)/.git || git clone --filter=blob:none --no-checkout --sparse \
		https://github.com/notofonts/noto-cjk.git $(CJK_DIR)
	git -C $(CJK_DIR) fetch origin main
	git -C $(CJK_DIR) sparse-checkout set --no-cone \
		'/Sans/SubsetOTF/**' '/Serif/SubsetOTF/**'
	git -C $(CJK_DIR) checkout -f -B main origin/main

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
