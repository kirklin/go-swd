package swd

import "sort"

type config struct {
	defaultDict bool
	words       map[string]Category
	allow       []string
	maxGap      int
	collapse    bool
}

// Option configures New.
type Option func(*config)

// WithoutDefaultDict starts with an empty word list instead of the built-in
// dictionary.
func WithoutDefaultDict() Option {
	return func(c *config) { c.defaultDict = false }
}

// WithWords adds words (with their categories) on top of the dictionary.
func WithWords(words map[string]Category) Option {
	return func(c *config) {
		if c.words == nil {
			c.words = map[string]Category{}
		}
		for w, cat := range words {
			c.words[w] |= cat
		}
	}
}

// WithAllowWords adds phrases that must never be reported: a match that lies
// completely inside an allowed phrase is suppressed (e.g. allow "特色情怀"
// so that it no longer triggers "色情").
func WithAllowWords(words ...string) Option {
	return func(c *config) { c.allow = append(c.allow, words...) }
}

// WithMaxGap tolerates up to n consecutive separator characters
// (whitespace, punctuation, symbols, emoji) between the characters of a
// word, so that "f*u*c*k" or "法 轮 功" are still detected. The reported
// match spans the separators. 0 (the default) means exact matching.
//
// Gap matching disables the 2-gram prefilter and is therefore slower on
// clean text; it can also produce more false positives.
func WithMaxGap(n int) Option {
	return func(c *config) {
		if n < 0 {
			n = 0
		}
		c.maxGap = n
	}
}

// WithCollapseRepeats treats a repeated character as part of the previous
// one when it cannot continue any word, so "fuuuck" matches "fuck" while
// words with genuine doubles such as "妈妈" keep matching.
func WithCollapseRepeats(on bool) Option {
	return func(c *config) { c.collapse = on }
}

func sortedKeys(m map[string]Category) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
