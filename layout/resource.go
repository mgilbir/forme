package layout

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mgilbir/forme/internal/ascii"
)

// Resolving the references a document makes to things outside itself.
//
// This is the first and only place where the engine can read anything the
// caller did not hand it, and it is written as a policy rather than as a
// convenience. Resolution is the caller's job and the engine has no network;
// what is here is the shape that makes both enforceable rather than merely
// intended.
//
// # Deny by default
//
// An Input with no resolver loads nothing. There is no "well, it looks like a
// path, so read it" fallback, and there is no flag that turns one on: a caller
// who wants an <img> to draw something, or a <link rel=stylesheet> to style
// anything, must say where the bytes may come from. That is the difference
// between a template renderer and a file-disclosure primitive, because the
// documents this engine renders are untrusted — an invoice template from a
// customer, a report body from a form — and "src" and "href" are strings in one
// of them.
//
// # No network, at any level
//
// A reference with a scheme is refused here rather than passed to the resolver,
// so a caller cannot accidentally implement one by writing a resolver that hands
// the string to something else. An HTML-to-PDF engine that fetches URLs is a
// server-side request forgery primitive with a friendly interface: the attacker
// writes <img src="http://169.254.169.254/...">, the server fetches it, and the
// only question left is whether the response is visible in the PDF. This one
// does not fetch, and the refusal is reported so that a document relying on it
// is not silently blank.
//
// A "data:" reference is the exception and is not an exception to anything that
// matters: its bytes are in the document already, so honouring it reads nothing
// the caller did not supply. It is bounded by the same caps as everything else.

// ResourceResolver turns a reference written in a document into bytes.
//
// # What is done with the error, and when it is called
//
// The text of a returned error is put into a finding verbatim: "the image at
// "x.png" was not loaded: " and then whatever Error() said. Findings are what a
// document's *author* is shown, so a resolver's error text is author-facing —
// and a resolver that writes an absolute path, a host name or a credential into
// one has published it to whoever wrote the document. Say what the author can
// act on and nothing else; the caller's own logging is where the rest belongs.
//
// Resolve is called from the goroutine that called Build or Layout, never from
// one of this package's, because this package starts none. A resolver therefore
// need not be safe for concurrent use — unless the caller renders two documents
// at once and hands both the same resolver, which is the caller's arrangement to
// make safe. The same is true of a Recorder: one render's is its own.
//
// It is deliberately not an io.Reader factory or a URL fetcher. A resolver is
// handed a reference relative to the document and returns the whole resource
// or an error. Returning an error is normal: a missing image is a finding, not
// a failure of the render.
//
// Relative to the document, because that is the one base a resolver can know.
// A reference written in the markup is handed over as written, or joined onto
// the document's <base href> first where it has one — <img src="a.png"> under
// <base href="img/"> is handed over as "img/a.png"; see base.go, which is also
// where a base that would take a reference outside this boundary is refused.
// One written in a stylesheet is relative to that stylesheet (CSS Values 4
// §4.5.1), and is resolved against the sheet's name first — "url(f.ttf)" in
// "css/a.css" is handed over as "css/f.ttf" — which is why Stylesheet.Name is
// a path; a <style> element's and a style attribute's are the document's, and
// so relative to its <base>. Either way it is read the way the URL standard
// reads a reference first; see below.
//
// # What the engine has already refused, and what it has not
//
// Two things, and they are one boundary: a reference naming a scheme never
// reaches a resolver, and neither does a reference naming a host. That refusal
// is not a convenience, it is the boundary — an engine that fetches URLs is a
// server-side request forgery primitive with a friendly interface, and it must
// not be possible to build one by writing a resolver that hands the string to
// something that does.
//
// The host is the half that is easy to miss. "//169.254.169.254/latest" names
// no scheme, and it is not a path either: the URL standard reads it as a
// *network-path reference*, a host and a path on it, and a resolver that did
// the natural thing with the paragraph below — resolve the reference against
// the URL its document was served from, with net/url's ResolveReference — would
// hand back "https://169.254.169.254/latest" and fetch it. The same is true of
// "\\host\x" and "/\\host", because the URL standard reads a backslash as a
// slash in every URL a document is served from. So a reference whose first two
// characters are slashes of either kind is refused here, with the schemes.
//
// Both are asked of the reference as the URL standard reads it, which is not
// quite the string the document wrote: the standard strips control characters
// and spaces from either end, and removes every tab and newline wherever they
// are, before it looks for a scheme. "ht\ttp://evil/" is "http://evil/" to
// every URL parser there is, so it is refused as that, and a resolver is handed
// what the standard reads — see referenceText.
//
// Everything else arrives. In particular a resolver **is** handed
// "/etc/passwd" and "../../secrets/id_rsa" when a document writes them, because
// neither is refusable here without refusing an ordinary document: "/css/x.png"
// is a reference to the root of wherever the document is served from, and
// "../images/logo.png" is a reference to a sibling directory, and only the
// caller knows where either of those is or whether it is allowed. A browser
// resolves both.
//
// So the containment is the resolver's, and it needs to be real containment.
// os.ReadFile(filepath.Join(dir, ref)) is not: it walks out of dir on a "..",
// it reads anywhere on an absolute path, and it follows a symbolic link inside
// dir that points out of it — which no check on the name can see, because the
// name says nothing about what it resolves to. DirResolver uses os.Root, which
// resolves every path component at the system call; a resolver written by hand
// should do the same, and one that cannot should refuse rather than guess.
//
// A resolver must also bound what it returns. The engine caps what it will
// decode, but it cannot cap what a resolver allocates before returning, so a
// resolver reading from anywhere unbounded has to impose its own limit.
// DirResolver does.
type ResourceResolver interface {
	// Resolve returns the bytes of the resource a document referred to.
	Resolve(ref string) ([]byte, error)
}

// ErrNoResolver is what the engine reports when a document refers to something
// and the caller configured no resolver.
var ErrNoResolver = errors.New("no resource resolver is configured, so nothing outside the document can be loaded")

// maxResourceBytes is the largest resource DirResolver will read.
//
// It bounds the read itself rather than the decode: a file this size is
// allocated whole before anything looks at it, so a cap applied afterwards is a
// cap applied too late. Sixteen megabytes is several times the largest image
// anyone puts in a document and small enough that a directory full of them
// cannot exhaust a server.
//
// It is a variable rather than a constant so that a test can lower it far
// enough to watch it fire without writing sixteen megabytes to a disk. A bound
// that has only ever been observed not to trip is one nobody knows works.
var maxResourceBytes int64 = 16 << 20

// DirResolver serves files from one directory and from nowhere else.
//
// Containment is enforced by os.Root, which resolves every path component
// against the directory at the system-call level: a reference with "..", an
// absolute path, and a symbolic link pointing outside the directory are all
// refused by the kernel rather than by a string comparison this code performs.
// That distinction is the whole reason os.Root exists — a check on the name
// followed by an open is a race, and a check on the *resolved* name is one
// symlink away from being wrong.
//
// A resolver holds an open directory handle and should be closed when the
// caller is done with it.
type DirResolver struct {
	root *os.Root
	// max is the largest file this will read, defaulting to maxResourceBytes.
	max int64
}

// NewDirResolver opens dir as the only place a document may read from.
func NewDirResolver(dir string) (*DirResolver, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("layout: rooting a resource resolver at %s: %w", dir, err)
	}
	return &DirResolver{root: root, max: maxResourceBytes}, nil
}

// WithMaxBytes returns the resolver with a different size cap. A cap of zero or
// less is refused rather than treated as "no limit": an unbounded resolver is
// the thing this type exists to prevent, and a zero value in a configuration
// struct must not switch it off.
func (d *DirResolver) WithMaxBytes(n int64) *DirResolver {
	if n <= 0 {
		return d
	}
	d.max = n
	return d
}

// Close releases the directory handle.
func (d *DirResolver) Close() error {
	if d == nil || d.root == nil {
		return nil
	}
	return d.root.Close()
}

// Resolve reads one file from the rooted directory.
func (d *DirResolver) Resolve(ref string) ([]byte, error) {
	if d == nil || d.root == nil {
		return nil, ErrNoResolver
	}
	name, err := resourcePath(ref)
	if err != nil {
		return nil, err
	}

	f, err := d.root.Open(name)
	if err != nil {
		// os.Root's error already says "path escapes from parent" for the
		// containment failures, which is the sentence a caller needs to see.
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		// A device, a socket or a named pipe. Reading /dev/zero through a
		// resolver rooted at /dev would produce bytes for ever, and reading a
		// fifo would block the render until something wrote to it — neither is
		// a file the cap below can save us from, because neither has a size.
		return nil, fmt.Errorf("%s is not a regular file", name)
	}
	max := d.max
	if max <= 0 {
		max = maxResourceBytes
	}
	if info.Size() > max {
		return nil, fmt.Errorf("%s is %d bytes, larger than the %d this engine will read",
			name, info.Size(), max)
	}

	// Read through a limit even though the size was checked. The two are not
	// the same guarantee: the size came from a stat and the file may grow
	// between the stat and the read, and on some filesystems a stat size is a
	// hint rather than a fact.
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("%s is larger than the %d bytes this engine will read", name, max)
	}
	return data, nil
}

// resourcePath turns a reference written in a document into a relative path,
// refusing everything that is not one.
//
// The refusals are the point, so each is named rather than folded into a single
// "invalid":
//
//   - A scheme means a URL. Nothing here fetches one, and reading "file:" as a
//     path would put the scheme back by another spelling.
//   - Two leading slashes name a host, which is a URL without its scheme. See
//     the note on ResourceResolver.
//   - A leading slash is an absolute path, which is a reference to the
//     filesystem rather than to the document's own directory.
//   - A path component of ".." asks to leave the directory. os.Root refuses it
//     too; it is refused here as well so that the reason reported is the one
//     the author needs, and so the containment does not rest on one mechanism.
//
// The reference is read as the URL standard reads it first — see
// referenceText — so that the tab in "ht\ttp:" cannot hide a scheme from this
// any more than from the engine.
//
// The query and fragment are dropped, as a browser drops them when a URL
// resolves to a file. Per-cent escapes are decoded, because a document that
// writes "blue%2015.png" means a file with a space in it — and decoded the way
// the URL standard decodes them, which leaves a "%" that begins no escape as
// the "%" it is: "100%.png" is a file name, not a malformed reference.
func resourcePath(ref string) (string, error) {
	ref = referenceText(ref)
	if ref == "" {
		return "", errors.New("the reference is empty")
	}
	if scheme, ok := schemeOf(ref); ok {
		return "", fmt.Errorf("%q names the %q scheme; this engine resolves no URLs", ref, scheme)
	}
	if namesAHost(ref) {
		return "", fmt.Errorf("%q names a host; this engine resolves no URLs", ref)
	}
	if i := strings.IndexAny(ref, "?#"); i >= 0 {
		ref = ref[:i]
	}
	if ref == "" {
		return "", errors.New("the reference has no path")
	}
	ref = percentDecode(ref)

	if strings.HasPrefix(ref, "/") || strings.HasPrefix(ref, `\`) {
		return "", fmt.Errorf("%q is an absolute path; a document may only refer to its own directory", ref)
	}
	// Both separators, because a document written on Windows uses the other one
	// and the check must not depend on which platform is reading it.
	for _, part := range strings.FieldsFunc(ref, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == ".." {
			return "", fmt.Errorf("%q leaves the directory it may read from", ref)
		}
	}
	return ref, nil
}

// referenceText is a reference as the URL standard reads it before it reads
// anything else: with the C0 control characters and spaces at either end
// stripped, and every ASCII tab and newline removed wherever it is (the URL
// standard's basic URL parser, steps 1 to 3).
//
// It is what every refusal in this file is asked of, and what a resolver is
// handed, because it is what every URL parser a resolver might use will read.
// Asked of the string as written instead, "ht\ttp://evil/" has no scheme —
// the tab stops schemeOf at the second letter — and reached a resolver, whose
// own parser would strip the tab and fetch it. The same stripping is why a
// data: URL an attribute wraps across lines is one URL.
func referenceText(ref string) string {
	start, end := 0, len(ref)
	for start < end && ref[start] <= ' ' {
		start++
	}
	for end > start && ref[end-1] <= ' ' {
		end--
	}
	ref = ref[start:end]
	if strings.IndexAny(ref, "\t\n\r") < 0 {
		return ref
	}
	// Byte by byte rather than strings.Map, which would also turn every byte
	// that is not UTF-8 into U+FFFD — and a data: URL is bytes.
	out := make([]byte, 0, len(ref))
	for i := 0; i < len(ref); i++ {
		if c := ref[i]; c != '\t' && c != '\n' && c != '\r' {
			out = append(out, c)
		}
	}
	return string(out)
}

// namesAHost reports whether a reference is what RFC 3986 calls a
// network-path reference: two slashes, and then a host.
//
// Either slash may be a backslash, because the URL standard reads one as a
// slash in every scheme a document is served over — http, https and file —
// and a resolver resolving against such a base would reach the host whichever
// was written.
func namesAHost(ref string) bool {
	isSlash := func(c byte) bool { return c == '/' || c == '\\' }
	return len(ref) >= 2 && isSlash(ref[0]) && isSlash(ref[1])
}

// percentDecode is the URL standard's percent-decode: every "%" followed by two
// hexadecimal digits is the byte they spell, and every other byte — a "%"
// that begins no escape among them — is itself.
//
// It is not net/url's PathUnescape, which refuses the whole string at the
// first "%" it cannot read. "100%" in a hand-written SVG data: URL is the
// ordinary case, and every browser draws it.
func percentDecode(s string) string {
	i := strings.IndexByte(s, '%')
	if i < 0 {
		return s
	}
	out := make([]byte, 0, len(s))
	out = append(out, s[:i]...)
	for ; i < len(s); i++ {
		c := s[i]
		if c == '%' && i+2 < len(s) && isHexDigit(s[i+1]) && isHexDigit(s[i+2]) {
			out = append(out, hexValue(s[i+1])<<4|hexValue(s[i+2]))
			i += 2
			continue
		}
		out = append(out, c)
	}
	return string(out)
}

func isHexDigit(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func hexValue(c byte) byte {
	switch {
	case c >= 'a':
		return c - 'a' + 10
	case c >= 'A':
		return c - 'A' + 10
	}
	return c - '0'
}

// fetchReference is resource.go's policy applied to one reference, and the
// one place it is applied: an <img>, a background, a list marker, generated
// content, an <object>, a <link>, an @import and an @font-face all read through
// it. Three loaders each had their own copy of the same three answers, and a
// boundary kept three times is one that a change to it has to find three times.
//
// The answers, in order: a data: URL is decoded from the document's own bytes;
// any other scheme, and any host, is refused; and everything else goes to the
// resolver, or is refused when there is none. What is handed to the resolver is
// referenceText, the reference as a URL parser reads it.
//
// what names the thing being read in a finding — "image", "stylesheet" — and
// notDone says what did not happen because of a refusal: "so it was not
// drawn". bad is the rule a data: URL that cannot be decoded is reported
// under, which differs by what it was meant to be. The type a data: URL
// declares is returned beside its bytes, and is empty for a resolver's, which
// declare none.
//
// An empty result from a resolver is returned as it is, because whether it is
// a failure depends on what was asked for: an empty stylesheet is a stylesheet
// with no rules, and an empty image is not an image.
func fetchReference(res ResourceResolver, src, what, notDone string, bad Rule) ([]byte, string, *loadFailure) {
	ref := referenceText(src)
	if scheme, ok := schemeOf(ref); ok {
		if scheme == "data" {
			return decodeDataURI(ref, what, bad)
		}
		return nil, "", &loadFailure{
			rule: RuleResourceBlocked,
			message: "the " + what + " at " + quoteValue(src) + " names the " + quoteValue(scheme) +
				" scheme; this engine resolves no URLs and fetches nothing, " + notDone,
		}
	}
	if namesAHost(ref) {
		return nil, "", &loadFailure{
			rule: RuleResourceBlocked,
			message: "the " + what + " at " + quoteValue(src) + " names a host, which is a URL " +
				"with its scheme left out; this engine resolves no URLs and fetches nothing, " + notDone,
		}
	}
	if res == nil {
		return nil, "", &loadFailure{
			rule:    RuleResourceBlocked,
			message: "the " + what + " at " + quoteValue(src) + " was not loaded: " + ErrNoResolver.Error(),
		}
	}
	data, err := res.Resolve(ref)
	if err != nil {
		return nil, "", &loadFailure{
			rule:    RuleResourceBlocked,
			message: "the " + what + " at " + quoteValue(src) + " was not loaded: " + err.Error(),
		}
	}
	return data, "", nil
}

// schemeOf reports whether a reference begins with a URL scheme.
//
// The grammar is RFC 3986's: a letter followed by letters, digits, "+", "-" and
// ".", then a colon. A Windows drive letter — "c:/x" — matches it, and being
// refused as a scheme is the right outcome for that too.
func schemeOf(ref string) (string, bool) {
	for i := 0; i < len(ref); i++ {
		c := ref[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
			continue
		case c >= '0' && c <= '9', c == '+', c == '-', c == '.':
			if i == 0 {
				return "", false
			}
			continue
		case c == ':':
			if i == 0 {
				return "", false
			}
			return ascii.Lower(ref[:i]), true
		}
		return "", false
	}
	return "", false
}
