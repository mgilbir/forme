package cmd

import (
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mgilbir/forme/cmd/internal/tables"
)

// Other people's work carries their notices.
//
// Most of what this repository generates is generated from somebody else's
// data, and a few of the files it keeps are somebody else's files. Their
// licences ask for little, and nearly all of them ask for the same thing: that
// a copy carry the copyright and the permission notice. Nothing held the tree
// to that. shape/langtags.go said "see its COPYING" of a licence whose one
// condition is that the notice appear in every copy, and there was no list
// anywhere of what came from where under which terms — the Makefile's pins said
// where, and nothing said what the terms asked.
//
// THIRD_PARTY_NOTICES is that list, and these are what keep it true:
//
//   - every generated table and every font or binary file the repository keeps
//     is named by an entry, and every file an entry names is kept;
//   - every table's header names what it was made from, and its entry names
//     the pin — the ${VARIABLE} the Makefile fetches it by;
//   - every notice the file quotes is quoted from the file it says it is,
//     letter for letter, white space aside. A notice typed out is a notice with
//     a typo in it, and a licence text somebody has edited is not the licence.
//
// The quoted sources the Makefile fetches are checked when they are here, and
// under TABLE_INPUTS=required their absence is a failure, as it is for the
// tables.

const noticesFile = "THIRD_PARTY_NOTICES"

// noticeEntry is one entry of the file: a title, the files it covers, where
// they came from, and the quotations under it.
type noticeEntry struct {
	title  string
	files  []string
	source string
	quotes []noticeQuote
}

// noticeQuote is one "Quoted from <file>:" block.
type noticeQuote struct {
	from  string
	whole bool
	text  string
	line  int
}

var (
	noticeRule = regexp.MustCompile(`^=+$`)
	quotedFrom = regexp.MustCompile(`^Quoted (whole )?from (\S+):$`)
	noticeVar  = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
)

// readNotices parses THIRD_PARTY_NOTICES.
func readNotices(t *testing.T) []noticeEntry {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, noticesFile))
	if err != nil {
		t.Fatalf("reading %s: %v", noticesFile, err)
	}
	ls := strings.Split(string(data), "\n")
	var out []noticeEntry
	var cur *noticeEntry
	field := ""
	for i := 0; i < len(ls); i++ {
		l := ls[i]
		switch {
		case noticeRule.MatchString(l) && i+2 < len(ls) && noticeRule.MatchString(ls[i+2]):
			out = append(out, noticeEntry{title: ls[i+1]})
			cur = &out[len(out)-1]
			i += 2
			field = ""
		case cur == nil:
		case strings.HasPrefix(l, "Files:"):
			field = "files"
			cur.files = append(cur.files, strings.TrimSpace(strings.TrimPrefix(l, "Files:")))
		case strings.HasPrefix(l, "Source:"):
			field = "source"
			cur.source += strings.TrimSpace(strings.TrimPrefix(l, "Source:")) + "\n"
		case strings.HasPrefix(l, "Licence:"):
			field = ""
		case strings.HasPrefix(l, "         ") && strings.TrimSpace(l) != "" && field != "":
			if field == "files" {
				cur.files = append(cur.files, strings.TrimSpace(l))
			} else {
				cur.source += strings.TrimSpace(l) + "\n"
			}
		default:
			field = ""
			m := quotedFrom.FindStringSubmatch(l)
			if m == nil {
				continue
			}
			q := noticeQuote{from: m[2], whole: m[1] != "", line: i + 1}
			// The quotation is the indented block after it, blank lines and
			// all, up to the first line that is neither.
			var body []string
			for i+1 < len(ls) && (strings.HasPrefix(ls[i+1], "    ") || strings.TrimSpace(ls[i+1]) == "") {
				i++
				body = append(body, strings.TrimPrefix(ls[i], "    "))
			}
			q.text = strings.Join(body, "\n")
			cur.quotes = append(cur.quotes, q)
		}
	}
	if len(out) < 10 {
		t.Fatalf("%s has %d entries this could read; it has stopped reading them", noticesFile, len(out))
	}
	return out
}

// noticeVars expands the ${VARIABLE}s the file uses, and fails on one the
// Makefile does not define.
func noticeVars(t *testing.T) map[string]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, noticesFile))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, m := range noticeVar.FindAllStringSubmatch(string(data), -1) {
		names = append(names, m[1])
	}
	vars := makeVarsFor(t, names...)
	vars["OUT"] = "${OUT}"
	return vars
}

// trackedFiles is what the repository carries.
func trackedFiles(t *testing.T) []string {
	t.Helper()
	out, err := exec.Command("git", "-C", root, "ls-files", "-z").Output()
	if err != nil {
		t.Skipf("git could not list the repository's files: %v", err)
	}
	var files []string
	for _, f := range strings.Split(string(out), "\x00") {
		if f != "" {
			files = append(files, f)
		}
	}
	return files
}

// keptThirdParty reports whether a kept file is one somebody else made: a font
// or a binary blob, which this repository does not write, or one of the files
// testdata/ms-use keeps as HarfBuzz published them.
func keptThirdParty(f string) bool {
	switch strings.ToLower(filepath.Ext(f)) {
	case ".ttf", ".otf", ".ttc", ".woff", ".woff2", ".pfb", ".pfa", ".afm", ".bin":
		return true
	}
	return strings.HasPrefix(f, "testdata/ms-use/") && strings.HasSuffix(f, ".txt")
}

// TestEveryThirdPartyFileHasANotice.
func TestEveryThirdPartyFileHasANotice(t *testing.T) {
	entries := readNotices(t)
	covered := map[string][]string{}
	for _, e := range entries {
		for _, f := range e.files {
			covered[f] = append(covered[f], e.title)
		}
	}
	tracked := trackedFiles(t)
	isTracked := map[string]bool{}
	for _, f := range tracked {
		isTracked[f] = true
	}
	for f, titles := range covered {
		if !isTracked[f] {
			t.Errorf("%s names %s under %q, and the repository has no such file",
				noticesFile, f, titles[0])
		}
	}
	for _, tb := range tables.Manifest {
		if len(covered[tb.Out]) == 0 {
			t.Errorf("%s is generated by cmd/%s from somebody else's data and no entry "+
				"of %s names it", tb.Out, tb.Generator, noticesFile)
		}
	}
	kept := 0
	for _, f := range tracked {
		if !keptThirdParty(f) {
			continue
		}
		kept++
		if len(covered[f]) == 0 {
			t.Errorf("%s is a font, a binary or a vendored data file, and no entry of %s "+
				"names it", f, noticesFile)
		}
	}
	if kept < 10 {
		t.Fatalf("only %d kept third-party files were found; this has stopped looking", kept)
	}
}

// TestEveryTableNamesItsSourceAndItsEntryItsPin. The header of a generated
// table — everything above its package clause — names the files it was made
// from, and the entry that covers it names the pin it was fetched at.
func TestEveryTableNamesItsSourceAndItsEntryItsPin(t *testing.T) {
	entries := readNotices(t)
	vars := makeVars(t)
	for _, tb := range tables.Manifest {
		src, err := os.ReadFile(filepath.Join(root, tb.Out))
		if err != nil {
			t.Fatal(err)
		}
		header := string(src)
		if i := strings.Index(header, "\npackage "); i >= 0 {
			header = header[:i]
		}
		var sources string
		for _, e := range entries {
			for _, f := range e.files {
				if f == tb.Out {
					sources += e.source
				}
			}
		}
		for _, a := range tb.Args {
			switch {
			case strings.HasPrefix(a, "${UCD}/"), strings.HasPrefix(a, "testdata/"):
				if base := filepath.Base(a); !strings.Contains(header, base) {
					t.Errorf("%s is made from %s and its header does not say so", tb.Out, base)
				}
			case strings.HasPrefix(a, "-source="):
				v, err := tables.Expand(strings.TrimPrefix(a, "-source="), vars)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(header, v) {
					t.Errorf("%s's header does not name %s, which it is made from", tb.Out, v)
				}
				// The pin is the variable the URL is built from.
				for _, m := range noticeVar.FindAllStringSubmatch(a, -1) {
					if !strings.Contains(sources, "${"+m[1]+"}") {
						t.Errorf("%s is fetched by ${%s} and the entry of %s that covers it "+
							"does not name it", tb.Out, m[1], noticesFile)
					}
				}
			case a == "-version=${UNICODE_VERSION}":
				if !strings.Contains(sources, "${UNICODE_VERSION}") {
					t.Errorf("%s is read from the Unicode database and the entry of %s that "+
						"covers it does not name ${UNICODE_VERSION}", tb.Out, noticesFile)
				}
			}
		}
	}
}

// TestEveryNoticeIsQuotedFromItsSource.
func TestEveryNoticeIsQuotedFromItsSource(t *testing.T) {
	entries := readNotices(t)
	vars := noticeVars(t)
	required := os.Getenv("TABLE_INPUTS") == "required"
	checked, missing := 0, 0
	for _, e := range entries {
		for _, q := range e.quotes {
			path, err := tables.Expand(q.from, vars)
			if err != nil {
				t.Fatalf("%s:%d: %v", noticesFile, q.line, err)
			}
			data, err := os.ReadFile(filepath.Join(root, path))
			if err != nil {
				missing++
				if required {
					t.Errorf("%s:%d quotes %s, which is not here, and TABLE_INPUTS=required "+
						"says it was fetched; `make notice-sources` and the table targets fetch it",
						noticesFile, q.line, path)
				}
				continue
			}
			source := string(data)
			if strings.HasSuffix(path, ".html") {
				source = htmlText(source)
			}
			got, want := squeeze(q.text), squeeze(source)
			switch {
			case len(got) < 40:
				t.Errorf("%s:%d quotes almost nothing of %s", noticesFile, q.line, path)
			case q.whole && got != want:
				t.Errorf("%s:%d says it quotes %s whole, and it does not: the two differ "+
					"at %s", noticesFile, q.line, path, firstDifference([]byte(got), []byte(want)))
			case !q.whole && !strings.Contains(want, got):
				t.Errorf("%s:%d says it quotes %s, and the text is not in it", noticesFile, q.line, path)
			}
			checked++
		}
	}
	if missing > 0 {
		t.Logf("%d quotations not checked, their sources not being here", missing)
	}
	if checked+missing < 20 {
		t.Fatalf("only %d quotations were found; this has stopped reading them", checked+missing)
	}
}

// squeeze is text as a quotation is compared: each line without the comment
// marks a source writes its header in ("#", "%"), and no white space at all —
// a notice rewrapped is the same notice, and the check is about the words.
func squeeze(s string) string {
	s = strings.TrimPrefix(s, "\ufeff")
	var b strings.Builder
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimLeft(l, " \t")
		l = strings.TrimLeft(l, "#%")
		for _, r := range l {
			switch r {
			case ' ', '\t', '\r', '\f', '\v', '\u00a0', '\ufeff':
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

// htmlText is an HTML page's text: its tags dropped and its entities read.
var htmlTag = regexp.MustCompile(`(?s)<script.*?</script>|<style.*?</style>|<[^>]*>`)

func htmlText(s string) string { return html.UnescapeString(htmlTag.ReplaceAllString(s, " ")) }

// TestTheNoticesParserReadsWhatIsThere. The checks above are only as good as
// the reading: an entry the parser missed is an entry nothing checks. So the
// titles it read are the titles the file has.
func TestTheNoticesParserReadsWhatIsThere(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(root, noticesFile))
	if err != nil {
		t.Fatal(err)
	}
	quoted := strings.Count(string(data), "\nQuoted ")
	entries := readNotices(t)
	n := 0
	var titles []string
	for _, e := range entries {
		n += len(e.quotes)
		titles = append(titles, e.title)
		if len(e.files) == 0 || e.source == "" {
			t.Errorf("the entry %q has no files or no source this could read", e.title)
		}
	}
	if n != quoted {
		t.Errorf("the file has %d quotations and %d were read", quoted, n)
	}
	if rules := strings.Count(string(data), "\n"+strings.Repeat("=", 76)+"\n"); rules != 2*len(entries) {
		t.Errorf("the file has %d rules, which is %d entries, and %d were read: %v",
			rules, rules/2, len(entries), titles)
	}
}
