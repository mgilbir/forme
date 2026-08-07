package shape

import "testing"

// The metrics a font states rather than implies.
//
// Each of these is a number some other layer needs and had been guessing at: a
// line height without the leading term, an ex unit assumed to be half an em, an
// underline placed where it looked right across the standard fourteen. The
// values below are read out of the bundled face and checked against what
// fontTools reports for the same file, so a misread offset shows up here rather
// than as a rule drawn in the wrong place.

func TestDescriptorReadsTheMetricsTheFontStates(t *testing.T) {
	f, err := NotoSans()
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	d := f.Descriptor()
	for _, c := range []struct {
		name string
		got  int
		want int
	}{
		{"LineGap", d.LineGap, 0},
		{"TypoAscent", d.TypoAscent, 1069},
		{"TypoDescent", d.TypoDescent, -293},
		{"TypoLineGap", d.TypoLineGap, 0},
		{"XHeight", d.XHeight, 536},
		{"UnderlinePosition", d.UnderlinePosition, -100},
		{"UnderlineThickness", d.UnderlineThickness, 50},
		{"StrikeoutPosition", d.StrikeoutPosition, 322},
		{"StrikeoutSize", d.StrikeoutSize, 50},
		{"Weight", d.Weight, 400},
	} {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
	if !d.UseTypoMetrics {
		t.Error("UseTypoMetrics is false; this face sets OS/2 fsSelection bit 7")
	}
}

// TestZeroAndUnknownAreDifferentAnswers is the point of Declared.
//
// The bundled face states a line gap of nothing, and the fourteen standard faces
// state none at all — they have no hhea to state it in. Both report LineGap as
// 0, and a consumer that cannot tell them apart will read the second as an
// instruction. Declared is the only thing that distinguishes them, so it is
// checked in both directions.
func TestZeroAndUnknownAreDifferentAnswers(t *testing.T) {
	noto, err := NotoSans()
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	d := noto.Descriptor()
	if d.LineGap != 0 {
		t.Fatalf("this test assumes the bundled face declares a zero line gap, got %d", d.LineGap)
	}
	for _, m := range []struct {
		name string
		bit  Metric
	}{
		{"MetricLineGap", MetricLineGap}, {"MetricTypoMetrics", MetricTypoMetrics},
		{"MetricXHeight", MetricXHeight}, {"MetricCapHeight", MetricCapHeight},
		{"MetricUnderline", MetricUnderline}, {"MetricStrikeout", MetricStrikeout},
		{"MetricWeight", MetricWeight},
	} {
		if !d.Has(m.bit) {
			t.Errorf("the bundled face does not report %s, which it states", m.name)
		}
	}

	std, err := Standard("Helvetica")
	if err != nil {
		t.Fatalf("loading Helvetica: %v", err)
	}
	s := std.Descriptor()
	// The x-height and the underline come from the AFM; everything else needs a
	// table the face has not got.
	if want := MetricXHeight | MetricUnderline; s.Declared != want {
		t.Errorf("a standard face declares %b, want %b: an AFM publishes an "+
			"x-height and an underline and nothing else here", s.Declared, want)
	}
	if s.LineGap != 0 {
		t.Errorf("LineGap = %d for a standard face, want 0 with the bit clear", s.LineGap)
	}
	// The two answers a consumer has to tell apart, spelled out.
	if d.LineGap == s.LineGap && d.Has(MetricLineGap) == s.Has(MetricLineGap) {
		t.Error("a stated zero and an absent value are indistinguishable, which is the bug this guards")
	}
}

// TestTheStandardFourteenReportWhatTheirAFMSays covers the faces with no tables.
//
// They are the commonest case a consumer meets — a PDF may name one and embed
// nothing — and they are the case where "stated zero" and "stated nothing" is
// decided per metric rather than per face. An AFM publishes an underline for all
// fourteen and an x-height for twelve; Symbol and ZapfDingbats have no lowercase
// to measure one from. Neither an AFM nor anything else here carries a line gap
// or a strikeout for them, so those bits stay clear and the zero beside them is
// not an instruction.
//
// The numbers are Adobe's own, read out of the AFM files rather than out of this
// package, and every one of the fourteen is listed because the cost of doing so
// is a table and the alternative is a sample that happens to miss the face that
// regenerated wrongly.
func TestTheStandardFourteenReportWhatTheirAFMSays(t *testing.T) {
	// The AFM's UnderlinePosition is -100 with a thickness of 50 throughout,
	// measured to the centre of the stroke; the top of it, which is what
	// Descriptor reports, is -100 + 50/2 = -75.
	const underlineTop, underlineThickness = -75, 50
	xHeights := map[string]int{
		"Courier": 426, "Courier-Bold": 439, "Courier-BoldOblique": 439,
		"Courier-Oblique": 426, "Helvetica": 523, "Helvetica-Bold": 532,
		"Helvetica-BoldOblique": 532, "Helvetica-Oblique": 523,
		"Times-Roman": 450, "Times-Bold": 461, "Times-Italic": 441,
		"Times-BoldItalic": 462,
		// Symbol and ZapfDingbats are absent on purpose: their AFM has no
		// XHeight line at all, which is a different answer from XHeight 0.
	}
	names := StandardNames()
	if len(names) != 14 {
		t.Fatalf("%d standard faces, want 14", len(names))
	}
	for _, name := range names {
		f, err := Standard(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		d := f.Descriptor()

		want, stated := xHeights[name]
		if d.Has(MetricXHeight) != stated || d.XHeight != want {
			t.Errorf("%s x-height %d stated=%v, want %d stated=%v",
				name, d.XHeight, d.Has(MetricXHeight), want, stated)
		}
		if !d.Has(MetricUnderline) ||
			d.UnderlinePosition != underlineTop || d.UnderlineThickness != underlineThickness {
			t.Errorf("%s underline %d/%d stated=%v, want %d/%d stated",
				name, d.UnderlinePosition, d.UnderlineThickness, d.Has(MetricUnderline),
				underlineTop, underlineThickness)
		}
		if d.Has(MetricLineGap) || d.LineGap != 0 {
			t.Errorf("%s line gap %d stated=%v, and an AFM has none",
				name, d.LineGap, d.Has(MetricLineGap))
		}
		if d.Has(MetricStrikeout) || d.StrikeoutPosition != 0 || d.StrikeoutSize != 0 {
			t.Errorf("%s strikeout %d/%d stated=%v, and an AFM has none",
				name, d.StrikeoutPosition, d.StrikeoutSize, d.Has(MetricStrikeout))
		}
	}

	// The distinction the bit exists for, on the two faces that make it: Symbol
	// reports the same x-height as a face that measured one and found zero
	// would, and only the bit says which happened. Without it a caller sizing an
	// ex unit gets a zero it cannot recognise as silence.
	symbol, err := Standard("Symbol")
	if err != nil {
		t.Fatal(err)
	}
	if s := symbol.Descriptor(); s.XHeight != 0 || s.Has(MetricXHeight) {
		t.Errorf("Symbol x-height %d stated=%v, want 0 with the bit clear",
			s.XHeight, s.Has(MetricXHeight))
	}
}

// TestTheAFMUnderlineIsConvertedToPostsConvention pins the half-stroke.
//
// An AFM's UnderlinePosition is the centre of the stroke and post's is its top,
// so the two conventions differ by half a thickness and a reader that carried
// the AFM number through unchanged would draw every standard face's underline
// half a stroke low. The error is small, silent, and only visible beside a
// browser — which is why it is asserted against the published number here rather
// than against whatever this package computes.
func TestTheAFMUnderlineIsConvertedToPostsConvention(t *testing.T) {
	m, ok := standard14["Helvetica"]
	if !ok {
		t.Fatal("Helvetica is missing from the generated metrics")
	}
	if m.underlineCenter != -100 || m.underlineThickness != 50 {
		t.Fatalf("the generated AFM values are %d/%d, not the published -100/50; "+
			"this test no longer checks what it says it does",
			m.underlineCenter, m.underlineThickness)
	}
	f, err := Standard("Helvetica")
	if err != nil {
		t.Fatal(err)
	}
	got := f.Descriptor().UnderlinePosition
	if got == m.underlineCenter {
		t.Fatalf("the AFM's centre-of-stroke %d was reported unconverted; post's "+
			"convention is the top of the stroke", got)
	}
	if want := m.underlineCenter + m.underlineThickness/2; got != want {
		t.Errorf("underline position %d, want %d — the AFM's %d raised by half of "+
			"a %d-unit stroke", got, want, m.underlineCenter, m.underlineThickness)
	}
}

// TestAxesSayWhereTheOutlinesActuallySit covers what a variable face reports.
//
// The bundled face is variable, and what this module hands back is the outlines
// as stored — the default instance. Axes is how a caller finds that out; the
// default coordinate is where it landed.
func TestAxesSayWhereTheOutlinesActuallySit(t *testing.T) {
	f, err := NotoSans()
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if !f.IsVariable() {
		t.Fatal("the bundled face has fvar and is not reported as variable")
	}
	axes := f.Axes()
	want := map[string]Axis{
		"wght": {Tag: "wght", Min: 100, Default: 400, Max: 900},
		"wdth": {Tag: "wdth", Min: 62.5, Default: 100, Max: 100},
	}
	if len(axes) != len(want) {
		t.Fatalf("%d axes, want %d: %+v", len(axes), len(want), axes)
	}
	for _, a := range axes {
		w, ok := want[a.Tag]
		if !ok {
			t.Errorf("unexpected axis %q", a.Tag)
			continue
		}
		if a != w {
			t.Errorf("axis %s = %+v, want %+v", a.Tag, a, w)
		}
	}
	// wdth's range is the one that catches a 16.16 misread: 62.5 is not an
	// integer, so a reader that took the high word alone would say 62.
	for _, a := range axes {
		if a.Tag == "wdth" && a.Min != 62.5 {
			t.Errorf("wdth minimum = %v, want 62.5", a.Min)
		}
	}

	// Axes hands out a copy: a caller that scribbles on it must not reach the
	// face, which every document sharing this parse would then see.
	axes[0].Tag = "XXXX"
	if f.Axes()[0].Tag == "XXXX" {
		t.Error("Axes returned the face's own slice")
	}

	std, err := Standard("Helvetica")
	if err != nil {
		t.Fatalf("loading Helvetica: %v", err)
	}
	if std.IsVariable() || std.Axes() != nil {
		t.Error("a standard face reports variation axes")
	}
}
