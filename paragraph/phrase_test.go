package paragraph

import (
	"strings"
	"testing"
)

// The model against the answers BudouX's own parser gives, which is the whole
// of what "the same segmentation the references were written from" means.
//
// The expectations were taken from the reference implementation run over the
// same model file this table is generated from, so a disagreement here is this
// engine's scoring and not a difference of vocabulary. They are phrases and not
// offsets because a phrase is what a reader can check: "東京へ" is one and
// "東京" is not, and a list of byte offsets says neither.
func TestThePhraseModelSegmentsWhatBudouXSegments(t *testing.T) {
	for _, want := range [][]string{
		{"東京へ", "行きましょう。"},
		{"楽しい", "ドライブ。"},
		{"楽しい", "ドライブ、", "楽しい", "ドライブ。"},
		{"一生懸命働きます。"},
		{"今日は", "天気が", "いいですね。"},
		{"日本語の", "テキストです。"},
		{"私は", "昨日、", "友達と", "映画を", "見に", "行きました。"},
		{"この", "機械学習モデルは", "日本語の", "文節を", "推定します。"},
		{"本を", "読むのが", "好きです。"},
		{"新しい", "図書館が", "駅の", "近くに", "建設されました。"},
		{"彼女は", "毎朝six時に", "起きて、", "公園を", "走っています。"},
		{"ＵＲＬは", "１２３ページに", "書いてあります。"},
		{"星★と", "音符♪と☆。"},
		{"カタカナの", "テストです。"},
		{"中", "国語や", "韓国語ではありません。"},
	} {
		text := strings.Join(want, "")
		got := phrasesOf(t, text)
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("%s\n got %s\nwant %s", text,
				strings.Join(got, " | "), strings.Join(want, " | "))
		}
	}
}

// phrasesOf cuts a run at the boundaries PhraseBreaks calls one.
func phrasesOf(t *testing.T, text string) []string {
	t.Helper()
	breaks := PhraseBreaks(text, WritingSystemJapanese)
	var out []string
	last := 0
	for at := 1; at <= len(text); at++ {
		if at == len(text) || breaks[at] {
			out = append(out, text[last:at])
			last = at
		}
	}
	return out
}

// The three answers PhraseBreaks gives, which are what §5.2's fallback is made
// of. Absent is not false: false suppresses an opportunity and absent leaves it
// to the rules that would otherwise decide.
func TestAPlaceTheModelHasNothingToSayAboutIsAbsentAndNotFalse(t *testing.T) {
	// The suite's word-break-auto-phrase-fallback-001: Thai tagged as Japanese.
	// Every offset in it must be absent, because a run of Thai characters is
	// not a phrase in Japanese and its own dictionary is what breaks it.
	const thai = "กรุงเทพคือสวยงาม"
	if got := PhraseBreaks(thai, WritingSystemJapanese); got != nil {
		t.Errorf("Thai tagged as Japanese: got %v, want no answer at all", got)
	}
	// And fallback-002: content with no language tag has no model.
	if got := PhraseBreaks("東京へ行きましょう。", WritingSystemOther); got != nil {
		t.Errorf("untagged Japanese: got %v, want no answer at all", got)
	}
	if HasPhraseModel(WritingSystemOther) || HasPhraseModel(WritingSystemChinese) {
		t.Error("a writing system with no model here reports one")
	}
	if !HasPhraseModel(WritingSystemJapanese) {
		t.Error("Japanese reports no model")
	}

	// A Latin word inside a Japanese sentence is scored with the sentence and
	// covered by nobody: the boundaries inside it belong to the rules for
	// Latin, and the boundaries at its edges belong to the model.
	const mixed = "彼女は毎朝six時に起きて、公園を走っています。"
	breaks := PhraseBreaks(mixed, WritingSystemJapanese)
	at := strings.Index(mixed, "six")
	for off := at + 1; off < at+len("six"); off++ {
		if _, ok := breaks[off]; ok {
			t.Errorf("offset %d, inside %q, has an answer from the model", off, "six")
		}
	}
	for _, edge := range []int{at, at + len("six")} {
		if _, ok := breaks[edge]; !ok {
			t.Errorf("offset %d, at the edge of %q, has no answer from the model", edge, "six")
		}
	}
	// And the sentence is segmented as the reference implementation segments
	// it, Latin word and all — which is only true because the whole run is
	// scored. Cutting it at the Latin would take the model's window away.
	if got := strings.Join(phrasesOf(t, mixed), " | "); got != "彼女は | 毎朝six時に | 起きて、 | 公園を | 走っています。" {
		t.Errorf("mixed script: got %s", got)
	}
}

// A run with nothing in it for the model to read costs nothing and says
// nothing. The first character of a run is never a boundary: what happens in
// front of it is the caller's question, as it is for DictionaryBreaks.
func TestTheModelSaysNothingAboutTheEdgesOfARun(t *testing.T) {
	for _, text := range []string{"", "東", "hello world"} {
		if got := PhraseBreaks(text, WritingSystemJapanese); len(got) != 0 {
			t.Errorf("PhraseBreaks(%q) = %v, want nothing", text, got)
		}
	}
	breaks := PhraseBreaks("東京へ行きましょう。", WritingSystemJapanese)
	if _, ok := breaks[0]; ok {
		t.Error("the offset in front of a run's first character has an answer")
	}
	if _, ok := breaks[len("東京へ行きましょう。")]; ok {
		t.Error("the offset past a run's last character has an answer")
	}
}

// Each group reads its own window, and a synthetic model with one weight in it
// says which. The text is seven distinct characters, so a feature string
// appears in it exactly once and at most one boundary can score above zero —
// and the one that does is the boundary the group was asked about.
//
// It is here because the sentences above cannot show it. A model has sixteen
// hundred weights and a handful of them decide any one boundary, so a window
// read one character to the left still segments almost every sentence
// correctly: shifting UW1's by one moved nothing in forty of them.
func TestEachGroupReadsItsOwnWindow(t *testing.T) {
	// か き く け こ さ し, with the boundary under test between け and こ —
	// three characters in from each end, which is as far as any window reaches.
	const text = "かきくけこさし"
	const boundary = 3 // in characters, and each of these is three bytes
	for _, c := range []struct {
		group int
		name  string
		reads string
	}{
		{uw1, "UW1", "か"},
		{uw2, "UW2", "き"},
		{uw3, "UW3", "く"},
		{uw4, "UW4", "け"},
		{uw5, "UW5", "こ"},
		{uw6, "UW6", "さ"},
		{bw1, "BW1", "きく"},
		{bw2, "BW2", "くけ"},
		{bw3, "BW3", "けこ"},
		{tw1, "TW1", "かきく"},
		{tw2, "TW2", "きくけ"},
		{tw3, "TW3", "くけこ"},
		{tw4, "TW4", "けこさ"},
	} {
		m := onlyWeight(c.group, c.reads, 1)
		var fired []int
		m.scorePhrases(text, func(at int, isBoundary bool) {
			if isBoundary {
				fired = append(fired, at/len("か"))
			}
		})
		if len(fired) != 1 || fired[0] != boundary {
			t.Errorf("%s weighted on %q: boundaries at %v, want only %d",
				c.name, c.reads, fired, boundary)
		}
	}
}

// The bias is doubled and the comparison is strict, which together decide every
// boundary a model has nothing much to say about. A weight that exactly
// cancels the bias is not a boundary: BudouX starts each score at minus half
// the sum of the weights and asks for more than zero, so a tie goes to the
// text staying whole.
func TestATieIsNotABoundary(t *testing.T) {
	const text = "かきくけこさし"
	for _, c := range []struct {
		weight int
		want   bool
	}{
		{1, false}, // 2*1 - 2 == 0, and zero is not above zero
		{2, true},  // 2*2 - 2 == 2
	} {
		m := onlyWeight(uw3, "く", c.weight)
		m.bias = -2
		got := false
		m.scorePhrases(text, func(at int, isBoundary bool) {
			if isBoundary {
				got = true
			}
		})
		if got != c.want {
			t.Errorf("weight %d against a bias of %d: boundary=%v, want %v",
				c.weight, m.bias, got, c.want)
		}
	}
}

// onlyWeight is a model with one weight in it and no bias.
func onlyWeight(group int, text string, weight int) *phraseModel {
	m := &phraseModel{}
	for i := range m.groups {
		m.groups[i] = map[string]int{}
	}
	m.groups[group][text] = weight
	return m
}

// A run the model is not about costs nothing. Almost every run in almost every
// document is one, including under a Japanese language tag: a page of Japanese
// still has its class names, its numbers and its Latin in boxes of their own.
func TestARunWithNoJapaneseInItAllocatesNothing(t *testing.T) {
	if n := testing.AllocsPerRun(100, func() {
		PhraseBreaks("The quick brown fox jumps over the lazy dog.", WritingSystemJapanese)
	}); n != 0 {
		t.Errorf("PhraseBreaks over Latin allocated %v times, want none", n)
	}
}
