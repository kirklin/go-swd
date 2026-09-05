package swd

import (
	"fmt"
	"strings"
)

// Category is the first-level classification of a sensitive word. It is a
// bitmask, so a word may belong to several categories at once and several
// categories can be combined with | when filtering.
//
// There are ten categories. Fraud, forged documents and personal-data
// trading are labels under Contraband rather than categories of their own,
// because all three are illegal transactions.
type Category uint32

// The ten first-level categories.
const (
	None Category = 0

	Pornography   Category = 1 << 1  // 色情低俗
	Political     Category = 1 << 2  // 涉政
	Violence      Category = 1 << 3  // 暴恐
	Contraband    Category = 1 << 4  // 违禁，含毒品、赌博、诈骗、伪造证件、买卖个人信息
	Inappropriate Category = 1 << 5  // 不良内容，含歧视、辱骂、价值观、迷信、灌水
	Promotion     Category = 1 << 6  // 引流广告
	Religion      Category = 1 << 7  // 宗教
	AdCompliance  Category = 1 << 8  // 广告法违规
	AIGC          Category = 1 << 9  // AI 生成内容
	Custom        Category = 1 << 10 // 自定义词库

	// All is every predefined category combined.
	All = Pornography | Political | Violence | Contraband | Inappropriate |
		Promotion | Religion | AdCompliance | AIGC | Custom

	// userCategoryShift is the first bit available for caller-defined
	// categories. Bits 11 to 31 are never used by this package.
	userCategoryShift = 11
)

// UserCategory returns the n-th caller-defined category, counting from 0. Up
// to 21 of them exist; they never collide with the predefined ones and are
// accepted everywhere a Category is.
//
// UserCategory panics if n is out of range.
func UserCategory(n int) Category {
	if n < 0 || n >= 32-userCategoryShift {
		panic(fmt.Sprintf("swd: user category %d out of range [0,%d)", n, 32-userCategoryShift))
	}
	return 1 << (userCategoryShift + n)
}

var categoryNames = [...]struct {
	c    Category
	name string
}{
	{Pornography, "色情低俗"},
	{Political, "涉政"},
	{Violence, "暴恐"},
	{Contraband, "违禁"},
	{Inappropriate, "不良内容"},
	{Promotion, "引流广告"},
	{Religion, "宗教"},
	{AdCompliance, "广告法"},
	{AIGC, "AI生成"},
	{Custom, "自定义"},
}

// String returns the Chinese name of the category. Combined categories are
// joined with "|"; None is "无风险".
func (c Category) String() string {
	if c == None {
		return "无风险"
	}
	var parts []string
	for _, cn := range categoryNames {
		if c&cn.c != 0 {
			parts = append(parts, cn.name)
		}
	}
	for n := 0; n < 32-userCategoryShift; n++ {
		if c&UserCategory(n) != 0 {
			parts = append(parts, fmt.Sprintf("自定义%d", n))
		}
	}
	return strings.Join(parts, "|")
}

// Contains reports whether c includes every category in other.
// Contains(None) is always false.
func (c Category) Contains(other Category) bool {
	return other != None && c&other == other
}

// userCategoryMask covers every bit UserCategory can return.
const userCategoryMask = Category((1<<32 - 1) &^ (1<<userCategoryShift - 1))

// IsValid reports whether c uses only predefined or caller-defined category
// bits. Bit 0 belongs to neither and is rejected, so a mistyped constant is
// still caught.
func (c Category) IsValid() bool {
	return c&^(All|userCategoryMask) == 0
}

func orCategories(cats []Category) Category {
	var mask Category
	for _, c := range cats {
		mask |= c
	}
	return mask
}

// Risk is how a match should be handled.
type Risk uint8

// Risk levels, in increasing severity.
const (
	// RiskNone means nothing was detected.
	RiskNone Risk = iota
	// RiskLow is a weak signal: act on it only when recall matters more
	// than precision, otherwise treat it like RiskNone.
	RiskLow
	// RiskMedium is a probable violation: queue it for human review.
	RiskMedium
	// RiskHigh is a clear violation: block it.
	RiskHigh
)

var riskNames = [...]string{"无风险", "低风险", "中风险", "高风险"}

// String returns the Chinese name of the risk level.
func (r Risk) String() string {
	if int(r) < len(riskNames) {
		return riskNames[r]
	}
	return fmt.Sprintf("Risk(%d)", uint8(r))
}

// Suggestion returns the handling advice for the risk level: "pass",
// "review" or "block".
func (r Risk) Suggestion() string {
	switch r {
	case RiskHigh:
		return "block"
	case RiskMedium:
		return "review"
	default:
		return "pass"
	}
}
