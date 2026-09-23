// Package tables is the one list of the files this repository generates: which
// command writes each, from what, and which make target does it.
//
// "Generated" meant three things here. A UCD table could be regenerated from
// pinned data and was checked; a dictionary, a hyphenation table or the phrase
// model could be regenerated only from whatever an upstream branch held that
// day, and nothing checked it; the standard-font metrics, the Brotli tables and
// the glyph list could be regenerated only from files a developer had to find
// themselves. And the check that did exist, cmd/regenerate_test.go, found the
// generators by matching lines of the Makefile, so a recipe it could not expand
// dropped out of the check without a word, one Makefile edit at a time.
//
// So the list is here, as data, and both ends read it: the Makefile runs every
// generator through cmd/maketables, which takes a target name and runs what this
// list says, and cmd/regenerate_test.go runs the same entries through the same
// Generate and compares the result with the committed file. Every input is
// pinned — a Unicode release, a commit, a SHA-256 — by a variable the Makefile
// defines, and every table records its pin.
package tables

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Table is one generated file.
type Table struct {
	// Out is the file, relative to the repository root.
	Out string
	// Generator is the command under cmd/ that writes it.
	Generator string
	// Args are the generator's arguments. "${NAME}" is the value of a Makefile
	// variable, which the caller supplies — see Names. "${OUT}" is where a
	// generator that writes its own file through -out is to write it;
	// every other generator writes to standard output.
	Args []string
	// Target is the make target that fetches the inputs and regenerates it.
	Target string
	// Inputs are the files it reads that are fetched rather than committed,
	// with "${NAME}" as in Args. A table whose inputs are absent cannot be
	// checked, and the test says so rather than passing.
	Inputs []string
}

// out is the placeholder for the file a -out generator writes.
const out = "${OUT}"

// ucdVersion is the argument every generator reading the database is told the
// release by; see cmd/internal/ucd.Check, which each of them calls.
const ucdVersion = "-version=${UNICODE_VERSION}"

// Manifest is every generated file.
var Manifest = []Table{
	// The shaper's tables, from the Unicode Character Database.
	{Out: "shape/scripts.go", Generator: "genscripts", Target: "shapetables",
		Args: []string{ucdVersion, "${UCD}/Scripts.txt", "${UCD}/ScriptExtensions.txt",
			"${UCD}/PropertyValueAliases.txt"}},
	{Out: "shape/joining.go", Generator: "genjoining", Target: "shapetables",
		Args: []string{ucdVersion, "${UCD}/ArabicShaping.txt", "${UCD}/UnicodeData.txt"}},
	{Out: "shape/ignorabletable.go", Generator: "genignorable", Target: "shapetables",
		Args: []string{ucdVersion, "${UCD}/DerivedCoreProperties.txt"}},
	{Out: "shape/indiccategory.go", Generator: "genindic", Target: "shapetables",
		Args: []string{ucdVersion, "${UCD}/IndicSyllabicCategory.txt", "${UCD}/IndicPositionalCategory.txt"}},
	{Out: "shape/indicmatra.go", Generator: "genmatra", Target: "shapetables",
		Args: []string{ucdVersion, "${UCD}/UnicodeData.txt"}},
	{Out: "shape/canonical.go", Generator: "gencanonical", Target: "shapetables",
		Args: []string{ucdVersion, "${UCD}/UnicodeData.txt", "${UCD}/CompositionExclusions.txt"}},
	// Not the database: the script development specifications' list, which is
	// committed under testdata/ms-use with its notice.
	{Out: "shape/indicvowel.go", Generator: "genvowel", Target: "shapetables",
		Args: []string{"testdata/ms-use/IndicShapingInvalidCluster.txt"}},
	{Out: "shape/usetable.go", Generator: "genuse", Target: "useable",
		Args: []string{ucdVersion,
			"${UCD}/IndicSyllabicCategory.txt", "${UCD}/IndicPositionalCategory.txt",
			"${UCD}/UnicodeData.txt", "${UCD}/DerivedCoreProperties.txt", "${UCD}/ArabicShaping.txt",
			"testdata/ms-use/IndicSyllabicCategory-Additional.txt",
			"testdata/ms-use/IndicPositionalCategory-Additional.txt"}},

	// The bidirectional properties and the grapheme clusters.
	{Out: "bidi/tables.go", Generator: "genbidi", Target: "bidi-tables",
		Args: []string{ucdVersion, "${UCD}/UnicodeData.txt", "${UCD}/extracted/DerivedBidiClass.txt",
			"${UCD}/BidiBrackets.txt", "${UCD}/BidiMirroring.txt"}},
	{Out: "segment/tables.go", Generator: "gensegment", Target: "grapheme-tables",
		Args: []string{ucdVersion, "-ucd=${UCD}", "-out=" + out}},

	// The paragraph's tables.
	{Out: "paragraph/linebreaktable.go", Generator: "genlinebreak", Target: "linebreak",
		Args: []string{ucdVersion, "${UCD}/LineBreak.txt"}},
	{Out: "paragraph/casingtable.go", Generator: "gencasing", Target: "casing",
		Args: []string{ucdVersion, "${UCD}/UnicodeData.txt", "${UCD}/SpecialCasing.txt"}},
	{Out: "paragraph/eastasiantable.go", Generator: "geneastasian", Target: "eastasian",
		Args: []string{ucdVersion, "${UCD}/EastAsianWidth.txt", "${UCD}/Scripts.txt",
			"${UCD}/UnicodeData.txt", "${UCD}/emoji/emoji-data.txt"}},
	{Out: "paragraph/verticaltable.go", Generator: "genvertical", Target: "vertical",
		Args: []string{ucdVersion, "${UCD}/VerticalOrientation.txt"}},
	{Out: "paragraph/widthtable.go", Generator: "genfullwidth", Target: "widths",
		Args: []string{ucdVersion, "${UCD}/UnicodeData.txt"}},
	{Out: "paragraph/kanatable.go", Generator: "genfullsizekana", Target: "widths",
		Args: []string{ucdVersion, "${UCD}/UnicodeData.txt"}},

	// The character properties no table above is about — General_Category,
	// White_Space, Soft_Dotted, Cased and Case_Ignorable — which were package
	// unicode's, from another release.
	{Out: "internal/charprop/tables.go", Generator: "gencharprop", Target: "charprops",
		Args: []string{ucdVersion, "${UCD}/UnicodeData.txt", "${UCD}/PropList.txt",
			"${UCD}/DerivedCoreProperties.txt"}},

	// The word lists, from ICU at ICU_COMMIT.
	dictionary("thai", "thaidict"),
	dictionary("lao", "laodict"),
	dictionary("khmer", "khmerdict"),
	dictionary("burmese", "burmesedict"),

	// The phrase model, from BudouX at BUDOUX_COMMIT.
	{Out: "paragraph/japanesephrases.go", Generator: "genphrase", Target: "phrases",
		Args: []string{"-source=${BUDOUX}/budoux/models/ja.json", "japanese",
			"${BUDOUX_DIR}/ja.json", "${BUDOUX_DIR}/LICENSE"},
		Inputs: []string{"${BUDOUX_DIR}/ja.json", "${BUDOUX_DIR}/LICENSE"}},

	// The hyphenation patterns, from tex-hyphen at TEX_HYPHEN_COMMIT.
	hyphenation("english", "en", "hyph-en-us"),
	hyphenation("dutch", "nl", "hyph-nl"),
	hyphenation("hungarian", "hu", "hyph-hu"),
	hyphenation("pinyin", "zh-latn", "hyph-zh-latn-pinyin"),

	// The standard fonts' metrics, from matplotlib's copies of Adobe's AFM files
	// at MATPLOTLIB_COMMIT.
	{Out: "shape/standard14.go", Generator: "genstdfonts", Target: "stdfonts",
		Args:   []string{"-source=${AFM_URL}", "${AFM_DIR}"},
		Inputs: afmFiles()},

	// The two tables a Brotli decoder cannot compute, from the reference
	// implementation at BROTLI_COMMIT.
	{Out: "brotli/tables.go", Generator: "genbrotli", Target: "brotli-tables",
		Args:   []string{"-source=${BROTLI_URL}", "${BROTLI_DIR}/context.c", "${BROTLI_DIR}/transform.c"},
		Inputs: []string{"${BROTLI_DIR}/context.c", "${BROTLI_DIR}/transform.c"}},

	// The glyph names the standard encodings use, from Adobe's Glyph List at
	// AGL_COMMIT.
	{Out: "font/glyphnames.go", Generator: "genglyphlist", Target: "glyphlist",
		Args:   []string{"-source=${AGL_URL}", "${AGL_DIR}/glyphlist.txt"},
		Inputs: []string{"${AGL_DIR}/glyphlist.txt"}},

	// The HTML standard's named character references, pinned by digest.
	{Out: "html/entities.go", Generator: "genhtmlentities", Target: "html-entities",
		Args: []string{"-source=${HTML_ENTITIES_URL}", "-sha256=${HTML_ENTITIES_SHA256}",
			"-in=${HTML_ENTITIES}", "-out=" + out},
		Inputs: []string{"${HTML_ENTITIES}"}},

	// Which OpenType language systems a BCP 47 tag selects, from HarfBuzz's
	// generated header at a release, pinned by digest.
	{Out: "shape/langtags.go", Generator: "genlangtags", Target: "language-tags",
		Args: []string{"-source=${HB_LANGTAGS_URL}", "-sha256=${HB_LANGTAGS_SHA256}",
			"-in=${HB_LANGTAGS}", "-license-source=${HB_COPYING_URL}",
			"-license-sha256=${HB_COPYING_SHA256}", "-license=${HB_COPYING}", "-out=" + out},
		Inputs: []string{"${HB_LANGTAGS}", "${HB_COPYING}"}},

	// CSS Color 4's named colours, from csswg-drafts at CSSWG_COMMIT.
	{Out: "style/colors.go", Generator: "gencolors", Target: "css-colors",
		Args:   []string{"-source=${CSS_COLOR_URL}", "-in=${CSS_COLOR_SPEC}", "-out=" + out},
		Inputs: []string{"${CSS_COLOR_SPEC}"}},
}

// Exempt are the commands under cmd/ named gen* that write no table in this
// list, and why. The test requires every such command to be in one or the
// other, so a new generator cannot be written and left out of both.
var Exempt = map[string]string{
	"genwoff2hmtx": "it writes a binary font fixture, not a table, and its output depends " +
		"on the brotli command that compresses it; font/woff2_test.go holds the " +
		"fixture to decoding to exactly what its input decodes to, which is the " +
		"property that matters and the one a byte comparison could not state",
}

func dictionary(name, file string) Table {
	return Table{
		Out: "paragraph/" + name + "dict.go", Generator: "gendict", Target: "dictionaries",
		Args:   []string{"-source=${ICU_DICTS}/" + file + ".txt", name, "${DICT_DIR}/" + file + ".txt"},
		Inputs: []string{"${DICT_DIR}/" + file + ".txt"},
	}
}

func hyphenation(name, key, file string) Table {
	return Table{
		Out: "paragraph/" + name + "hyphens.go", Generator: "genhyphen", Target: "hyphens",
		Args:   []string{"-source=${HYPHEN_URL}/" + file + ".tex", name, key, "${HYPHEN_DIR}/" + file + ".tex"},
		Inputs: []string{"${HYPHEN_DIR}/" + file + ".tex"},
	}
}

// afmFiles are the fourteen files cmd/genstdfonts reads.
func afmFiles() []string {
	var out []string
	for _, name := range []string{
		"Courier", "Courier-Bold", "Courier-Oblique", "Courier-BoldOblique",
		"Helvetica", "Helvetica-Bold", "Helvetica-Oblique", "Helvetica-BoldOblique",
		"Times-Roman", "Times-Bold", "Times-Italic", "Times-BoldItalic",
		"Symbol", "ZapfDingbats",
	} {
		out = append(out, "${AFM_DIR}/"+name+".afm")
	}
	return out
}

// reference is a "${NAME}" in an argument.
var reference = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// Names are the variables a set of tables refers to, OUT aside.
func Names(ts []Table) []string {
	seen := map[string]bool{}
	var names []string
	for _, t := range ts {
		for _, s := range append(append([]string(nil), t.Args...), t.Inputs...) {
			for _, m := range reference.FindAllStringSubmatch(s, -1) {
				if m[1] != "OUT" && !seen[m[1]] {
					seen[m[1]] = true
					names = append(names, m[1])
				}
			}
		}
	}
	return names
}

// Expand replaces every "${NAME}" in s with its value. A name with no value is
// an error and not an empty string: an argument that expanded to nothing would
// run the generator over the wrong file, or over none.
func Expand(s string, vars map[string]string) (string, error) {
	var missing []string
	got := reference.ReplaceAllStringFunc(s, func(ref string) string {
		name := reference.FindStringSubmatch(ref)[1]
		v, ok := vars[name]
		if !ok || v == "" {
			missing = append(missing, name)
		}
		return v
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("%q names %s, which has no value", s, strings.Join(missing, ", "))
	}
	return got, nil
}

// ExpandedInputs are the fetched files the table reads, expanded: its Inputs,
// and for a table read from the Unicode Character Database, every file of the
// database it names and UnicodeData.txt, which is what says a database is there
// at all.
func (t Table) ExpandedInputs(vars map[string]string) ([]string, error) {
	inputs := append([]string(nil), t.Inputs...)
	for _, a := range t.Args {
		if strings.Contains(a, "${UCD}") {
			inputs = append(inputs, "${UCD}/UnicodeData.txt")
			break
		}
	}
	for _, a := range t.Args {
		if strings.HasPrefix(a, "${UCD}/") {
			inputs = append(inputs, a)
		}
	}
	var out []string
	for _, in := range inputs {
		p, err := Expand(in, vars)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// Generate runs the table's generator from the repository root and returns what
// it produced, formatted as gofmt would. Nothing in the tree is written: a
// generator that writes through -out is pointed at a temporary file.
//
// Any failure is an error, and the output is never partial. A generator that
// exits non-zero, writes nothing, or writes something that is not Go returns
// an error and no bytes.
func (t Table) Generate(root string, vars map[string]string) ([]byte, error) {
	tmp, err := os.MkdirTemp("", "maketables-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	outFile := filepath.Join(tmp, filepath.Base(t.Out))

	all := map[string]string{"OUT": outFile}
	for k, v := range vars {
		all[k] = v
	}
	args := []string{"run", "./cmd/" + t.Generator}
	writesOut := false
	for _, a := range t.Args {
		if strings.Contains(a, out) {
			writesOut = true
		}
		e, err := Expand(a, all)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", t.Out, err)
		}
		args = append(args, e)
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.Command("go", args...)
	cmd.Dir, cmd.Stdout, cmd.Stderr = root, &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s: go %s: %v\n%s", t.Out, strings.Join(args, " "), err, stderr.String())
	}
	src := stdout.Bytes()
	if writesOut {
		if src, err = os.ReadFile(outFile); err != nil {
			return nil, fmt.Errorf("%s: cmd/%s wrote nothing: %v", t.Out, t.Generator, err)
		}
	}
	if len(bytes.TrimSpace(src)) == 0 {
		return nil, fmt.Errorf("%s: cmd/%s produced nothing", t.Out, t.Generator)
	}
	formatted, err := format.Source(src)
	if err != nil {
		return nil, fmt.Errorf("%s: cmd/%s produced something that is not Go: %v", t.Out, t.Generator, err)
	}
	return formatted, nil
}

// Replace writes data to path by writing a temporary file beside it and
// renaming it into place, so that path holds either what it held or all of
// data, and never a truncated file. A recipe of the form "go run ... > path"
// emptied path before the generator had run, and a generator that then failed
// left the build broken on a file under version control.
func Replace(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	_, werr := f.Write(data)
	cerr := f.Close()
	if werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(name, 0o644)
	}
	if werr == nil {
		werr = os.Rename(name, path)
	}
	if werr != nil {
		os.Remove(name)
		return werr
	}
	return nil
}
