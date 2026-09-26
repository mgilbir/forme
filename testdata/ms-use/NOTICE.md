# The Universal Shaping Engine's data, from HarfBuzz

This directory holds three files from HarfBuzz's `src/ms-use`, fetched by
`make ms-use-sources` and not committed:

| File | SHA-256 |
|---|---|
| `IndicPositionalCategory-Additional.txt` | `2baa1c1efe5a5f108c304b1e27d0d97864c806764eb2b0a1bd91db80ae26b5b8` |
| `IndicShapingInvalidCluster.txt` | `02024d4289864665721e14ec99eb320ce187f289514b793449b4f6a8ddaf5944` |
| `IndicSyllabicCategory-Additional.txt` | `b9472e3e72d5fba8cb3f2e0578e73012aaae786db25779de0f1a5a5ab69b84a6` |

They are taken at HarfBuzz 14.5.0 —
`https://raw.githubusercontent.com/harfbuzz/harfbuzz/14.5.0/src/ms-use/<file>` —
which is the Makefile's `HARFBUZZ_VERSION` and `MSUSE_URL`, and the fetch
refuses a file whose SHA-256 is not the one `MSUSE_FILES` lists (the same
three as above). To check any of this: `make ms-use-sources`, then
`sha256sum testdata/ms-use/*.txt`.

They used to be committed here, taken from HarfBuzz at a commit nobody
recorded. Two of the three were not the files at 14.5.0: HarfBuzz had since
dropped one override (U+11A3A ZANABAZAR SQUARE CLUSTER-INITIAL LETTER RA,
`Consonant_With_Stacker`, which Unicode 17's own `IndicSyllabicCategory.txt`
already gives it), added two header lines to each override file, and trimmed
trailing spaces. Regenerated from the pinned files, `shape/usetable.go` and
`shape/indicvowel.go` differ from the tables made from the old ones only in the
line of their header that names the source.

## What they are

The two `-Additional` files are the Universal Shaping Engine's corrections to
two Unicode properties, `Indic_Syllabic_Category` (134 entries) and
`Indic_Positional_Category` (63). Each file's header begins "Override values
For …" and "Not derivable", and records its updates, the last "for Unicode 18.0
by Andrew Glass 2026-09-04".

`IndicShapingInvalidCluster.txt` is `Indic_Shaping_Invalid_Cluster`: 103
sequences of an independent vowel and a vowel sign that every shaper draws with
a dotted circle between them. Its header dates it 2015-03-12 and 2019-11-08.

## Why they are read rather than derived

`cmd/genuse` derives the engine's categories from five properties of the
Unicode database, and the overrides are what the engine decides differently.
U+A9BE JAVANESE CONSONANT SIGN PENGKAL is `Bottom_And_Right` in Unicode 17's
`IndicPositionalCategory.txt` and `Right` in the override, so it is `Blw` to a
shaper that reads Unicode alone and `Pst` to the engine;
`shape.TestTheEngineTakesItsOwnViewWhereUnicodeDiffers` holds the table to the
second.

They are read by `cmd/genuse` (the two overrides) and `cmd/genvowel` (the
cluster list) when the tables are regenerated, and by
`testdata/harfbuzz/usecategories.py`, which runs HarfBuzz's own
`gen-use-table.py` over the same files to produce the oracle
`shape/usecategories_test.go` compares against. Nothing reads them at run
time.

## Licence

HarfBuzz is under the "Old MIT" licence. None of the three files carries a
notice of its own. HarfBuzz's COPYING at 14.5.0, whole, is quoted in
`THIRD_PARTY_NOTICES` at the root of this repository, under the entry that
lists the two tables made from these files, and `cmd/notices_test.go` holds
the quotation to the pinned file.
