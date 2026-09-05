package swd

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func newTest(t testing.TB, words map[string]Category, opts ...Option) *Engine {
	t.Helper()
	e, err := New(append([]Option{WithoutDefaultDict(), WithWords(words)}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func wordsOf(ms []Match) string {
	parts := make([]string, len(ms))
	for i, m := range ms {
		parts[i] = m.Word
	}
	return strings.Join(parts, ",")
}

func TestDefaultDictionary(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if n := e.Len(); n < 15000 {
		t.Fatalf("Len() = %d", n)
	}
	st := e.Stats()
	t.Logf("stats: %+v", st)
	if st.Words < 15000 || st.Nodes < 40000 || st.Alphabet < 2000 {
		t.Errorf("unexpected stats %+v", st)
	}
	text := "这段话提到了裸聊直播和法轮功。"
	if !e.Detect(text) {
		t.Fatal("Detect = false")
	}
	if m := e.MatchIn(text, Pornography); m == nil || m.Word != "裸聊" {
		t.Fatalf("MatchIn = %+v", m)
	}
	if !e.DetectIn(text, Political) || !e.DetectIn(text, Pornography) || e.DetectIn(text, Contraband) {
		t.Error("DetectIn category filter broken")
	}
	got := wordsOf(e.MatchAllIn(text, All))
	if !strings.Contains(got, "裸聊") || !strings.Contains(got, "法轮功") {
		t.Errorf("MatchAllIn(All) = %q", got)
	}
	// Every dictionary word now carries a label, so DetectIn covers the
	// whole dictionary.
	if !e.Detect("08宪章") || !e.DetectIn("08宪章", All) {
		t.Error("08宪章 not detected")
	}
	if e.Detect("今天天气不错，我们一起去公园散步。") {
		t.Error("false positive on clean text")
	}
}

func TestDetectAndMatch(t *testing.T) {
	e := newTest(t, map[string]Category{"敏感词": Custom, "感词": Political, "abc": Inappropriate, "b": Violence})
	text := "这是敏感词abc"
	all := e.MatchAll(text)
	want := []Match{
		{Word: "敏感词", Category: Custom, StartPos: 2, EndPos: 5, ByteStart: 6, ByteEnd: 15},
		{Word: "感词", Category: Political, StartPos: 3, EndPos: 5, ByteStart: 9, ByteEnd: 15},
		{Word: "b", Category: Violence, StartPos: 6, EndPos: 7, ByteStart: 16, ByteEnd: 17},
		{Word: "abc", Category: Inappropriate, StartPos: 5, EndPos: 8, ByteStart: 15, ByteEnd: 18},
	}
	if len(all) != len(want) {
		t.Fatalf("MatchAll = %+v", all)
	}
	for i := range want {
		g, w := all[i], want[i]
		if g.Word != w.Word || g.Category != w.Category || g.StartPos != w.StartPos ||
			g.EndPos != w.EndPos || g.ByteStart != w.ByteStart || g.ByteEnd != w.ByteEnd {
			t.Errorf("MatchAll[%d] = %+v, want %+v", i, g, w)
		}
	}
	if m := e.Match(text); m == nil || m.Word != want[0].Word {
		t.Errorf("Match = %+v", m)
	}
	if m := e.MatchIn(text, Political); m == nil || m.Word != "感词" {
		t.Errorf("MatchIn(Political) = %+v", m)
	}
	if m := e.MatchIn(text, None); m != nil {
		t.Errorf("MatchIn(None) = %+v", m)
	}
	if m := e.MatchIn(text); m != nil {
		t.Errorf("MatchIn() = %+v", m)
	}
	if got := wordsOf(e.MatchAllIn(text, Violence|Inappropriate)); got != "b,abc" {
		t.Errorf("MatchAllIn = %q", got)
	}
	if got := wordsOf(e.MatchAllIn(text, Violence, Custom)); got != "敏感词,b" {
		t.Errorf("MatchAllIn(two cats) = %q", got)
	}
	if e.DetectIn(text) || e.DetectIn(text, Contraband) || !e.DetectIn(text, Contraband, Custom) {
		t.Error("DetectIn broken")
	}
	if e.Detect("") || e.MatchAll("") != nil || e.Match("") != nil {
		t.Error("empty text")
	}
	if e.Detect("没有任何命中") {
		t.Error("clean text")
	}
}

func TestFoldingThroughAPI(t *testing.T) {
	e := newTest(t, map[string]Category{"FuCk": Inappropriate, "一夜情": Pornography})
	for _, s := range []string{"fuck", "ＦＵＣＫ", "\U0001D41F\U0001D42E\U0001D41C\U0001D424", "f\u200buck", "1夜情", "①夜情", "壹夜情"} {
		if !e.Detect(s) {
			t.Errorf("Detect(%q) = false", s)
		}
	}
	m := e.Match("xx ＦＵＣＫ yy")
	if m == nil || m.Word != "FuCk" || m.StartPos != 3 || m.EndPos != 7 ||
		m.ByteStart != 3 || m.ByteEnd != 3+len("ＦＵＣＫ") {
		t.Errorf("Match = %+v", m)
	}
}

func TestReplace(t *testing.T) {
	e := newTest(t, map[string]Category{"敏感词": Custom, "感词": Political, "abc": Inappropriate, "b": Violence})
	text := "这是敏感词abc!"
	if got := e.ReplaceWithAsterisk(text); got != "这是******!" {
		t.Errorf("ReplaceWithAsterisk = %q", got)
	}
	if got := e.Replace(text, '＃'); got != "这是＃＃＃＃＃＃!" {
		t.Errorf("Replace = %q", got)
	}
	if got := e.ReplaceIn(text, '*', Violence); got != "这是敏感词a*c!" {
		t.Errorf("ReplaceIn = %q", got)
	}
	if got := e.ReplaceWithAsteriskIn(text, Political); got != "这是敏**abc!" {
		t.Errorf("ReplaceWithAsteriskIn = %q", got)
	}
	if got := e.ReplaceIn(text, '*'); got != text {
		t.Errorf("ReplaceIn() = %q", got)
	}
	var seen []string
	strategy := func(m Match) string {
		seen = append(seen, fmt.Sprintf("%s:%d-%d", m.Word, m.StartPos, m.EndPos))
		return "[" + m.Category.String() + "]"
	}
	if got := e.ReplaceWithStrategy(text, strategy); got != "这是[涉政|自定义][暴恐|不良内容]!" {
		t.Errorf("ReplaceWithStrategy = %q", got)
	}
	if strings.Join(seen, " ") != "敏感词:2-5 abc:5-8" {
		t.Errorf("strategy saw %v", seen)
	}
	if got := e.ReplaceWithStrategyIn(text, func(Match) string { return "" }, Political); got != "这是敏abc!" {
		t.Errorf("ReplaceWithStrategyIn = %q", got)
	}
	if got := e.ReplaceWithStrategy(text, nil); got != text {
		t.Errorf("nil strategy = %q", got)
	}
	clean := "干净的文本"
	if got := e.ReplaceWithAsterisk(clean); got != clean {
		t.Errorf("clean = %q", got)
	}
	if got := e.ReplaceWithAsterisk(""); got != "" {
		t.Errorf("empty = %q", got)
	}
}

func TestAddRemoveClear(t *testing.T) {
	e := newTest(t, map[string]Category{"foo": Custom})
	if err := e.AddWord("独角兽", Political); err != nil {
		t.Fatal(err)
	}
	if !e.Detect("有独角兽出现") {
		t.Fatal("word added but not detected")
	}
	if err := e.AddWord(" 独角兽 ", Custom); err != nil {
		t.Fatal(err)
	}
	if got := e.Words()["独角兽"]; got != Political|Custom {
		t.Errorf("categories not merged: %v", got)
	}
	if m := e.Match("独角兽"); m == nil || m.Category != Political|Custom {
		t.Errorf("Match = %+v", m)
	}
	if err := e.AddWord("   ", Custom); !errors.Is(err, ErrEmptyWord) {
		t.Errorf("empty word: %v", err)
	}
	if err := e.AddWord("x", Category(1)); !errors.Is(err, ErrInvalidCategory) {
		t.Errorf("invalid category: %v", err)
	}
	if err := e.AddWord(strings.Repeat("长", 256), Custom); !errors.Is(err, ErrWordTooLong) {
		t.Errorf("too long: %v", err)
	}
	if err := e.AddWords(map[string]Category{"ok": Custom, "": Custom}); !errors.Is(err, ErrEmptyWord) || e.Detect("ok") {
		t.Errorf("AddWords must validate before adding: %v", err)
	}
	if err := e.RemoveWord("独角兽"); err != nil || e.Detect("独角兽") {
		t.Error("RemoveWord failed")
	}
	if err := e.RemoveWord("不存在"); err != nil {
		t.Error(err)
	}
	if err := e.AddWords(map[string]Category{"甲": Contraband, "乙": Contraband}); err != nil {
		t.Fatal(err)
	}
	if e.Len() != 3 || !e.DetectIn("甲乙", Contraband) {
		t.Errorf("Len = %d", e.Len())
	}
	if err := e.Clear(); err != nil {
		t.Fatal(err)
	}
	if e.Len() != 0 || e.Detect("foo甲乙") || e.Stats().Words != 0 {
		t.Error("Clear did not clear")
	}
	if err := e.AddWord("再来", Custom); err != nil || !e.Detect("再来一次") {
		t.Error("AddWord after Clear")
	}
	// many removals trigger compaction
	batch := map[string]Category{}
	for i := 0; i < 300; i++ {
		batch[fmt.Sprintf("w%03d", i)] = Custom
	}
	if err := e.AddWords(batch); err != nil {
		t.Fatal(err)
	}
	var rm []string
	for i := 0; i < 200; i++ {
		rm = append(rm, fmt.Sprintf("w%03d", i))
	}
	if err := e.RemoveWords(rm); err != nil {
		t.Fatal(err)
	}
	if e.Len() != 101 {
		t.Errorf("Len = %d", e.Len())
	}
	for i := 200; i < 300; i++ {
		if !e.Detect(fmt.Sprintf("w%03d", i)) {
			t.Fatalf("w%03d lost after compaction", i)
		}
	}
	if e.Detect("w000") {
		t.Error("removed word still detected")
	}
}

func TestAllowWords(t *testing.T) {
	e := newTest(t, map[string]Category{"色情": Pornography, "情怀": Custom}, WithAllowWords("特色情怀", " "))
	if e.Detect("特色情怀") {
		t.Error("allowed phrase reported")
	}
	if !e.Detect("色情") || !e.Detect("特色情怀和色情") {
		t.Error("deny word lost")
	}
	if got := wordsOf(e.MatchAll("特色情怀和色情")); got != "色情" {
		t.Errorf("MatchAll = %q", got)
	}
	if got := e.ReplaceWithAsterisk("特色情怀色情"); got != "特色情怀**" {
		t.Errorf("Replace = %q", got)
	}
	if err := e.RemoveAllowWords("特色情怀"); err != nil || !e.Detect("特色情怀") {
		t.Error("RemoveAllowWords")
	}
	if err := e.AddAllowWords("特色情怀"); err != nil || e.Detect("特色情怀") {
		t.Error("AddAllowWords")
	}
	if e.Stats().AllowWords != 1 {
		t.Errorf("AllowWords = %d", e.Stats().AllowWords)
	}
}

func TestGapOptions(t *testing.T) {
	e := newTest(t, map[string]Category{"fuck": Inappropriate, "法轮功": Political}, WithMaxGap(1), WithCollapseRepeats(true))
	for _, s := range []string{"f*u*c*k", "法 轮 功", "fuuuck", "f u c k", "法🙂轮🙂功"} {
		if !e.Detect(s) {
			t.Errorf("Detect(%q) = false", s)
		}
	}
	if e.Detect("f**u**c**k") {
		t.Error("gap larger than MaxGap matched")
	}
	if got := e.ReplaceWithAsterisk("say f*u*c*k now"); got != "say ******* now" {
		t.Errorf("Replace = %q", got)
	}
	exact := newTest(t, map[string]Category{"fuck": Inappropriate})
	if exact.Detect("f*u*c*k") || exact.Detect("fuuuck") {
		t.Error("exact engine matched gapped text")
	}
}

func TestMatchesIterator(t *testing.T) {
	e := newTest(t, map[string]Category{"aa": Custom, "bb": Political})
	text := "aabbaa"
	n := 0
	for range e.Matches(text) {
		n++
	}
	if n != 3 {
		t.Errorf("Matches yielded %d", n)
	}
	n = 0
	for m := range e.Matches(text) {
		n++
		if m.Word != "aa" {
			t.Errorf("first = %+v", m)
		}
		break
	}
	if n != 1 {
		t.Errorf("break did not stop iteration: %d", n)
	}
	n = 0
	for m := range e.MatchesIn(text, Political) {
		n++
		if m.Word != "bb" {
			t.Errorf("MatchesIn = %+v", m)
		}
	}
	if n != 1 {
		t.Errorf("MatchesIn yielded %d", n)
	}
	for range e.MatchesIn(text) {
		t.Error("MatchesIn() must be empty")
	}
}

func TestCategory(t *testing.T) {
	cases := map[Category]string{
		None: "无风险", Pornography: "色情低俗", Custom: "自定义",
		Pornography | Custom: "色情低俗|自定义",
		All:                  "色情低俗|涉政|暴恐|违禁|不良内容|引流广告|宗教|广告法|AI生成|自定义",
		UserCategory(0):      "自定义0",
	}
	for c, want := range cases {
		if got := c.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", c, got, want)
		}
	}
	if !All.Contains(Pornography|Contraband) || Pornography.Contains(All) || Pornography.Contains(None) {
		t.Error("Contains")
	}
	if !All.IsValid() || !None.IsValid() || !UserCategory(20).IsValid() || Category(1).IsValid() {
		t.Error("IsValid")
	}
}

func TestOptionsValidation(t *testing.T) {
	if _, err := New(WithoutDefaultDict(), WithWords(map[string]Category{"x": Category(1)})); !errors.Is(err, ErrInvalidCategory) {
		t.Errorf("err = %v", err)
	}
	if _, err := New(WithoutDefaultDict(), WithWords(map[string]Category{" ": Custom})); !errors.Is(err, ErrEmptyWord) {
		t.Errorf("err = %v", err)
	}
	e, err := New(WithoutDefaultDict(), nil, WithMaxGap(-5))
	if err != nil || e.Len() != 0 || e.Detect("anything") {
		t.Errorf("empty engine: %v", err)
	}
}

func TestConcurrent(t *testing.T) {
	e := newTest(t, map[string]Category{"常驻词": Custom})
	text := "文本里有常驻词"
	var failures atomic.Int32
	var wg sync.WaitGroup
	for r := 0; r < 8; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 2000; i++ {
				if !e.Detect(text) || e.ReplaceWithAsterisk(text) != "文本里有***" || len(e.MatchAll(text)) == 0 {
					failures.Add(1)
				}
			}
		}()
	}
	for w := 0; w < 2; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				word := fmt.Sprintf("临时%d-%d", w, i)
				if err := e.AddWord(word, Custom); err != nil || !e.Detect(word) {
					failures.Add(1)
				}
				if err := e.RemoveWord(word); err != nil || e.Detect(word) {
					failures.Add(1)
				}
			}
		}(w)
	}
	wg.Wait()
	if n := failures.Load(); n != 0 {
		t.Errorf("%d failures", n)
	}
	if e.Len() != 1 {
		t.Errorf("Len = %d", e.Len())
	}
}
