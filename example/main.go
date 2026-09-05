// Command example demonstrates the go-swd API.
package main

import (
	"fmt"
	"log"

	"github.com/kirklin/go-swd"
)

func main() {
	// 1. 创建引擎（默认加载内置词库）
	engine, err := swd.New()
	if err != nil {
		log.Fatal(err)
	}

	// 2. 添加自定义敏感词；同一个词可以同时属于多个分类
	customWords := map[string]swd.Category{
		"涉黄":    swd.Pornography,
		"涉政":    swd.Political,
		"赌博词汇":  swd.Gambling,
		"毒品词汇":  swd.Drugs,
		"脏话词汇":  swd.Profanity,
		"歧视词汇":  swd.Discrimination,
		"诈骗词汇":  swd.Scam,
		"自定义词汇": swd.Custom,
		"多分类词汇": swd.Gambling | swd.Scam,
	}
	if err := engine.AddWords(customWords); err != nil {
		log.Fatal(err)
	}

	// 3. 基本检测
	text := "这是一段包含敏感词涉黄和涉政的文本"
	fmt.Println("是否包含敏感词:", engine.Detect(text))

	// 4. 按分类检测
	fmt.Println("是否包含涉黄内容:", engine.DetectIn(text, swd.Pornography))
	fmt.Println("是否包含涉政内容:", engine.DetectIn(text, swd.Political))
	fmt.Println("是否包含赌博内容:", engine.DetectIn(text, swd.Gambling))
	fmt.Println("是否包含涉黄或涉政内容:", engine.DetectIn(text, swd.Pornography, swd.Political))
	fmt.Println("是否包含任意预定义分类:", engine.DetectIn(text, swd.All))

	// 5. 第一个命中（最早结束的那个）
	if m := engine.Match(text); m != nil {
		fmt.Printf("首个敏感词: %s (分类: %s)\n", m.Word, m.Category)
	}

	// 6. 全部命中，带 rune 下标和字节偏移
	for _, m := range engine.MatchAll(text) {
		fmt.Printf("敏感词: %s (分类: %s, 位置: %d-%d, 原文: %q)\n",
			m.Word, m.Category, m.StartPos, m.EndPos, text[m.ByteStart:m.ByteEnd])
	}

	// 7. 迭代器：不分配结果切片，可随时 break
	for m := range engine.Matches(text) {
		fmt.Println("迭代到:", m.Word)
		break
	}

	// 8. 替换
	fmt.Println("星号替换:", engine.ReplaceWithAsterisk(text))
	fmt.Println("按分类替换:", engine.ReplaceWithStrategy(text, func(m swd.Match) string {
		return "[" + m.Category.String() + "]"
	}))
	fmt.Println("只替换涉政:", engine.ReplaceWithAsteriskIn(text, swd.Political))

	// 9. 归一化是内建的：大小写、全半角、数字样式、不可见字符
	_ = engine.AddWord("fuck", swd.Profanity)
	fmt.Println("变体检测:", engine.Detect("ＦＵＣＫ"), engine.Detect("𝐟𝐮𝐜𝐤"), engine.Detect("f\u200buck"))

	// 10. 白名单：落在允许短语内部的命中不再上报
	_ = engine.AddWord("色情", swd.Pornography)
	_ = engine.AddAllowWords("特色情怀")
	fmt.Println("白名单:", engine.Detect("特色情怀"), engine.Detect("特色情怀和色情"))

	// 11. 容忍分隔符和重复字符的实例
	loose, err := swd.New(swd.WithoutDefaultDict(), swd.WithWords(customWords),
		swd.WithMaxGap(1), swd.WithCollapseRepeats(true))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("间隔匹配:", loose.Detect("涉*黄"), loose.ReplaceWithAsterisk("这里有涉 黄内容"))

	// 12. 移除与清空
	if err := engine.RemoveWord("自定义词汇"); err != nil {
		log.Printf("移除敏感词失败: %v", err)
	}
	fmt.Println("词数:", engine.Len())
	if err := engine.Clear(); err != nil {
		log.Printf("清空词库失败: %v", err)
	}
	fmt.Println("清空后:", engine.Detect(text), engine.Len())
}
