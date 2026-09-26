package html

import (
	"strings"
	"testing"
)

// The HTML half of "ASCII case-insensitive means ASCII": an attribute name asked
// for by name, and the <meta> the encoding check reads.

// TestAnAttributeIsAskedForByItsASCIIFold. Attr lowercases the name it is
// asked for, because HTML attribute names are lowercased as they are read. It
// lowercased with strings.ToLower, which is Unicode's, so a name holding a
// KELVIN SIGN found the attribute spelled with a "k": attr() and an attribute
// selector naming "[\u212Aind]" read a <track>'s kind.
func TestAnAttributeIsAskedForByItsASCIIFold(t *testing.T) {
	doc := mustParseHTML(t, `<p><track kind="subtitles"/></p>`)
	track := findElement(doc, "track")
	if v, ok := track.Attr("KIND"); !ok || v != "subtitles" {
		t.Errorf("Attr(\"KIND\") = (%q, %v), want the kind", v, ok)
	}
	if v, ok := track.Attr("\u212Aind"); ok {
		t.Errorf("Attr with a KELVIN SIGN found %q; the name is not \"kind\"", v)
	}
	if doc.Element("TRACK") != track {
		t.Error("Element(\"TRACK\") did not find the <track>")
	}
	if doc.Element("\u212Abd") != nil || doc.Element("TRAC\u212A") != nil {
		t.Error("Element with a KELVIN SIGN found an element")
	}
}

// TestTheEncodingFindingPointsAtTheMeta. The encoding check lowercased the
// head of the document with strings.ToLower and searched the copy for
// "<meta". Unicode's lowercasing does not keep a string's length — a KELVIN
// SIGN is three bytes and its "k" one, a dotted capital I two bytes and its
// lowercase three — so the offset found in the copy was not the offset of the
// <meta> in the document once any such character came before it, and the
// finding sent the author to the wrong place.
func TestTheEncodingFindingPointsAtTheMeta(t *testing.T) {
	for _, before := range []string{"\u212A\u212A\u212A", "\u0130\u0130\u0130", "caf\u00e9"} {
		src := "<html><head><title>" + before + "</title>" +
			`<meta charset="shift_jis"></head><body><p>` + before + "</p></body></html>"
		want := strings.Index(src, "<meta")
		errs, _ := errorsOf(t, src)
		found := false
		for _, e := range errs {
			if strings.Contains(e.Message, "shift_jis") {
				found = true
				if e.Offset != want {
					t.Errorf("after %q the finding is at byte %d, want %d, where the <meta> is",
						before, e.Offset, want)
				}
			}
		}
		if !found {
			t.Errorf("after %q the declaration was not reported: %v", before, errs)
		}
	}
}
