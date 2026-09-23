// Package paragraph is the half of inline layout that is about text rather
// than about boxes: what CSS Text and Unicode say about the characters of a
// paragraph, and the cutting of its runs into lines.
//
// The rules are stated per character and per run, and none of them needs a box
// tree, a cascade or a document, so they live here and layout calls them:
//
//   - reading the text-level properties from their computed values — WhiteSpaceOf,
//     WordBreakOf, LineBreakOf, OverflowWrapOf, HyphensOf, TransformOf and their
//     neighbours;
//   - white space processing and segment breaks, CollapseWhitespace;
//   - text-transform, TransformText, with Unicode's full and language-tailored
//     case mappings;
//   - where a line may break, SplitAtBreaks: UAX #14's classes with CSS Text's
//     tailorings, word lists for the scripts written without spaces
//     (DictionaryBreaks), BudouX's phrase model for "word-break: auto-phrase"
//     (PhraseBreaks) and Liang's hyphenation patterns for "hyphens: auto"
//     (HyphenPoints);
//   - the spacing a line adds or trims — letter and word spacing, text-autospace,
//     text-spacing-trim, hanging punctuation, tab stops;
//   - the bidirectional order of a line's items, through package bidi
//     (NewBidiBuilder, LineVisualOrder), and which characters stand upright in
//     vertical text (IsUpright);
//   - and Breaker, which measures runs with the faces package shape supplies and
//     cuts them into lines, text-wrap: balance included.
//
// The Unicode properties it reads come from tables generated from the release
// the Makefile pins — linebreaktable.go, eastasiantable.go, casingtable.go,
// verticaltable.go, widthtable.go and kanatable.go — and the word lists, phrase
// model and hyphenation patterns from upstreams pinned the same way; each says
// which command made it. See cmd/internal/tables.
package paragraph
