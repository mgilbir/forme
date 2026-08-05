package shape

import "testing"

// The bundled face's per-script rule selection. It lives here rather than beside
// the font because it reads the layout this package parses, which is not
// something the package that merely carries the file can see.
// TestBundledFontGivesEachScriptItsOwnRules is the script selection checked
// against a real face rather than a fixture.
//
// Noto Sans declares twenty-one 'locl' substitutions across its scripts and
// languages — the letterform corrections that make Serbian Cyrillic look
// Serbian and Romanian Latin look Romanian — spread over a dozen separate
// features. Taken together, as a reader that ignores the ScriptList must take
// them, a Latin run receives the corrections meant for Serbian. Taken per
// script, each run receives its own set, and the sets differ.
func TestBundledFontGivesEachScriptItsOwnRules(t *testing.T) {
	f, err := NotoSans()
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	const tag = "locl"

	everything := f.layout.single[tag]
	if len(everything) == 0 {
		t.Fatalf("the fixture assumption is gone: the face declares no %q substitutions at all", tag)
	}

	latin := f.layoutFor(runScript("abc")).single[tag]
	cyrillic := f.layoutFor(runScript("абв")).single[tag]
	if sameSubstitutions(latin, cyrillic) {
		t.Errorf("Latin and Cyrillic runs get the same %q substitutions (%d of them); "+
			"the script list was not consulted", tag, len(latin))
	}
	for name, sel := range map[string]map[int]int{"Latin": latin, "Cyrillic": cyrillic} {
		if len(sel) >= len(everything) {
			t.Errorf("%s gets %d %q substitutions and the whole font declares %d; "+
				"a script should get fewer than all of them", name, len(sel), tag, len(everything))
		}
		for from, to := range sel {
			if everything[from] != to {
				t.Errorf("%s substitutes glyph %d with %d, which is not what the font declares anywhere",
					name, from, to)
			}
		}
	}

	// And a language system narrows it further. Romanian is one of the seven
	// Noto Sans declares under 'latn'.
	f.SetLanguage("ROM ")
	romanian := f.layoutFor(runScript("abc")).single[tag]
	if sameSubstitutions(romanian, latin) {
		t.Errorf("Romanian gets the same %d %q substitutions as the default language system",
			len(romanian), tag)
	}
}

func sameSubstitutions(a, b map[int]int) bool {
	if len(a) != len(b) {
		return false
	}
	for from, to := range a {
		if b[from] != to {
			return false
		}
	}
	return true
}
