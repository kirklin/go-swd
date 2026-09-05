package swd

import (
	"fmt"
	"strings"
)

// Category is a bitmask of sensitive-word categories. A word may belong to
// several categories at once; combine categories with |.
type Category uint32

// Predefined categories. The bit values are stable across versions.
const (
	None           Category = 0
	Pornography    Category = 1 << 1 // 涉黄
	Political      Category = 1 << 2 // 涉政
	Violence       Category = 1 << 3 // 暴力
	Gambling       Category = 1 << 4 // 赌博
	Drugs          Category = 1 << 5 // 毒品
	Profanity      Category = 1 << 6 // 脏话
	Discrimination Category = 1 << 7 // 歧视
	Scam           Category = 1 << 8 // 诈骗
	Custom         Category = 1 << 9 // 自定义

	// All is every predefined category combined.
	All = Pornography | Political | Violence | Gambling | Drugs | Profanity | Discrimination | Scam | Custom
)

var categoryNames = [...]struct {
	c    Category
	name string
}{
	{Pornography, "涉黄"},
	{Political, "涉政"},
	{Violence, "暴力"},
	{Gambling, "赌博"},
	{Drugs, "毒品"},
	{Profanity, "脏话"},
	{Discrimination, "歧视"},
	{Scam, "诈骗"},
	{Custom, "自定义"},
}

// String returns the Chinese name of the category; combined categories are
// joined with "|" and None is "未分类".
func (c Category) String() string {
	if c == None {
		return "未分类"
	}
	var parts []string
	for _, cn := range categoryNames {
		if c&cn.c != 0 {
			parts = append(parts, cn.name)
		}
	}
	if rest := c &^ All; rest != 0 {
		parts = append(parts, fmt.Sprintf("未知(0x%x)", uint32(rest)))
	}
	return strings.Join(parts, "|")
}

// Contains reports whether c includes every category in other.
// Contains(None) is always false.
func (c Category) Contains(other Category) bool {
	return other != None && c&other == other
}

// IsValid reports whether c only uses predefined category bits.
func (c Category) IsValid() bool {
	return c&^All == 0
}

func orCategories(cats []Category) Category {
	var mask Category
	for _, c := range cats {
		mask |= c
	}
	return mask
}
