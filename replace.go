package swd

import (
	"slices"
	"strings"
	"unicode/utf8"
)

// Replace replaces every character of every sensitive word with replacement.
// Overlapping matches are merged, so the whole sensitive region is masked.
func (e *Engine) Replace(text string, replacement rune) string {
	return e.replace(text, 0, replacement, nil)
}

// ReplaceIn is Replace restricted to the given categories.
func (e *Engine) ReplaceIn(text string, replacement rune, categories ...Category) string {
	mask := orCategories(categories)
	if mask == 0 {
		return text
	}
	return e.replace(text, mask, replacement, nil)
}

// ReplaceWithAsterisk masks sensitive words with '*'.
func (e *Engine) ReplaceWithAsterisk(text string) string {
	return e.Replace(text, '*')
}

// ReplaceWithAsteriskIn is ReplaceWithAsterisk restricted to the given categories.
func (e *Engine) ReplaceWithAsteriskIn(text string, categories ...Category) string {
	return e.ReplaceIn(text, '*', categories...)
}

// ReplaceWithStrategy replaces each sensitive region with strategy(match).
// Overlapping matches are merged into one region: the Match passed to
// strategy spans the whole region, carries the union of the categories and
// names the leftmost-longest word.
func (e *Engine) ReplaceWithStrategy(text string, strategy func(word Match) string) string {
	if strategy == nil {
		return text
	}
	return e.replace(text, 0, 0, strategy)
}

// ReplaceWithStrategyIn is ReplaceWithStrategy restricted to the given categories.
func (e *Engine) ReplaceWithStrategyIn(text string, strategy func(word Match) string, categories ...Category) string {
	mask := orCategories(categories)
	if mask == 0 || strategy == nil {
		return text
	}
	return e.replace(text, mask, 0, strategy)
}

func (e *Engine) replace(text string, mask Category, r rune, strategy func(Match) string) string {
	matches := e.collect(text, mask)
	if len(matches) == 0 {
		return text
	}
	slices.SortFunc(matches, func(a, b Match) int {
		if a.ByteStart != b.ByteStart {
			return a.ByteStart - b.ByteStart
		}
		return b.ByteEnd - a.ByteEnd
	})
	var sb strings.Builder
	sb.Grow(len(text))
	var buf [utf8.UTFMax]byte
	n := 0
	if strategy == nil {
		n = utf8.EncodeRune(buf[:], r)
	}
	last := 0
	flush := func(m Match) {
		sb.WriteString(text[last:m.ByteStart])
		if strategy != nil {
			sb.WriteString(strategy(m))
		} else {
			for i := m.StartPos; i < m.EndPos; i++ {
				sb.Write(buf[:n])
			}
		}
		last = m.ByteEnd
	}
	cur := matches[0]
	for _, m := range matches[1:] {
		if m.ByteStart < cur.ByteEnd {
			if m.ByteEnd > cur.ByteEnd {
				cur.ByteEnd, cur.EndPos = m.ByteEnd, m.EndPos
			}
			cur.Category |= m.Category
			continue
		}
		flush(cur)
		cur = m
	}
	flush(cur)
	sb.WriteString(text[last:])
	return sb.String()
}
