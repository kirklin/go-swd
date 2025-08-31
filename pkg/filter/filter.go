package filter

import (
	"sort"

	"github.com/kirklin/go-swd/pkg/core"
	"github.com/kirklin/go-swd/pkg/types/category"
)

// filter 实现了敏感词过滤器接口
type filter struct {
	detector core.Detector
}

// NewFilter 创建一个新的过滤器实例
func NewFilter(detector core.Detector) core.Filter {
	return &filter{
		detector: detector,
	}
}

// Replace 使用指定的替换字符替换敏感词
func (f *filter) Replace(text string, replacement rune) string {
	if text == "" {
		return text
	}
	return f.ReplaceWithStrategy(text, func(word core.SensitiveWord) string {
		chars := make([]rune, len([]rune(word.Word)))
		for i := range chars {
			chars[i] = replacement
		}
		return string(chars)
	})
}

// ReplaceIn 使用指定的替换字符替换指定分类的敏感词
func (f *filter) ReplaceIn(text string, replacement rune, categories ...category.Category) string {
	if text == "" || len(categories) == 0 {
		return text
	}

	// 获取所有匹配的敏感词
	matches := f.detector.MatchAllIn(text, categories...)
	if len(matches) == 0 {
		return text
	}

	// 按照位置排序，从后向前替换
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].StartPos > matches[j].StartPos
	})

	// 转换为rune数组以便处理中文
	runes := []rune(text)
	for _, match := range matches {
		// 替换敏感词
		for i := match.StartPos; i < match.EndPos; i++ {
			runes[i] = replacement
		}
	}

	return string(runes)
}

// ReplaceWithAsterisk 使用 * 号替换敏感词
func (f *filter) ReplaceWithAsterisk(text string) string {
	return f.Replace(text, '*')
}

// ReplaceWithAsteriskIn 使用 * 号替换指定分类的敏感词
func (f *filter) ReplaceWithAsteriskIn(text string, categories ...category.Category) string {
	return f.ReplaceIn(text, '*', categories...)
}

// ReplaceWithStrategy 使用自定义替换策略替换敏感词
func (f *filter) ReplaceWithStrategy(text string, strategy func(word core.SensitiveWord) string) string {
	if text == "" || strategy == nil {
		return text
	}
	matches := f.detector.MatchAll(text)
	return f.replaceWords(text, matches, strategy)
}

// ReplaceWithStrategyIn 使用自定义策略替换指定分类的敏感词
func (f *filter) ReplaceWithStrategyIn(text string, strategy func(word core.SensitiveWord) string, categories ...category.Category) string {
	if text == "" || len(categories) == 0 || strategy == nil {
		return text
	}

	// 获取所有匹配的敏感词
	matches := f.detector.MatchAllIn(text, categories...)
	if len(matches) == 0 {
		return text
	}

	// 按照位置排序，从后向前替换
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].StartPos > matches[j].StartPos
	})

	// 转换为rune数组以便处理中文
	result := []rune(text)
	for _, match := range matches {
		// 获取替换文本
		replacement := []rune(strategy(match))
		// 计算需要替换的长度
		replaceLen := match.EndPos - match.StartPos
		if len(replacement) != replaceLen {
			// 如果替换文本长度不同，需要调整结果数组
			newResult := make([]rune, len(result)+(len(replacement)-replaceLen))
			copy(newResult, result[:match.StartPos])
			copy(newResult[match.StartPos:], replacement)
			copy(newResult[match.StartPos+len(replacement):], result[match.EndPos:])
			result = newResult
		} else {
			// 长度相同，直接替换
			copy(result[match.StartPos:], replacement)
		}
	}

	return string(result)
}

// replaceWords 替换文本中的敏感词
func (f *filter) replaceWords(text string, matches []core.SensitiveWord, strategy func(word core.SensitiveWord) string) string {
	if len(matches) == 0 {
		return text
	}

	// 按照 StartPos 排序
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].StartPos == matches[j].StartPos {
			return matches[i].EndPos > matches[j].EndPos
		}
		return matches[i].StartPos < matches[j].StartPos
	})

	runes := []rune(text)
	result := make([]rune, 0, len(runes))
	lastPos := 0

	for _, match := range matches {
		// 如果当前匹配的起始位置在上一个匹配的结束位置之前，说明是重叠词，跳过
		if match.StartPos < lastPos {
			continue
		}
		// 添加敏感词前的文本
		result = append(result, runes[lastPos:match.StartPos]...)
		// 添加替换后的文本
		replacement := []rune(strategy(match))
		result = append(result, replacement...)
		lastPos = match.EndPos
	}

	// 添加最后一个敏感词后的文本
	if lastPos < len(runes) {
		result = append(result, runes[lastPos:]...)
	}

	return string(result)
}
