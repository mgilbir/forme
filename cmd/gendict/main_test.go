package main

import (
	"reflect"
	"strings"
	"testing"
)

// What travels into the generated file as "the licence".
//
// The Unicode terms require the notice to appear with the data, so it is copied
// rather than summarised — which makes it worth being right about what the
// notice *is*. It is the header: the comments above the first word. These files
// annotate their own data as well, and taking every "#" line put ICU's
// "TODO: why does this have full stop in it?" into paragraph/thaidict.go under
// the words "the licence the word list is under".
func TestOnlyTheHeaderIsTheLicence(t *testing.T) {
	// The shape of ICU's own files: a notice with a blank line inside it, the
	// words, and annotations among them.
	const src = "# Copyright (C) 2016 and later: Unicode, Inc. and others.\n" +
		"# License & terms of use: http://www.unicode.org/copyright.html\n" +
		"\n" +
		"#  Copyright (c) 2006-2015 International Business Machines Corporation\n" +
		"กก\n" +
		"#   ดี.ซี. -- TODO: why does this have full stop in it?\n" +
		"กกขนาก\n" +
		"# TODO: why do these have full stops?\n" +
		"กกช้าง\t500\n"

	notice, words, err := readDictionary(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	wantNotice := []string{
		"# Copyright (C) 2016 and later: Unicode, Inc. and others.",
		"# License & terms of use: http://www.unicode.org/copyright.html",
		"#  Copyright (c) 2006-2015 International Business Machines Corporation",
	}
	if !reflect.DeepEqual(notice, wantNotice) {
		t.Errorf("the notice is\n  %q\nand the header is\n  %q", notice, wantNotice)
	}
	for _, line := range notice {
		if strings.Contains(line, "TODO") {
			t.Errorf("%q is in the notice; an annotation among the data is not "+
				"part of anybody's licence", line)
		}
	}
	// A blank line inside the header does not end it, and the words are the
	// words: the annotations are neither notice nor entries.
	if want := []string{"กก", "กกขนาก", "กกช้าง"}; !reflect.DeepEqual(words, want) {
		t.Errorf("the words are %q, want %q", words, want)
	}
}

// TestAFileWithNoWordsKeepsItsWholeHeader is the boundary: a header ends at the
// first word, and a file with none is all header.
func TestAFileWithNoWordsKeepsItsWholeHeader(t *testing.T) {
	notice, words, err := readDictionary(strings.NewReader("# one\n# two\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(words) != 0 {
		t.Errorf("words %q from a file with none", words)
	}
	if want := []string{"# one", "# two"}; !reflect.DeepEqual(notice, want) {
		t.Errorf("the notice is %q, want %q", notice, want)
	}
}
