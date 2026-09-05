package swd

import (
	"strings"
	"sync"
	"testing"
)

var (
	benchOnce   sync.Once
	benchEngine *Engine
	benchGap    *Engine
	benchTexts  = map[string]string{}
	benchNames  = []string{"clean", "clean_long", "ascii", "hit", "long"}
	sinkB       bool
	sinkM       []Match
	sinkP       *Match
	sinkS       string
)

func setupBench(b *testing.B) {
	benchOnce.Do(func() {
		var err error
		if benchEngine, err = New(); err != nil {
			panic(err)
		}
		if benchGap, err = New(WithMaxGap(1), WithCollapseRepeats(true)); err != nil {
			panic(err)
		}
		var words []string
		parseLines(dictAll, func(w string) { words = append(words, w) })
		w1, w2, w3 := words[1000], words[20000], words[30000]
		clean := "今天天气不错，我们一起去公园散步，然后回家吃饭看书，晚上早点休息。"
		hit := "这是一段普通的测试文本，" + w1 + "，中间还有一些别的内容，" + w2 + "以及" + w3 + "。结束。"
		benchTexts["clean"] = clean
		benchTexts["clean_long"] = strings.Repeat(clean, 25)
		benchTexts["ascii"] = strings.Repeat("The quick brown fox jumps over the lazy dog while the cat sleeps. ", 12)
		benchTexts["hit"] = hit
		benchTexts["long"] = strings.Repeat(clean+hit, 10)
	})
}

func benchEach(b *testing.B, fn func(text string)) {
	setupBench(b)
	for _, name := range benchNames {
		text := benchTexts[name]
		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(len(text)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				fn(text)
			}
		})
	}
}

func BenchmarkNew(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := New(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDetect(b *testing.B) {
	benchEach(b, func(t string) { sinkB = benchEngine.Detect(t) })
}

func BenchmarkDetectIn(b *testing.B) {
	benchEach(b, func(t string) { sinkB = benchEngine.DetectIn(t, All) })
}

func BenchmarkMatch(b *testing.B) {
	benchEach(b, func(t string) { sinkP = benchEngine.Match(t) })
}

func BenchmarkMatchAll(b *testing.B) {
	benchEach(b, func(t string) { sinkM = benchEngine.MatchAll(t) })
}

func BenchmarkMatchesIter(b *testing.B) {
	benchEach(b, func(t string) {
		n := 0
		for range benchEngine.Matches(t) {
			n++
		}
		sinkB = n > 0
	})
}

func BenchmarkReplaceWithAsterisk(b *testing.B) {
	benchEach(b, func(t string) { sinkS = benchEngine.ReplaceWithAsterisk(t) })
}

func BenchmarkGapDetect(b *testing.B) {
	benchEach(b, func(t string) { sinkB = benchGap.Detect(t) })
}

func BenchmarkAddRemoveWord(b *testing.B) {
	setupBench(b)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := benchEngine.AddWord("基准临时词", Custom); err != nil {
			b.Fatal(err)
		}
		if err := benchEngine.RemoveWord("基准临时词"); err != nil {
			b.Fatal(err)
		}
	}
}
