package filter

import (
	"reflect"
	"strings"
	"testing"

	"github.com/kirklin/go-swd/pkg/core"
	"github.com/kirklin/go-swd/pkg/types/category"
)

// mockDetector 是一个用于测试的mock检测器
type mockDetector struct {
	matchAllFunc   func(text string) []core.SensitiveWord
	matchAllInFunc func(text string, categories ...category.Category) []core.SensitiveWord
}

func (m *mockDetector) Detect(text string) bool {
	return len(m.MatchAll(text)) > 0
}

func (m *mockDetector) DetectIn(text string, categories ...category.Category) bool {
	return len(m.MatchAllIn(text, categories...)) > 0
}

func (m *mockDetector) Match(text string) *core.SensitiveWord {
	matches := m.MatchAll(text)
	if len(matches) > 0 {
		return &matches[0]
	}
	return nil
}

func (m *mockDetector) MatchIn(text string, categories ...category.Category) *core.SensitiveWord {
	matches := m.MatchAllIn(text, categories...)
	if len(matches) > 0 {
		return &matches[0]
	}
	return nil
}

func (m *mockDetector) MatchAll(text string) []core.SensitiveWord {
	if m.matchAllFunc != nil {
		return m.matchAllFunc(text)
	}
	return nil
}

func (m *mockDetector) MatchAllIn(text string, categories ...category.Category) []core.SensitiveWord {
	if m.matchAllInFunc != nil {
		return m.matchAllInFunc(text, categories...)
	}
	return nil
}

func TestFilter_Replace(t *testing.T) {
	tests := []struct {
		name        string
		text        string
		replacement rune
		matches     []core.SensitiveWord
		want        string
	}{
		{
			name:        "empty text",
			text:        "",
			replacement: '*',
			matches:     nil,
			want:        "",
		},
		{
			name:        "no sensitive words",
			text:        "hello world",
			replacement: '*',
			matches:     nil,
			want:        "hello world",
		},
		{
			name:        "single sensitive word",
			text:        "hello bad world",
			replacement: '*',
			matches: []core.SensitiveWord{
				{Word: "bad", StartPos: 6, EndPos: 9, Category: category.Violence},
			},
			want: "hello *** world",
		},
		{
			name:        "multiple sensitive words",
			text:        "bad hello bad world",
			replacement: '#',
			matches: []core.SensitiveWord{
				{Word: "bad", StartPos: 0, EndPos: 3, Category: category.Violence},
				{Word: "bad", StartPos: 10, EndPos: 13, Category: category.Violence},
			},
			want: "### hello ### world",
		},
		{
			name:        "overlapping sensitive words",
			text:        "hello badword world",
			replacement: '*',
			matches: []core.SensitiveWord{
				{Word: "bad", StartPos: 6, EndPos: 9, Category: category.Violence},
				{Word: "word", StartPos: 9, EndPos: 13, Category: category.Violence},
			},
			want: "hello ******* world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := &mockDetector{
				matchAllFunc: func(text string) []core.SensitiveWord {
					return tt.matches
				},
			}
			f := NewFilter(detector)
			got := f.Replace(tt.text, tt.replacement)
			if got != tt.want {
				t.Errorf("Replace() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilter_ReplaceIn(t *testing.T) {
	tests := []struct {
		name        string
		text        string
		replacement rune
		categories  []category.Category
		matches     []core.SensitiveWord
		want        string
	}{
		{
			name:        "empty text",
			text:        "",
			replacement: '*',
			categories:  []category.Category{category.Violence},
			matches:     nil,
			want:        "",
		},
		{
			name:        "no categories",
			text:        "hello bad world",
			replacement: '*',
			categories:  nil,
			matches:     nil,
			want:        "hello bad world",
		},
		{
			name:        "single category match",
			text:        "hello bad world",
			replacement: '*',
			categories:  []category.Category{category.Violence},
			matches: []core.SensitiveWord{
				{Word: "bad", StartPos: 6, EndPos: 9, Category: category.Violence},
			},
			want: "hello *** world",
		},
		{
			name:        "multiple categories",
			text:        "bad hello evil world",
			replacement: '#',
			categories:  []category.Category{category.Violence, category.Pornography},
			matches: []core.SensitiveWord{
				{Word: "bad", StartPos: 0, EndPos: 3, Category: category.Violence},
				{Word: "evil", StartPos: 10, EndPos: 14, Category: category.Pornography},
			},
			want: "### hello #### world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := &mockDetector{
				matchAllInFunc: func(text string, categories ...category.Category) []core.SensitiveWord {
					if !reflect.DeepEqual(categories, tt.categories) {
						t.Errorf("Categories mismatch, got %v, want %v", categories, tt.categories)
					}
					return tt.matches
				},
			}
			f := NewFilter(detector)
			got := f.ReplaceIn(tt.text, tt.replacement, tt.categories...)
			if got != tt.want {
				t.Errorf("ReplaceIn() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilter_ReplaceWithAsterisk(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		matches []core.SensitiveWord
		want    string
	}{
		{
			name:    "empty text",
			text:    "",
			matches: nil,
			want:    "",
		},
		{
			name:    "no sensitive words",
			text:    "hello world",
			matches: nil,
			want:    "hello world",
		},
		{
			name: "single sensitive word",
			text: "hello bad world",
			matches: []core.SensitiveWord{
				{Word: "bad", StartPos: 6, EndPos: 9, Category: category.Violence},
			},
			want: "hello *** world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := &mockDetector{
				matchAllFunc: func(text string) []core.SensitiveWord {
					return tt.matches
				},
			}
			f := NewFilter(detector)
			got := f.ReplaceWithAsterisk(tt.text)
			if got != tt.want {
				t.Errorf("ReplaceWithAsterisk() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilter_ReplaceWithStrategy(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		strategy func(word core.SensitiveWord) string
		matches  []core.SensitiveWord
		want     string
	}{
		{
			name: "custom replacement strategy",
			text: "hello bad world",
			strategy: func(word core.SensitiveWord) string {
				return "[REMOVED]"
			},
			matches: []core.SensitiveWord{
				{Word: "bad", StartPos: 6, EndPos: 9, Category: category.Violence},
			},
			want: "hello [REMOVED] world",
		},
		{
			name: "length based replacement",
			text: "hello bad evil world",
			strategy: func(word core.SensitiveWord) string {
				return "?" + strings.Repeat("*", len([]rune(word.Word))-2) + "?"
			},
			matches: []core.SensitiveWord{
				{Word: "bad", StartPos: 6, EndPos: 9, Category: category.Violence},
				{Word: "evil", StartPos: 10, EndPos: 14, Category: category.Violence},
			},
			want: "hello ?*? ?**? world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := &mockDetector{
				matchAllFunc: func(text string) []core.SensitiveWord {
					return tt.matches
				},
			}
			f := NewFilter(detector)
			got := f.ReplaceWithStrategy(tt.text, tt.strategy)
			if got != tt.want {
				t.Errorf("ReplaceWithStrategy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilter_Replace_Additional(t *testing.T) {
	tests := []struct {
		name        string
		text        string
		replacement rune
		matches     []core.SensitiveWord
		want        string
	}{
		{
			name:        "chinese text",
			text:        "你好世界，这是一个测试",
			replacement: '*',
			matches: []core.SensitiveWord{
				{Word: "世界", StartPos: 2, EndPos: 4, Category: category.Violence},
			},
			want: "你好**，这是一个测试",
		},
		{
			name:        "long text with multiple matches",
			text:        strings.Repeat("hello bad world ", 100),
			replacement: '*',
			matches: []core.SensitiveWord{
				{Word: "bad", StartPos: 6, EndPos: 9, Category: category.Violence},
				{Word: "bad", StartPos: 22, EndPos: 25, Category: category.Violence},
				{Word: "bad", StartPos: 38, EndPos: 41, Category: category.Violence},
			},
			want: strings.Replace(strings.Repeat("hello bad world ", 100), "bad", "***", 3),
		},
		{
			name:        "complex overlapping words",
			text:        "hellobadwordworld",
			replacement: '*',
			matches: []core.SensitiveWord{
				{Word: "bad", StartPos: 5, EndPos: 8, Category: category.Violence},
				{Word: "word", StartPos: 8, EndPos: 12, Category: category.Violence},
			},
			want: "hello*******world",
		},
		{
			name:        "special characters",
			text:        "hello!@#$bad%^&*world",
			replacement: '*',
			matches: []core.SensitiveWord{
				{Word: "bad", StartPos: 9, EndPos: 12, Category: category.Violence},
			},
			want: "hello!@#$***%^&*world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := &mockDetector{
				matchAllFunc: func(text string) []core.SensitiveWord {
					return tt.matches
				},
			}
			f := NewFilter(detector)
			got := f.Replace(tt.text, tt.replacement)
			if got != tt.want {
				t.Errorf("Replace() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilter_ReplaceWithAsteriskIn(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		categories []category.Category
		matches    []core.SensitiveWord
		want       string
	}{
		{
			name:       "empty text",
			text:       "",
			categories: []category.Category{category.Violence},
			matches:    nil,
			want:       "",
		},
		{
			name:       "no categories",
			text:       "hello bad world",
			categories: nil,
			matches:    nil,
			want:       "hello bad world",
		},
		{
			name:       "single category match",
			text:       "hello bad world",
			categories: []category.Category{category.Violence},
			matches: []core.SensitiveWord{
				{Word: "bad", StartPos: 6, EndPos: 9, Category: category.Violence},
			},
			want: "hello *** world",
		},
		{
			name:       "multiple categories",
			text:       "bad hello evil world",
			categories: []category.Category{category.Violence, category.Pornography},
			matches: []core.SensitiveWord{
				{Word: "bad", StartPos: 0, EndPos: 3, Category: category.Violence},
				{Word: "evil", StartPos: 10, EndPos: 14, Category: category.Pornography},
			},
			want: "*** hello **** world",
		},
		{
			name:       "chinese sensitive words",
			text:       "你好世界，这是一个测试",
			categories: []category.Category{category.Violence},
			matches: []core.SensitiveWord{
				{Word: "世界", StartPos: 2, EndPos: 4, Category: category.Violence},
			},
			want: "你好**，这是一个测试",
		},
		{
			name:       "overlapping matches",
			text:       "helloworldbad",
			categories: []category.Category{category.Violence, category.Pornography},
			matches: []core.SensitiveWord{
				{Word: "world", StartPos: 5, EndPos: 10, Category: category.Violence},
				{Word: "bad", StartPos: 10, EndPos: 13, Category: category.Pornography},
			},
			want: "hello********",
		},
		{
			name:       "long text with multiple matches",
			text:       strings.Repeat("hello bad world ", 10),
			categories: []category.Category{category.Violence},
			matches: []core.SensitiveWord{
				{Word: "bad", StartPos: 6, EndPos: 9, Category: category.Violence},
				{Word: "bad", StartPos: 22, EndPos: 25, Category: category.Violence},
				{Word: "bad", StartPos: 38, EndPos: 41, Category: category.Violence},
			},
			want: strings.Replace(strings.Repeat("hello bad world ", 10), "bad", "***", 3),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := &mockDetector{
				matchAllInFunc: func(text string, categories ...category.Category) []core.SensitiveWord {
					if !reflect.DeepEqual(categories, tt.categories) {
						t.Errorf("Categories mismatch, got %v, want %v", categories, tt.categories)
					}
					return tt.matches
				},
			}
			f := NewFilter(detector)
			got := f.ReplaceWithAsteriskIn(tt.text, tt.categories...)
			if got != tt.want {
				t.Errorf("ReplaceWithAsteriskIn() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilter_ReplaceWithStrategy_Additional(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		strategy func(word core.SensitiveWord) string
		matches  []core.SensitiveWord
		want     string
	}{
		{
			name:     "nil strategy",
			text:     "hello bad world",
			strategy: nil,
			matches: []core.SensitiveWord{
				{Word: "bad", StartPos: 6, EndPos: 9, Category: category.Violence},
			},
			want: "hello bad world",
		},
		{
			name: "chinese text replacement",
			text: "你好世界，这是一个测试",
			strategy: func(word core.SensitiveWord) string {
				return "[敏感词]"
			},
			matches: []core.SensitiveWord{
				{Word: "世界", StartPos: 2, EndPos: 4, Category: category.Violence},
			},
			want: "你好[敏感词]，这是一个测试",
		},
		{
			name: "variable length replacement",
			text: "hello badword world",
			strategy: func(word core.SensitiveWord) string {
				return "<" + strings.Repeat("-", len([]rune(word.Word))) + ">"
			},
			matches: []core.SensitiveWord{
				{Word: "badword", StartPos: 6, EndPos: 13, Category: category.Violence},
			},
			want: "hello <-------> world",
		},
		{
			name: "category based replacement",
			text: "bad hello evil world",
			strategy: func(word core.SensitiveWord) string {
				switch word.Category {
				case category.Violence:
					return "[暴力]"
				case category.Pornography:
					return "[色情]"
				default:
					return "[敏感词]"
				}
			},
			matches: []core.SensitiveWord{
				{Word: "bad", StartPos: 0, EndPos: 3, Category: category.Violence},
				{Word: "evil", StartPos: 10, EndPos: 14, Category: category.Pornography},
			},
			want: "[暴力] hello [色情] world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := &mockDetector{
				matchAllFunc: func(text string) []core.SensitiveWord {
					return tt.matches
				},
			}
			f := NewFilter(detector)
			got := f.ReplaceWithStrategy(tt.text, tt.strategy)
			if got != tt.want {
				t.Errorf("ReplaceWithStrategy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilter_ReplaceWithStrategyIn(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		categories []category.Category
		strategy   func(word core.SensitiveWord) string
		matches    []core.SensitiveWord
		want       string
	}{
		{
			name:       "empty text",
			text:       "",
			categories: []category.Category{category.Violence},
			strategy: func(word core.SensitiveWord) string {
				return "[REMOVED]"
			},
			matches: nil,
			want:    "",
		},
		{
			name:       "nil strategy",
			text:       "hello bad world",
			categories: []category.Category{category.Violence},
			strategy:   nil,
			matches: []core.SensitiveWord{
				{Word: "bad", StartPos: 6, EndPos: 9, Category: category.Violence},
			},
			want: "hello bad world",
		},
		{
			name:       "single category with custom replacement",
			text:       "hello bad world",
			categories: []category.Category{category.Violence},
			strategy: func(word core.SensitiveWord) string {
				return "[CENSORED]"
			},
			matches: []core.SensitiveWord{
				{Word: "bad", StartPos: 6, EndPos: 9, Category: category.Violence},
			},
			want: "hello [CENSORED] world",
		},
		{
			name:       "multiple categories with different replacements",
			text:       "bad hello evil world",
			categories: []category.Category{category.Violence, category.Pornography},
			strategy: func(word core.SensitiveWord) string {
				switch word.Category {
				case category.Violence:
					return "[V]"
				case category.Pornography:
					return "[P]"
				default:
					return "[X]"
				}
			},
			matches: []core.SensitiveWord{
				{Word: "bad", StartPos: 0, EndPos: 3, Category: category.Violence},
				{Word: "evil", StartPos: 10, EndPos: 14, Category: category.Pornography},
			},
			want: "[V] hello [P] world",
		},
		{
			name:       "chinese text with custom replacement",
			text:       "你好世界，这是一个测试",
			categories: []category.Category{category.Violence},
			strategy: func(word core.SensitiveWord) string {
				return "【敏感词】"
			},
			matches: []core.SensitiveWord{
				{Word: "世界", StartPos: 2, EndPos: 4, Category: category.Violence},
			},
			want: "你好【敏感词】，这是一个测试",
		},
		{
			name:       "overlapping matches with length-based replacement",
			text:       "helloworldbad",
			categories: []category.Category{category.Violence, category.Pornography},
			strategy: func(word core.SensitiveWord) string {
				return strings.Repeat("*", len([]rune(word.Word)))
			},
			matches: []core.SensitiveWord{
				{Word: "world", StartPos: 5, EndPos: 10, Category: category.Violence},
				{Word: "bad", StartPos: 10, EndPos: 13, Category: category.Pornography},
			},
			want: "hello********",
		},
		{
			name:       "special characters in replacement",
			text:       "hello bad!@# world",
			categories: []category.Category{category.Violence},
			strategy: func(word core.SensitiveWord) string {
				return "<!@#>"
			},
			matches: []core.SensitiveWord{
				{Word: "bad!@#", StartPos: 6, EndPos: 12, Category: category.Violence},
			},
			want: "hello <!@#> world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := &mockDetector{
				matchAllInFunc: func(text string, categories ...category.Category) []core.SensitiveWord {
					if !reflect.DeepEqual(categories, tt.categories) {
						t.Errorf("Categories mismatch, got %v, want %v", categories, tt.categories)
					}
					return tt.matches
				},
			}
			f := NewFilter(detector)
			got := f.ReplaceWithStrategyIn(tt.text, tt.strategy, tt.categories...)
			if got != tt.want {
				t.Errorf("ReplaceWithStrategyIn() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilter_Replace_OverlappingWords(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		matches []core.SensitiveWord
		want    string
	}{
		{
			name: "Fully overlapping, should choose the longer word",
			text: "abchello",
			matches: []core.SensitiveWord{
				{Word: "abc", StartPos: 0, EndPos: 3, Category: category.Discrimination},
				{Word: "bc", StartPos: 1, EndPos: 3, Category: category.Discrimination},
			},
			want: "***hello",
		},
		{
			name: "Partially overlapping",
			text: "abcefg",
			matches: []core.SensitiveWord{
				{Word: "abc", StartPos: 0, EndPos: 3, Category: category.Discrimination},
				{Word: "cefg", StartPos: 2, EndPos: 6, Category: category.Discrimination},
			},
			want: "***efg",
		},
		{
			name: "Multiple overlapping matches, unsorted",
			text: "abchello",
			matches: []core.SensitiveWord{
				{Word: "bc", StartPos: 1, EndPos: 3, Category: category.Discrimination},  // Shorter match first
				{Word: "abc", StartPos: 0, EndPos: 3, Category: category.Discrimination}, // Longer match second
			},
			want: "***hello",
		},
		{
			name: "Adjacent, non-overlapping",
			text: "badword",
			matches: []core.SensitiveWord{
				{Word: "bad", StartPos: 0, EndPos: 3, Category: category.Discrimination},
				{Word: "word", StartPos: 3, EndPos: 7, Category: category.Discrimination},
			},
			want: "*******",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := &mockDetector{
				matchAllFunc: func(text string) []core.SensitiveWord {
					return tt.matches
				},
			}
			f := NewFilter(detector)
			got := f.Replace(tt.text, '*')
			if got != tt.want {
				t.Errorf("Replace() = %q, want %q", got, tt.want)
			}
		})
	}
}
