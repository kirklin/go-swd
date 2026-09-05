package automaton

import (
	"bufio"
	"math/rand"
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

func mustBuild(t testing.TB, words ...Word) *Matcher {
	t.Helper()
	m, err := Build(words)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func all(m *Matcher, text string, opt Options) []Hit {
	var hits []Hit
	m.Scan(text, opt, func(h Hit) bool { hits = append(hits, h); return true })
	return hits
}

func names(m *Matcher, hits []Hit) []string {
	var out []string
	for _, h := range hits {
		out = append(out, m.Word(h.Word).Text)
	}
	return out
}

func TestBasic(t *testing.T) {
	m := mustBuild(t, Word{"敏感词", 1}, Word{"感词", 2}, Word{"abc", 4}, Word{"b", 8})
	text := "这是敏感词abc"
	got := all(m, text, Options{})
	want := []Hit{
		{Word: 0, StartRune: 2, EndRune: 5, StartByte: 6, EndByte: 15},
		{Word: 1, StartRune: 3, EndRune: 5, StartByte: 9, EndByte: 15},
		{Word: 3, StartRune: 6, EndRune: 7, StartByte: 16, EndByte: 17},
		{Word: 2, StartRune: 5, EndRune: 8, StartByte: 15, EndByte: 18},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("hit %d: got %+v want %+v", i, got[i], want[i])
		}
		if text[got[i].StartByte:got[i].EndByte] != string([]rune(text)[got[i].StartRune:got[i].EndRune]) {
			t.Errorf("hit %d: byte/rune spans disagree", i)
		}
	}
	// early stop
	n := 0
	m.Scan(text, Options{}, func(Hit) bool { n++; return false })
	if n != 1 {
		t.Errorf("early stop: callback called %d times", n)
	}
	if got := all(m, "", Options{}); len(got) != 0 {
		t.Errorf("empty text: %v", got)
	}
	if got := all(m, "没有任何命中", Options{}); len(got) != 0 {
		t.Errorf("clean text: %v", got)
	}
}

func TestMask(t *testing.T) {
	m := mustBuild(t, Word{"aa", 1}, Word{"bb", 2}, Word{"cc", 0})
	text := "aabbcc"
	if got := names(m, all(m, text, Options{})); strings.Join(got, ",") != "aa,bb,cc" {
		t.Errorf("mask 0: %v", got)
	}
	if got := names(m, all(m, text, Options{Mask: 2})); strings.Join(got, ",") != "bb" {
		t.Errorf("mask 2: %v", got)
	}
	if got := names(m, all(m, text, Options{Mask: 3})); strings.Join(got, ",") != "aa,bb" {
		t.Errorf("mask 3: %v", got)
	}
}

func TestMergeDuplicates(t *testing.T) {
	m := mustBuild(t, Word{"Fuck", 1}, Word{"fuck", 2}, Word{"ＦＵＣＫ", 4}, Word{"", 8}, Word{"\u200b", 16})
	if len(m.Words()) != 1 {
		t.Fatalf("words = %+v", m.Words())
	}
	if w := m.Word(0); w.Text != "Fuck" || w.Payload != 7 {
		t.Errorf("merged word = %+v", w)
	}
}

func TestFolding(t *testing.T) {
	m := mustBuild(t, Word{"FuCk", 1}, Word{"一夜情", 2}, Word{"08宪章", 4})
	cases := []struct {
		text string
		want string
	}{
		{"fuck", "FuCk"},
		{"FUCK", "FuCk"},
		{"ｆＵｃｋ", "FuCk"},
		{"\U0001D41F\U0001D42E\U0001D41C\U0001D424", "FuCk"}, // 𝐟𝐮𝐜𝐤
		{"🄵🅄🄲🄺", "FuCk"},
		{"f\u200bu\u0301c\ufe0fk", "FuCk"},
		{"1夜情", "一夜情"},
		{"①夜情", "一夜情"},
		{"壹夜情", "一夜情"},
		{"零八宪章", "08宪章"},
		{"⓪⑧宪章", "08宪章"},
		{"０８宪章", "08宪章"},
		{"\U0001D7CE\U0001D7D6宪章", "08宪章"}, // 𝟎𝟖
	}
	for _, c := range cases {
		hits := all(m, "x"+c.text+"y", Options{})
		if len(hits) != 1 {
			t.Errorf("%q: hits = %+v", c.text, hits)
			continue
		}
		h := hits[0]
		if m.Word(h.Word).Text != c.want {
			t.Errorf("%q: matched %q, want %q", c.text, m.Word(h.Word).Text, c.want)
		}
		if h.StartRune != 1 || h.EndRune != 1+utf8.RuneCountInString(c.text) || h.StartByte != 1 || h.EndByte != 1+len(c.text) {
			t.Errorf("%q: span %+v", c.text, h)
		}
	}
}

func TestSingleRuneWord(t *testing.T) {
	m := mustBuild(t, Word{"屄", 1}, Word{"你妈", 2})
	got := names(m, all(m, "你屄你妈", Options{}))
	if strings.Join(got, ",") != "屄,你妈" {
		t.Errorf("got %v", got)
	}
}

func TestGapMode(t *testing.T) {
	m := mustBuild(t, Word{"fuck", 1}, Word{"法轮功", 2}, Word{"a.b", 4}, Word{"妈妈", 8})
	type tc struct {
		text string
		opt  Options
		want []string
	}
	cases := []tc{
		{"f*u*c*k", Options{}, nil},
		{"f*u*c*k", Options{MaxGap: 1}, []string{"fuck"}},
		{"f**u**c**k", Options{MaxGap: 1}, nil},
		{"f**u**c**k", Options{MaxGap: 2}, []string{"fuck"}},
		{"f u c k", Options{MaxGap: 1}, []string{"fuck"}},
		{"f\u200bu c k", Options{MaxGap: 1}, []string{"fuck"}},
		{"法 轮 功", Options{MaxGap: 1}, []string{"法轮功"}},
		{"法🙂轮🙂功", Options{MaxGap: 1}, []string{"法轮功"}},
		{"法x轮功", Options{MaxGap: 3}, nil},
		{"fuuuck", Options{}, nil},
		{"fuuuck", Options{CollapseRepeats: true}, []string{"fuck"}},
		{"ffuucckk", Options{CollapseRepeats: true}, []string{"fuck"}},
		{"妈妈", Options{CollapseRepeats: true}, []string{"妈妈"}},
		{"妈妈妈", Options{CollapseRepeats: true}, []string{"妈妈", "妈妈"}},
		{"a.b", Options{}, []string{"a.b"}},
		{"a..b", Options{MaxGap: 1}, []string{"a.b"}},
		{"f*u*u*c*k", Options{MaxGap: 1, CollapseRepeats: true}, []string{"fuck"}},
	}
	for _, c := range cases {
		got := names(m, all(m, c.text, c.opt))
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("%q %+v: got %v want %v", c.text, c.opt, got, c.want)
		}
	}
	// spans cover the skipped characters
	hits := all(m, "xx f*u*c*k yy", Options{MaxGap: 1})
	if len(hits) != 1 || hits[0].StartRune != 3 || hits[0].EndRune != 10 || hits[0].StartByte != 3 || hits[0].EndByte != 10 {
		t.Errorf("gap span: %+v", hits)
	}
	hits = all(m, "法 轮 功", Options{MaxGap: 1})
	if len(hits) != 1 || hits[0].StartByte != 0 || hits[0].EndByte != len("法 轮 功") {
		t.Errorf("gap span: %+v", hits)
	}
}

func TestInvalidUTF8(t *testing.T) {
	m := mustBuild(t, Word{"ab", 1})
	text := "\xff\xfea\xffb ab\xe4\xbd"
	hits := all(m, text, Options{})
	rs := []rune(text)
	if len(hits) != 1 {
		t.Fatalf("hits = %+v", hits)
	}
	h := hits[0]
	if string(rs[h.StartRune:h.EndRune]) != "ab" || text[h.StartByte:h.EndByte] != "ab" {
		t.Errorf("span %+v", h)
	}
}

func TestEmptyMatcher(t *testing.T) {
	m := mustBuild(t)
	if hits := all(m, "anything 任何", Options{MaxGap: 2}); len(hits) != 0 {
		t.Errorf("hits = %+v", hits)
	}
}

func TestBuildErrors(t *testing.T) {
	if _, err := Build([]Word{{strings.Repeat("a", 256), 1}}); err == nil {
		t.Error("expected ErrWordTooLong")
	}
	if _, err := Build([]Word{{strings.Repeat("a", 255), 1}}); err != nil {
		t.Errorf("255 runes should be fine: %v", err)
	}
}

// randomDict builds a small dictionary over a tiny alphabet to force overlaps.
func randomDict(rng *rand.Rand) []Word {
	alphabet := []rune("abAB中文１2")
	n := 1 + rng.Intn(12)
	words := make([]Word, 0, n)
	for i := 0; i < n; i++ {
		l := 1 + rng.Intn(4)
		var sb strings.Builder
		for j := 0; j < l; j++ {
			sb.WriteRune(alphabet[rng.Intn(len(alphabet))])
		}
		words = append(words, Word{sb.String(), 1 << uint(rng.Intn(4))})
	}
	return words
}

func randomText(rng *rand.Rand, extra bool) string {
	pieces := []string{"a", "b", "A", "B", "中", "文", "1", "2", "１", "Ａ", "\U0001D41A", " ", "*", "🙂", "\u200b", "\u0301", "\xff", "x", "。"}
	if !extra {
		pieces = pieces[:8]
	}
	n := rng.Intn(30)
	var sb strings.Builder
	for i := 0; i < n; i++ {
		sb.WriteString(pieces[rng.Intn(len(pieces))])
	}
	return sb.String()
}

func TestCrossCheckRandom(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for iter := 0; iter < 3000; iter++ {
		words := randomDict(rng)
		m := mustBuild(t, words...)
		rf := buildRef(words)
		if len(m.Words()) != len(rf.words) {
			t.Fatalf("iter %d: word count %d vs %d", iter, len(m.Words()), len(rf.words))
		}
		for k := range rf.words {
			if m.Word(int32(k)) != rf.words[k] {
				t.Fatalf("iter %d: word %d differs", iter, k)
			}
		}
		for j := 0; j < 20; j++ {
			text := randomText(rng, j%2 == 0)
			mask := uint32(rng.Intn(4))
			got := sortHits(all(m, text, Options{Mask: mask}))
			want := sortHits(rf.matchAll(text, mask))
			if len(got) != len(want) {
				t.Fatalf("iter %d text %q dict %+v mask %d:\n got %+v\nwant %+v", iter, text, words, mask, got, want)
			}
			for k := range got {
				if got[k] != want[k] {
					t.Fatalf("iter %d text %q dict %+v: hit %d got %+v want %+v", iter, text, words, k, got[k], want[k])
				}
			}
		}
	}
}

func loadDictWords(t testing.TB) []Word {
	t.Helper()
	f, err := os.Open("../../dict/all.txt")
	if err != nil {
		t.Skip("dictionary not available:", err)
	}
	defer func() { _ = f.Close() }()
	var words []Word
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		w := strings.TrimSpace(sc.Text())
		if w == "" || strings.HasPrefix(w, "#") {
			continue
		}
		words = append(words, Word{w, 1})
	}
	return words
}

func TestCrossCheckDefaultDict(t *testing.T) {
	words := loadDictWords(t)
	m := mustBuild(t, words...)
	rf := buildRef(words)
	st := m.Stats()
	t.Logf("stats: %+v", st)
	rng := rand.New(rand.NewSource(7))
	filler := []rune("的一是在不了有和人这中大为上个国我以要他时来用们生到作地于出就分对成会可主发年动同工也能下过子说产种面而方后多定行学法所民得经十三之进着等部度家电力里如水化高自二理起小物现实加量都两体制机当使点从业本去把性好应开它合还因由其些然前外天政四日那社义事平形相全表间样与关各重新线内数正心反你明看原又么利比或但质气第向道命此变条只没结解问意建月公无系军很情者最立代想已通并提直题党程展五果料象员革位入常文总次品式活设及管特件长求老头基资边流路级少图山统接知较将组见计别她手角期根论运农指几九区强放决西被干做必战先回则任取据处队南给色光门即保治北造百规热领七海口东导器压志世金增争济阶油思术极交受联什认六共权收证改清己美再采转更单风切打白教速花带安场身车例真务具万每目至达走积示议声报斗完类八离华名确才科张信马节话米整空元况今集温传土许步群广石记需段研界拉林律叫且究观越织装影算低持音众书布复容儿须际商非验连断深难近矿千周委素技备半办青省列习响约支般史感劳便团往酸历市克何除消构府称太准精值号率族维划选标写存候毛亲快效斯院查江型眼王按格养易置派层片始却专状育厂京识适属圆包火住调满县局照参红细引听该铁价严龙飞 ,.*ABCabc0123ＡＢ①")
	for iter := 0; iter < 1500; iter++ {
		var sb strings.Builder
		n := 1 + rng.Intn(50)
		for i := 0; i < n; i++ {
			switch rng.Intn(6) {
			case 0:
				sb.WriteString(words[rng.Intn(len(words))].Text)
			case 1:
				w := []rune(words[rng.Intn(len(words))].Text)
				sb.WriteString(string(w[:1+rng.Intn(len(w))]))
			case 2:
				w := []rune(words[rng.Intn(len(words))].Text)
				sb.WriteString(string(w[rng.Intn(len(w)):]))
			case 3:
				w := []rune(words[rng.Intn(len(words))].Text)
				sb.WriteString(strings.ToUpper(string(w)))
			default:
				sb.WriteRune(filler[rng.Intn(len(filler))])
			}
		}
		text := sb.String()
		if iter%100 == 0 {
			text += "\xff\xfe" + text
		}
		got := sortHits(all(m, text, Options{}))
		want := sortHits(rf.matchAll(text, 0))
		if len(got) != len(want) {
			t.Fatalf("iter %d text %q:\n got %d %+v\nwant %d %+v", iter, text, len(got), got, len(want), want)
		}
		for k := range got {
			if got[k] != want[k] {
				t.Fatalf("iter %d text %q: hit %d got %+v want %+v", iter, text, k, got[k], want[k])
			}
		}
	}
}

func FuzzScan(f *testing.F) {
	words := []Word{{"fuck", 1}, {"敏感词", 2}, {"感词", 4}, {"a.b", 8}, {"妈妈", 16}, {"屄", 32}, {"08宪章", 64}, {"ab", 128}}
	m, err := Build(words)
	if err != nil {
		f.Fatal(err)
	}
	rf := buildRef(words)
	for _, s := range []string{"", "fuck", "f*u*c*k", "这是敏感词", "妈妈妈", "a..b", "\xff\xfe", "ＦＵＣＫ", "f\u200buck", "零八宪章"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, text string) {
		got := sortHits(all(m, text, Options{}))
		want := sortHits(rf.matchAll(text, 0))
		if len(got) != len(want) {
			t.Fatalf("%q: got %+v want %+v", text, got, want)
		}
		for k := range got {
			if got[k] != want[k] {
				t.Fatalf("%q: hit %d got %+v want %+v", text, k, got[k], want[k])
			}
		}
		for _, opt := range []Options{{MaxGap: 1}, {MaxGap: 3, CollapseRepeats: true}, {CollapseRepeats: true}} {
			for _, h := range all(m, text, opt) {
				if h.StartByte < 0 || h.StartByte >= h.EndByte || h.EndByte > len(text) || h.StartRune < 0 || h.StartRune >= h.EndRune {
					t.Fatalf("%q %+v: bad span %+v", text, opt, h)
				}
				if !utf8.RuneStart(text[h.StartByte]) {
					t.Fatalf("%q %+v: start not on rune boundary %+v", text, opt, h)
				}
				if utf8.RuneCountInString(text[:h.StartByte]) != h.StartRune || utf8.RuneCountInString(text[:h.EndByte]) != h.EndRune {
					t.Fatalf("%q %+v: byte/rune positions disagree %+v", text, opt, h)
				}
			}
		}
	})
}

func BenchmarkBuildDefaultDict(b *testing.B) {
	words := loadDictWords(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Build(words); err != nil {
			b.Fatal(err)
		}
	}
}
