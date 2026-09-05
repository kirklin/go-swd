// Package normalize folds runes into a canonical form so that the matcher can
// treat visually or semantically equivalent characters as identical without a
// separate preprocessing pass over the text.
//
// Fold is a 1:1 mapping (one rune in, one rune out), which keeps positions in
// the original text exact. It covers:
//
//   - ASCII and Unicode case folding (FuCk -> fuck, Δ -> δ)
//   - full-width to half-width (ｆｕｃｋ -> fuck, ！ -> !)
//   - every digit style to ASCII digits (９ ⓽ ⁹ ₉ 玖 九 𝟡 -> 9)
//   - enclosed / mathematical alphanumerics to plain letters (Ⓐ 𝐚 🅰 -> a)
//   - Latin letters with diacritics to their base letter (é ñ ł -> e n l)
//
// Class reports whether a rune is a separator (whitespace, punctuation,
// symbols, emoji) or ignorable (zero-width and format characters, combining
// marks, variation selectors).
package normalize

import (
	"sync"
	"unicode"
)

// Class flags returned by Class.
const (
	// Separator marks whitespace, punctuation and symbols. In gap mode the
	// scanner may skip a bounded run of separators inside a word (f*u*c*k).
	Separator uint8 = 1 << iota
	// Ignorable marks invisible characters that never carry meaning:
	// format controls, zero-width joiners, combining marks, variation
	// selectors. The scanner always skips them.
	Ignorable
)

// latin1 maps U+00C0..U+00FF to base letters; U+00D7 (×) and U+00F7 (÷) are
// handled before the table is consulted.
const latin1 = "aaaaaaaceeeeiiiidnooooo*ouuuuyps" + "aaaaaaaceeeeiiiidnooooo/ouuuuypy"

// latinExtA maps U+0100..U+017F to base letters.
const latinExtA = "aaaaaa" + "cccccccc" + "dddd" + "eeeeeeeeee" + "gggggggg" + "hhhh" +
	"iiiiiiiiii" + "ii" + "jj" + "kkk" + "llllllllll" + "nnnnnnnnn" + "oooooooo" +
	"rrrrrr" + "ssssssss" + "tttttt" + "uuuuuuuuuuuu" + "ww" + "yyy" + "zzzzzz" + "s"

// Fold returns the canonical form of r. It is deterministic, total and 1:1.
func Fold(r rune) rune {
	switch {
	case r < 0x80:
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	case r >= 0x4E00 && r <= 0x9FFF:
		return foldCJK(r)
	case r >= 0xFF01 && r <= 0xFF5E: // full-width ASCII
		return Fold(r - 0xFEE0)
	case r == 0x3000: // ideographic space
		return ' '
	case r == 0x3007: // 〇
		return '0'
	case r == 0xD7, r == 0xF7: // × ÷
		return r
	case r >= 0xC0 && r <= 0xFF:
		return rune(latin1[r-0xC0])
	case r >= 0x100 && r <= 0x17F:
		return rune(latinExtA[r-0x100])
	case r < 0x2000:
		return foldOther(r)
	case r >= 0x1D400 && r <= 0x1D6A3: // mathematical Latin letters
		return 'a' + (r-0x1D400)%52%26
	case r >= 0x1D7CE && r <= 0x1D7FF: // mathematical digits
		return '0' + (r-0x1D7CE)%10
	}
	if d, ok := enclosedDigit(r); ok {
		return d
	}
	if l, ok := enclosedLetter(r); ok {
		return l
	}
	return foldOther(r)
}

func foldCJK(r rune) rune {
	switch r {
	case '零':
		return '0'
	case '一', '壹':
		return '1'
	case '二', '贰', '貳':
		return '2'
	case '三', '叁', '參':
		return '3'
	case '四', '肆':
		return '4'
	case '五', '伍':
		return '5'
	case '六', '陆', '陸':
		return '6'
	case '七', '柒':
		return '7'
	case '八', '捌':
		return '8'
	case '九', '玖':
		return '9'
	}
	return r
}

func enclosedDigit(r rune) (rune, bool) {
	switch {
	case r >= 0x2460 && r <= 0x2468: // ①..⑨
		return '1' + r - 0x2460, true
	case r >= 0x2474 && r <= 0x247C: // ⑴..⑼
		return '1' + r - 0x2474, true
	case r >= 0x2488 && r <= 0x2490: // ⒈..⒐
		return '1' + r - 0x2488, true
	case r == 0x24EA, r == 0x24FF: // ⓪ ⓿
		return '0', true
	case r >= 0x24F5 && r <= 0x24FD: // ⓵..⓽
		return '1' + r - 0x24F5, true
	case r >= 0x2776 && r <= 0x277E: // ❶..❾
		return '1' + r - 0x2776, true
	case r >= 0x2780 && r <= 0x2788: // ➀..➈
		return '1' + r - 0x2780, true
	case r >= 0x278A && r <= 0x2792: // ➊..➒
		return '1' + r - 0x278A, true
	case r == 0x2070: // ⁰
		return '0', true
	case r >= 0x2074 && r <= 0x2079: // ⁴..⁹
		return '4' + r - 0x2074, true
	case r >= 0x2080 && r <= 0x2089: // ₀..₉
		return '0' + r - 0x2080, true
	}
	return 0, false
}

func enclosedLetter(r rune) (rune, bool) {
	switch {
	case r >= 0x249C && r <= 0x24B5: // ⒜..⒵
		return 'a' + r - 0x249C, true
	case r >= 0x24B6 && r <= 0x24CF: // Ⓐ..Ⓩ
		return 'a' + r - 0x24B6, true
	case r >= 0x24D0 && r <= 0x24E9: // ⓐ..ⓩ
		return 'a' + r - 0x24D0, true
	case r >= 0x1F130 && r <= 0x1F149: // 🄰..🅉
		return 'a' + r - 0x1F130, true
	case r >= 0x1F150 && r <= 0x1F169: // 🅐..🅩
		return 'a' + r - 0x1F150, true
	case r >= 0x1F170 && r <= 0x1F189: // 🅰..🆉
		return 'a' + r - 0x1F170, true
	case r >= 0x1F1E6 && r <= 0x1F1FF: // regional indicators 🇦..🇿
		return 'a' + r - 0x1F1E6, true
	}
	return 0, false
}

// foldOther handles the long tail: superscript digits in Latin-1, letters of
// any script (lower-cased) and decimal digits of any script.
func foldOther(r rune) rune {
	switch r {
	case 0xB2, 0xB3: // ² ³
		return '0' + r - 0xB0
	case 0xB9: // ¹
		return '1'
	}
	if unicode.IsUpper(r) || unicode.IsTitle(r) {
		if l := unicode.ToLower(r); l != r {
			return Fold(l) // e.g. ẞ -> ß -> s
		}
		return r
	}
	if unicode.Is(unicode.Nd, r) {
		if d, ok := decimalValue(r); ok {
			return '0' + d
		}
	}
	return r
}

// decimalValue returns the numeric value of a Unicode decimal digit. Every Nd
// block is a run of ten code points starting at the digit zero, so the value
// is the offset inside the run.
func decimalValue(r rune) (rune, bool) {
	if r < 0x10000 {
		for _, rg := range unicode.Nd.R16 {
			if r >= rune(rg.Lo) && r <= rune(rg.Hi) {
				if rg.Stride != 1 {
					return 0, false
				}
				return (r - rune(rg.Lo)) % 10, true
			}
		}
		return 0, false
	}
	for _, rg := range unicode.Nd.R32 {
		if r >= rune(rg.Lo) && r <= rune(rg.Hi) {
			if rg.Stride != 1 {
				return 0, false
			}
			return (r - rune(rg.Lo)) % 10, true
		}
	}
	return 0, false
}

// Class returns the Separator / Ignorable flags of r (0 for ordinary runes).
func Class(r rune) uint8 {
	if isIgnorable(r) {
		return Ignorable
	}
	if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
		return Separator
	}
	return 0
}

func isIgnorable(r rune) bool {
	switch {
	case r >= 0x300 && r <= 0x36F, // combining diacritical marks
		r >= 0x1AB0 && r <= 0x1AFF,   // combining diacritical marks extended
		r >= 0x1DC0 && r <= 0x1DFF,   // combining diacritical marks supplement
		r >= 0x20D0 && r <= 0x20FF,   // combining marks for symbols
		r >= 0xFE00 && r <= 0xFE0F,   // variation selectors
		r >= 0xFE20 && r <= 0xFE2F,   // combining half marks
		r >= 0xE0100 && r <= 0xE01EF: // variation selectors supplement
		return true
	}
	return unicode.Is(unicode.Cf, r)
}

var (
	bmpOnce  sync.Once
	bmpFold  [1 << 16]uint16
	bmpClass [1 << 16]uint8
)

// BMP returns lazily-built lookup tables covering the Basic Multilingual
// Plane: fold[r] == Fold(r) and class[r] == Class(r) for every r < 0x10000.
// Both tables are built once per process and shared.
func BMP() (fold *[1 << 16]uint16, class *[1 << 16]uint8) {
	bmpOnce.Do(func() {
		for r := rune(0); r < 1<<16; r++ {
			f := Fold(r)
			if f >= 1<<16 {
				f = r
			}
			bmpFold[r] = uint16(f)
			bmpClass[r] = Class(r)
		}
	})
	return &bmpFold, &bmpClass
}
