package normalize

import "testing"

func TestFold(t *testing.T) {
	cases := []struct {
		in, want rune
	}{
		{'A', 'a'}, {'z', 'z'}, {'5', '5'}, {'-', '-'},
		{'Ａ', 'a'}, {'ｚ', 'z'}, {'９', '9'}, {'！', '!'}, {'　', ' '},
		{'①', '1'}, {'⑼', '9'}, {'⒈', '1'}, {'⓪', '0'}, {'⓿', '0'}, {'⓵', '1'}, {'❾', '9'}, {'➈', '9'}, {'➒', '9'},
		{'⁰', '0'}, {'¹', '1'}, {'²', '2'}, {'³', '3'}, {'⁹', '9'}, {'₀', '0'}, {'₈', '8'},
		{'零', '0'}, {'〇', '0'}, {'一', '1'}, {'二', '2'}, {'九', '9'}, {'壹', '1'}, {'肆', '4'}, {'玖', '9'},
		{'十', '十'}, {'两', '两'}, {'中', '中'}, {'国', '国'},
		{'Ⓐ', 'a'}, {'ⓩ', 'z'}, {'⒜', 'a'}, {0x1F130, 'a'}, {0x1F189, 'z'}, {0x1F1E6, 'a'},
		{0x1D400, 'a'}, {0x1D41A, 'a'}, {0x1D433, 'z'}, {0x1D7CE, '0'}, {0x1D7D9, '1'}, {0x1D7FF, '9'},
		{'É', 'e'}, {'é', 'e'}, {'ñ', 'n'}, {'ł', 'l'}, {'ž', 'z'}, {'ÿ', 'y'}, {'ß', 's'}, {'×', '×'},
		{'Δ', 'δ'}, {'Ж', 'ж'}, {'٣', '3'}, {'३', '3'},
		{0x200B, 0x200B}, {0x1F642, 0x1F642},
	}
	for _, c := range cases {
		if got := Fold(c.in); got != c.want {
			t.Errorf("Fold(%U %q) = %U %q, want %U %q", c.in, c.in, got, got, c.want, c.want)
		}
	}
}

func TestClass(t *testing.T) {
	cases := []struct {
		in   rune
		want uint8
	}{
		{' ', Separator}, {'\t', Separator}, {'　', Separator}, {'*', Separator}, {'，', Separator},
		{'。', Separator}, {'$', Separator}, {'+', Separator}, {0x1F642, Separator}, {'_', Separator},
		{0x200B, Ignorable}, {0x200D, Ignorable}, {0xFEFF, Ignorable}, {0xAD, Ignorable},
		{0x301, Ignorable}, {0x336, Ignorable}, {0xFE0F, Ignorable}, {0xE0100, Ignorable},
		{'a', 0}, {'中', 0}, {'5', 0}, {'é', 0},
	}
	for _, c := range cases {
		if got := Class(c.in); got != c.want {
			t.Errorf("Class(%U) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestBMPTablesMatchFunctions(t *testing.T) {
	fold, class := BMP()
	for r := rune(0); r < 1<<16; r++ {
		if rune(fold[r]) != Fold(r) {
			t.Fatalf("fold table mismatch at %U: %U vs %U", r, fold[r], Fold(r))
		}
		if class[r] != Class(r) {
			t.Fatalf("class table mismatch at %U", r)
		}
	}
}

func TestFoldIsIdempotent(t *testing.T) {
	for r := rune(0); r < 0x30000; r++ {
		f := Fold(r)
		if Fold(f) != f {
			t.Fatalf("Fold not idempotent at %U: Fold=%U, Fold(Fold)=%U", r, f, Fold(f))
		}
	}
}
