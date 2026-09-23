package layout

// The user-agent stylesheet: what makes a <p> a block and a <b> bold before any
// author has said anything.
//
// It is written as CSS and parsed like any other stylesheet rather than being
// compiled into a table of defaults, and that is worth a sentence. A table would
// be faster to load and would put these rules outside the cascade — so an author
// writing "p { display: inline }" would be fighting something that is not a
// stylesheet, and the origin ordering that makes their rule win would have
// nothing to order. Keeping it as CSS means the defaults lose to the author for
// exactly the reason everything else does.
//
// It is deliberately smaller than a browser's. A browser's has to style
// elements this engine refuses — form controls, media, the interactive ones —
// and rules for those would be dead weight that reads as coverage.

// UserAgentCSS is the default stylesheet.
//
// The lengths are the ones every browser converged on, and the em-based ones
// are em on purpose: a document that sets a larger font gets proportionally
// larger spacing, which is what an author expects and what a fixed pixel margin
// quietly fails to do.
const UserAgentCSS = `
/* The elements that produce no box at all: HTML's rendering section, §15.3.1,
   less the three kept elsewhere in this sheet (area and param below, and rp,
   which is the one this engine keeps on purpose — see the ruby rules). A
   <datalist> is the list of suggestions a text field offers as it is typed
   into, and its options are not content: it was drawn, so every suggestion
   appeared as a line of text (audit C84). */
head, title, meta, link, base, style, script, template,
datalist, basefont, noembed, noframes { display: none }

/* And the attribute that says so about any element, §15.3.1. It was not read at
   all, so "<div hidden>" was a visible div — which is the ordinary way a
   document hides something and the one this engine was most likely to meet.

   "until-found" is the value that means "hidden, but findable by the browser's
   own search", and there is nothing to search a printed page with; the element
   is laid out, which is what a browser does with it once the search has found
   it. The rule is HTML's own, flag for flag. */
[hidden]:not([hidden=until-found i]) { display: none }

/* Block-level structure. */
html, body, div, p, blockquote, figure, figcaption, address,
header, footer, nav, section, article, aside, main, hgroup,
h1, h2, h3, h4, h5, h6, ul, ol, dl, dt, dd, pre, hr { display: block }

li { display: list-item; counter-increment: list-item }
/* Each list creates its own counter, which is what makes a nested list start
   again at one while the list around it carries on. "list-item" is the name CSS
   Lists reserves for exactly this. */
ol, ul, menu, dir { counter-reset: list-item }

/* Tables. The display values are the ones the table algorithm keys off, so
   these are not decoration: a <tr> that were left inline would not be a row. */
table { display: table; border-collapse: separate; border-spacing: 2px }
caption { display: table-caption; text-align: center }
colgroup { display: table-column-group }
col { display: table-column }
thead { display: table-header-group }
tbody { display: table-row-group }
tfoot { display: table-footer-group }
tr { display: table-row }
td, th { display: table-cell; padding: 1px }
th { font-weight: bold; text-align: center }
/* A cell's vertical alignment comes down the table rather than from the
   property's own initial value, which is HTML's rendering section 15.3.8 and
   the reason a cell's content sits in the middle of a tall row by default:

       thead, tbody, tfoot, table > tr { vertical-align: middle }
       tr, td, th                      { vertical-align: inherit }

   The inherit is what carries an author's "vertical-align: top" on a row to
   the cells in it — vertical-align does not inherit on its own, so a UA rule
   naming the cells directly would win over the author's rule on the row and
   the declaration would do nothing. */
thead, tbody, tfoot, table > tr { vertical-align: middle }
tr, td, th { vertical-align: inherit }

/* Vertical rhythm. Margins are em so that spacing follows the type size. */
p, blockquote, figure, ul, ol, dl, pre { margin-top: 1em; margin-bottom: 1em }
blockquote { margin-left: 40px; margin-right: 40px }
figure { margin-left: 40px; margin-right: 40px }
ul, ol { padding-left: 40px }
dd { margin-left: 40px }

h1 { font-size: 2em; margin-top: 0.67em; margin-bottom: 0.67em; font-weight: bold }
h2 { font-size: 1.5em; margin-top: 0.83em; margin-bottom: 0.83em; font-weight: bold }
h3 { font-size: 1.17em; margin-top: 1em; margin-bottom: 1em; font-weight: bold }
h4 { font-size: 1em; margin-top: 1.33em; margin-bottom: 1.33em; font-weight: bold }
h5 { font-size: 0.83em; margin-top: 1.67em; margin-bottom: 1.67em; font-weight: bold }
h6 { font-size: 0.67em; margin-top: 2.33em; margin-bottom: 2.33em; font-weight: bold }

body { margin-top: 8px; margin-right: 8px; margin-bottom: 8px; margin-left: 8px }

/* §15.3.6's rule, all of it. The colour is the part that shows on every <hr>
   ever drawn: border-color defaults to currentcolor, so the rule's own colour
   *is* its border's, and without this line every horizontal rule was drawn in
   the colour it inherited — black in almost every document, where a browser
   draws grey. The overflow is what keeps a rule shorter than its content from
   being pushed open by it. */
hr {
  color: gray;
  overflow-x: hidden; overflow-y: hidden;
  margin-top: 0.5em; margin-bottom: 0.5em;
  margin-left: auto; margin-right: auto;
  border-top-width: 1px; border-right-width: 1px;
  border-bottom-width: 1px; border-left-width: 1px;
  border-top-style: inset; border-right-style: inset;
  border-bottom-style: inset; border-left-style: inset;
}

/* The align attribute, §15.3.2, which is how a document said "centre this"
   before text-align existed. The element lists are HTML's own and are not a
   family that can be shortened: <legend> and <caption> are deliberately absent
   from all four, and <table align> is not alignment at all but a float.

   The "i" is HTML's too, and is redundant with the folding of an enumerated
   value — it is kept because the rule is quoted rather than derived, and a
   reader comparing the two should find them the same. */
center,
div[align=center i], div[align=middle i],
p[align=center i], h1[align=center i], h2[align=center i], h3[align=center i],
h4[align=center i], h5[align=center i], h6[align=center i],
thead[align=center i], tbody[align=center i], tfoot[align=center i],
tr[align=center i], td[align=center i], th[align=center i] { text-align: center }

div[align=left i],
p[align=left i], h1[align=left i], h2[align=left i], h3[align=left i],
h4[align=left i], h5[align=left i], h6[align=left i],
thead[align=left i], tbody[align=left i], tfoot[align=left i],
tr[align=left i], td[align=left i], th[align=left i] { text-align: left }

div[align=right i],
p[align=right i], h1[align=right i], h2[align=right i], h3[align=right i],
h4[align=right i], h5[align=right i], h6[align=right i],
thead[align=right i], tbody[align=right i], tfoot[align=right i],
tr[align=right i], td[align=right i], th[align=right i] { text-align: right }

div[align=justify i],
p[align=justify i], h1[align=justify i], h2[align=justify i], h3[align=justify i],
h4[align=justify i], h5[align=justify i], h6[align=justify i],
thead[align=justify i], tbody[align=justify i], tfoot[align=justify i],
tr[align=justify i], td[align=justify i], th[align=justify i] { text-align: justify }

/* And on an <hr>, where align is not about the text but about which way the
   rule is pushed. §15.3.6.

   "color" and "noshade" both mean "draw this as a line rather than as a
   groove", which is the one part of the <hr> attributes that is a selector: the
   rest is arithmetic on the size attribute and is in style/hints.go. */
hr[color], hr[noshade] { border-style: solid }
hr[align=left i] { margin-left: 0; margin-right: auto }
hr[align=right i] { margin-left: auto; margin-right: 0 }
hr[align=center i] { margin-left: auto; margin-right: auto }

/* The align attribute on a replaced box, §15.3.5, which is how a document put a
   picture beside its text before there was a float property to say it with.
   "<img align=left>" is the oldest illustration layout there is.

   The element list is HTML's own and is wider than <img>: an <iframe>, an
   <object>, an <embed> and an <input type=image> are all boxes a document could
   align this way, and all four are boxes this engine lays out.

   "center" and "middle" are not here and are not a transcription this left out.
   The specification states those two as prose rather than as CSS — the
   element's vertical middle against the parent's *baseline* — and that is not
   "vertical-align: middle", which is the baseline plus half an x-height. A rule
   written from the value's name rather than from the sentence would be a guess
   at a position, which is the one thing a box's position must not be. */
embed[align=left i], iframe[align=left i], img[align=left i],
input[type=image i][align=left i], object[align=left i] { float: left }

embed[align=right i], iframe[align=right i], img[align=right i],
input[type=image i][align=right i], object[align=right i] { float: right }

embed[align=top i], iframe[align=top i], img[align=top i],
input[type=image i][align=top i], object[align=top i] { vertical-align: top }

embed[align=baseline i], iframe[align=baseline i], img[align=baseline i],
input[type=image i][align=baseline i], object[align=baseline i] { vertical-align: baseline }

/* The frame and rules attributes, §15.3.8, which are how a table said which of
   its edges were drawn before there was a border property to say it with.

   This is HTML's own CSS, transcribed. The selector lists are long because the
   specification writes out both the tbody-less shape and the three section
   shapes for every value, and they are kept that way rather than shortened: the
   parser inserts a <tbody> around a bare <tr>, so "table > tr" matches nothing
   in a document this engine parsed, and a list written from what *should*
   match rather than from what the specification says is a list that drifts.

   A "rules" value puts the table in the collapsed border model, which is the
   only one where a row's or a section's border is drawn at all — CSS 2.1
   §17.6.1 ignores both in the separated model, which is why rules=groups needs
   the collapse and not only the borders. */
table[rules=none i], table[rules=groups i], table[rules=rows i],
table[rules=cols i], table[rules=all i],
table[frame=void i], table[frame=above i], table[frame=below i],
table[frame=hsides i], table[frame=lhs i], table[frame=rhs i],
table[frame=vsides i], table[frame=box i], table[frame=border i],
table[rules=none i] > tr > td, table[rules=none i] > tr > th,
table[rules=groups i] > tr > td, table[rules=groups i] > tr > th,
table[rules=rows i] > tr > td, table[rules=rows i] > tr > th,
table[rules=cols i] > tr > td, table[rules=cols i] > tr > th,
table[rules=all i] > tr > td, table[rules=all i] > tr > th,
table[rules=none i] > thead > tr > td, table[rules=none i] > thead > tr > th,
table[rules=groups i] > thead > tr > td, table[rules=groups i] > thead > tr > th,
table[rules=rows i] > thead > tr > td, table[rules=rows i] > thead > tr > th,
table[rules=cols i] > thead > tr > td, table[rules=cols i] > thead > tr > th,
table[rules=all i] > thead > tr > td, table[rules=all i] > thead > tr > th,
table[rules=none i] > tbody > tr > td, table[rules=none i] > tbody > tr > th,
table[rules=groups i] > tbody > tr > td, table[rules=groups i] > tbody > tr > th,
table[rules=rows i] > tbody > tr > td, table[rules=rows i] > tbody > tr > th,
table[rules=cols i] > tbody > tr > td, table[rules=cols i] > tbody > tr > th,
table[rules=all i] > tbody > tr > td, table[rules=all i] > tbody > tr > th,
table[rules=none i] > tfoot > tr > td, table[rules=none i] > tfoot > tr > th,
table[rules=groups i] > tfoot > tr > td, table[rules=groups i] > tfoot > tr > th,
table[rules=rows i] > tfoot > tr > td, table[rules=rows i] > tfoot > tr > th,
table[rules=cols i] > tfoot > tr > td, table[rules=cols i] > tfoot > tr > th,
table[rules=all i] > tfoot > tr > td, table[rules=all i] > tfoot > tr > th {
  border-top-color: black; border-right-color: black;
  border-bottom-color: black; border-left-color: black;
}

table[frame=void i] { border-style: hidden }
table[frame=above i] { border-style: outset hidden hidden hidden }
table[frame=below i] { border-style: hidden hidden outset hidden }
table[frame=hsides i] { border-style: outset hidden outset hidden }
table[frame=lhs i] { border-style: hidden hidden hidden outset }
table[frame=rhs i] { border-style: hidden outset hidden hidden }
table[frame=vsides i] { border-style: hidden outset }
table[frame=box i], table[frame=border i] { border-style: outset }

table[rules=none i], table[rules=groups i], table[rules=rows i],
table[rules=cols i], table[rules=all i] {
  border-style: hidden; border-collapse: collapse;
}

table[rules=groups i] > colgroup {
  border-inline-width: 1px; border-inline-style: solid;
}
table[rules=groups i] > thead,
table[rules=groups i] > tbody,
table[rules=groups i] > tfoot {
  border-block-width: 1px; border-block-style: solid;
}

table[rules=rows i] > tr, table[rules=rows i] > thead > tr,
table[rules=rows i] > tbody > tr, table[rules=rows i] > tfoot > tr {
  border-block-width: 1px; border-block-style: solid;
}

table[rules=none i] > tr > td, table[rules=none i] > tr > th,
table[rules=none i] > thead > tr > td, table[rules=none i] > thead > tr > th,
table[rules=none i] > tbody > tr > td, table[rules=none i] > tbody > tr > th,
table[rules=none i] > tfoot > tr > td, table[rules=none i] > tfoot > tr > th,
table[rules=groups i] > tr > td, table[rules=groups i] > tr > th,
table[rules=groups i] > thead > tr > td, table[rules=groups i] > thead > tr > th,
table[rules=groups i] > tbody > tr > td, table[rules=groups i] > tbody > tr > th,
table[rules=groups i] > tfoot > tr > td, table[rules=groups i] > tfoot > tr > th,
table[rules=rows i] > tr > td, table[rules=rows i] > tr > th,
table[rules=rows i] > thead > tr > td, table[rules=rows i] > thead > tr > th,
table[rules=rows i] > tbody > tr > td, table[rules=rows i] > tbody > tr > th,
table[rules=rows i] > tfoot > tr > td, table[rules=rows i] > tfoot > tr > th {
  border-width: 1px; border-style: none;
}

table[rules=cols i] > tr > td, table[rules=cols i] > tr > th,
table[rules=cols i] > thead > tr > td, table[rules=cols i] > thead > tr > th,
table[rules=cols i] > tbody > tr > td, table[rules=cols i] > tbody > tr > th,
table[rules=cols i] > tfoot > tr > td, table[rules=cols i] > tfoot > tr > th {
  border-width: 1px; border-block-style: none; border-inline-style: solid;
}

table[rules=all i] > tr > td, table[rules=all i] > tr > th,
table[rules=all i] > thead > tr > td, table[rules=all i] > thead > tr > th,
table[rules=all i] > tbody > tr > td, table[rules=all i] > tbody > tr > th,
table[rules=all i] > tfoot > tr > td, table[rules=all i] > tfoot > tr > th {
  border-width: 1px; border-style: solid;
}

/* Lists. */
ul { list-style-type: disc }
ol { list-style-type: decimal }

/* The type attribute, which is how a list said which counter it wanted before
   list-style-type existed and is still how half the older documents say it.
   HTML's rendering section §15.3.7 gives these nine rules and this is them,
   spelling for spelling.

   The "s" on the ordered ones is the point of them: "type" is one of the
   attributes HTML compares ASCII case-insensitively, so without the flag
   "[type=a]" and "[type=A]" would be one selector and "<ol type=A>" would be
   numbered in lower case. The unordered ones take "i" for the same reason in
   reverse — "SQUARE" is a square. */
ol[type="1"], li[type="1"] { list-style-type: decimal }
ol[type=a s], li[type=a s] { list-style-type: lower-alpha }
ol[type=A s], li[type=A s] { list-style-type: upper-alpha }
ol[type=i s], li[type=i s] { list-style-type: lower-roman }
ol[type=I s], li[type=I s] { list-style-type: upper-roman }
ul[type=none i], li[type=none i] { list-style-type: none }
ul[type=disc i], li[type=disc i] { list-style-type: disc }
ul[type=circle i], li[type=circle i] { list-style-type: circle }
ul[type=square i], li[type=square i] { list-style-type: square }

/* Text-level semantics. */
b, strong { font-weight: bold }
i, em, cite, var, dfn, address { font-style: italic }
code, kbd, samp, pre, tt { font-family: monospace }
pre { white-space: pre }

/* CSS Text 4's own default sheet turns the ideograph spacing off inside the
   elements whose content is preformatted. The spacing is a typesetter's, and
   what these elements hold is text quoted exactly: a fragment of program source
   is not prose, and putting an eighth of an em between a letter and an
   ideograph in it changes what the reader is being shown.

   text-autospace-preformatted-001 is the five below beside two paragraphs that
   do take the spacing. A <textarea> and an <input> are the same case for the
   same reason — what a control shows is the user's own text — and no test in
   the suite reaches them. */
pre, code, kbd, samp, tt, textarea, input { text-autospace: no-autospace }
small { font-size: 0.83em }
sub, sup { font-size: 0.83em }
sub { vertical-align: sub }
sup { vertical-align: super }
u, ins { text-decoration-line: underline }
s, del, strike { text-decoration-line: line-through }
/* The obsolete presentational elements, from HTML's own rendering section. */
big { font-size: larger }
nobr { white-space: nowrap }
center { display: block; text-align: center }
mark { background-color: yellow; color: black }
/* Links, and only links. HTML's rendering section writes this as ":link,
   :visited", and :link matches an <a> *that has an href* — an <a> without one
   is an anchor and not a link, and browsers leave it the colour of its
   surroundings. Writing it as a bare "a" was measurably wrong: the suite is full
   of empty <a> elements used as a place to hang a pseudo-element on, and every
   one of them came out blue and underlined against a reference that had no such
   thing. */
a[href] { text-decoration-line: underline; color: #0000ee }

/* Ruby, as HTML's rendering section (§15.3.4) writes it. This engine does not
   lay ruby out — a "display: ruby" box is an inline one, and its annotation
   runs along the line — and pipeline.go reports that wherever a ruby holds an
   annotation. HTML's own <ruby> was given no display at all, so the same page
   written with the elements instead of the display values was set the same
   wrong way and said nothing (audit C84). The size is the part of the
   annotation this engine can express.

   <rp> is shown, although §15.3.1 hides it, and that is the one departure here
   from HTML's list. It holds the parentheses a user agent that cannot lay ruby
   out puts around the annotation, and this is such a user agent: "漢(kan)"
   says what "漢kan" does not. */
ruby { display: ruby }
rt { display: ruby-text; font-size: 0.5em; vertical-align: super }

/* Bidirectional overrides are the two elements whose whole purpose is to change
   the direction, so they say so rather than inheriting it. */
bdo { unicode-bidi: bidi-override }
bdi { unicode-bidi: isolate }

/* The dir attribute, which is how nearly every document in the world states a
   direction — a stylesheet saying so is the exception. HTML's own rendering
   section defines it as these declarations, and the isolation is part of the
   definition rather than an extra: an element that says which way it runs must
   not reorder the text around it.

   dir=auto is the first-strong rule over the element's own content, which is
   what unicode-bidi: plaintext is; it deliberately leaves direction alone,
   because the content decides. */
[dir="ltr"] { direction: ltr; unicode-bidi: isolate }
[dir="rtl"] { direction: rtl; unicode-bidi: isolate }
[dir="auto"] { unicode-bidi: plaintext }
bdo[dir] { unicode-bidi: isolate-override }

/* Form controls, as the static boxes a printed page has. control.go carries the
   half of this that CSS cannot say — an intrinsic size in characters and lines,
   and the text a control shows — and says what is approximated and reported.

   The display values and "white-space: pre-wrap" are HTML's rendering section.
   The chrome is not: the specification leaves a control's border, padding and
   colours to the user agent, and every one below is the shape desktop browsers
   converged on. It is here rather than left off because a text field with no
   border is invisible on paper, which is a page that has quietly lost a
   control rather than one that shows an approximate one. */
form, fieldset, legend, optgroup, option { display: block }
input, button, select, textarea { display: inline-block }
param { display: none }

/* HTML's own rendering section hides an <area>: it is markup about where a
   reader may click, and it draws nothing of itself. A rule and not a refusal,
   which is the difference the suite's content-100 turns on — it writes
   "area { display: block }" and asks for the generated content on the box that
   makes. */
area { display: none }

/* <slot> renders what it holds where no shadow tree fills it, and none ever
   does here. "display: contents" is HTML's own rule for it and is what makes
   the fallback content reach the page without the slot's own box. */
slot { display: contents }

/* <marquee> stands still on paper, which is what a browser asked to print one
   draws. HTML's rendering section gives it an inline-block. */
marquee { display: inline-block }
input[type="hidden" i] { display: none }

/* A <textarea> is preserved white space that wraps, which is the one rule in
   here that changes what the text says rather than how it looks. */
textarea { white-space: pre-wrap; overflow-x: auto; overflow-y: auto }

/* The text-entry chrome. A <select> is drawn as a field rather than as a
   drop-down, because the arrow is a widget and this is paper. */
input, textarea, select {
  border-top: 1px solid #767676; border-right: 1px solid #767676;
  border-bottom: 1px solid #767676; border-left: 1px solid #767676;
  padding-top: 1px; padding-right: 2px; padding-bottom: 1px; padding-left: 2px;
  background-color: #ffffff;
}

/* The push buttons, whose label is centred in a raised box. */
button,
input[type="submit" i], input[type="reset" i], input[type="button" i] {
  text-align: center;
  padding-top: 1px; padding-right: 6px; padding-bottom: 1px; padding-left: 6px;
  border-top: 2px outset #c0c0c0; border-right: 2px outset #c0c0c0;
  border-bottom: 2px outset #c0c0c0; border-left: 2px outset #c0c0c0;
  background-color: #f0f0f0;
}

/* A checkbox and a radio are a small fixed square. HTML sizes these from the
   border box, which is why the size does not move when the border does. */
input[type="checkbox" i], input[type="radio" i] {
  box-sizing: border-box; width: 13px; height: 13px;
  margin-top: 3px; margin-right: 3px; margin-bottom: 0; margin-left: 4px;
  padding-top: 0; padding-right: 0; padding-bottom: 0; padding-left: 0;
}

/* An image button is a picture, so it carries none of the field's chrome. */
input[type="image" i] {
  border-top-style: none; border-right-style: none;
  border-bottom-style: none; border-left-style: none;
  padding-top: 0; padding-right: 0; padding-bottom: 0; padding-left: 0;
  background-color: transparent;
}

/* §15.3.9. "ThreeDFace" is a system colour this engine has no palette for, so
   the grey every desktop resolves it to is written out. */
fieldset {
  margin-left: 2px; margin-right: 2px;
  border-top: 2px groove #c0c0c0; border-right: 2px groove #c0c0c0;
  border-bottom: 2px groove #c0c0c0; border-left: 2px groove #c0c0c0;
  padding-top: 0.35em; padding-right: 0.75em;
  padding-bottom: 0.625em; padding-left: 0.75em;
}
legend { padding-left: 2px; padding-right: 2px }

/* <form> deliberately has no margin. HTML's rendering section gives it
   "margin-block-end: 1em" in *quirks mode* only, and this engine has one
   document mode; taking the quirk would indent every standards-mode document
   by an em nothing asked for. */
`
