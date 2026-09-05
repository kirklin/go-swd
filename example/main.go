// Command example demonstrates the go-swd API.
package main

import (
	"fmt"
	"log"

	"github.com/kirklin/go-swd"
)

func main() {
	engine, err := swd.New()
	if err != nil {
		log.Fatal(err)
	}

	// 1. 整体判定：风险等级 + 处置建议 + 命中分类
	for _, text := range []string{
		"长期供应冰毒麻古，货到付款",
		"你他妈的傻逼，去死吧",
		"加我微信看福利，扫码进群",
		"本品为国家级产品，包治百病",
		"今天天气不错，我们一起去公园散步",
	} {
		r := engine.Check(text)
		fmt.Printf("%-28s %-6s %-8s %s\n",
			text, r.Risk, r.Suggestion(), r.Categories)
	}

	// 2. 逐个命中：标签、分类、风险、置信度、位置
	fmt.Println()
	for _, m := range engine.Check("出售仿真手枪和子弹，加微信详聊").Matches {
		fmt.Printf("  %-10s 标签=%-22s 分类=%-8s 风险=%-6s 置信=%3d 位置=%d-%d\n",
			m.Word, m.Label, m.Category, m.Risk, m.Confidence, m.StartPos, m.EndPos)
	}

	// 3. 按一级分类过滤
	fmt.Println()
	text := "这段文本包含裸聊直播和网上赌场"
	fmt.Println("含色情:", engine.DetectIn(text, swd.Pornography))
	fmt.Println("含违禁:", engine.DetectIn(text, swd.Contraband))
	fmt.Println("含涉政:", engine.DetectIn(text, swd.Political))

	// 4. 只处理高风险，中风险转人工
	fmt.Println()
	if r := engine.Check(text); r.Risk >= swd.RiskHigh {
		fmt.Println("直接拦截，主要原因:", r.Label.Chinese())
	}

	// 5. 自定义词：按标签加，继承该标签的分类与风险
	if err := engine.AddLabeledWords(map[string]swd.Label{
		"内部黑话甲": swd.ContrabandFraud,
		"内部黑话乙": swd.PromotionToSites,
	}); err != nil {
		log.Fatal(err)
	}
	// 也可以按分类加，风险默认为高
	if err := engine.AddWord("自定义词", swd.UserCategory(0)); err != nil {
		log.Fatal(err)
	}
	fmt.Println("\n自定义分类命中:", engine.Check("这里有自定义词").Categories)

	// 6. 替换
	fmt.Println("\n星号替换:", engine.ReplaceWithAsterisk("这段文本包含裸聊直播"))
	fmt.Println("按标签替换:", engine.ReplaceWithStrategy("这段文本包含裸聊直播",
		func(m swd.Match) string { return "[" + m.Label.Chinese() + "]" }))

	// 7. 白名单：落在允许短语内部的命中被抑制
	_ = engine.AddAllowWords("特色情怀")
	fmt.Println("\n白名单:", engine.Detect("特色情怀"), engine.Detect("特色情怀和色情片"))

	fmt.Printf("\n词库规模: %+v\n", engine.Stats())
}
