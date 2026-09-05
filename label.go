package swd

import "fmt"

// Label is the second-level classification of a sensitive word. Every label
// belongs to exactly one Category and carries a default Risk, so a match can
// be routed without any further configuration.
//
// Each label corresponds to one file under dict/, so the taxonomy lives in
// the file layout.
type Label uint8

// The labels, grouped by category.
const (
	LabelNone Label = iota

	// 色情低俗
	PornographicAdult // 色情内容
	SexualSuggestive  // 低俗性暗示
	SexualTerms       // 性健康与两性科普，识别但通常放行

	// 涉政
	PoliticalSensitive // 敏感政治内容
	PoliticalFigure    // 涉政人物
	PoliticalEntity    // 涉政组织与实体

	// 暴恐
	ViolentExtremist // 极端组织与恐怖主义
	ViolentIncidents // 暴力伤害行为
	ViolentWeapons   // 武器弹药与危险品

	// 违禁
	ContrabandDrug     // 毒品
	ContrabandGambling // 赌博
	ContrabandAct      // 违禁行为
	ContrabandEntity   // 违禁物品与工具
	ContrabandFraud    // 诈骗话术
	ContrabandForgery  // 伪造证件与凭证
	ContrabandPrivacy  // 买卖个人信息

	// 不良内容
	InappropriateDiscrimination // 偏见歧视
	InappropriateProfanity      // 攻击辱骂
	InappropriateOral           // 低俗口头语
	InappropriateEthics         // 不良价值观
	InappropriateSuperstition   // 封建迷信
	InappropriateNonsense       // 无意义灌水

	// 引流广告
	PromotionToSites     // 站外引流
	PromotionRecruitment // 网赚兼职广告
	PromotionContact     // 引流联系方式

	// 宗教
	ReligionGeneral // 宗教内容

	// 广告法
	AdComplianceViolation // 广告法违规用语

	// AI 生成
	AIGCGenerated // AI 生成内容特征

	// 自定义
	Customized // 命中自定义词库

	numLabels
)

type labelInfo struct {
	name string // dict/<name>.txt
	zh   string
	cat  Category
	risk Risk
	// conf is the base confidence for this label, 0-100. It reflects how
	// reliably a keyword of this class indicates a real violation, and is
	// adjusted per word by length in confidenceOf.
	conf uint8
}

var labelTable = [numLabels]labelInfo{
	LabelNone: {"", "无", None, RiskNone, 0},

	PornographicAdult: {"pornographic_adult", "色情内容", Pornography, RiskHigh, 90},
	SexualSuggestive:  {"sexual_suggestive", "低俗性暗示", Pornography, RiskMedium, 75},
	SexualTerms:       {"sexual_terms", "性健康内容", Pornography, RiskLow, 60},

	PoliticalSensitive: {"political_sensitive", "敏感政治内容", Political, RiskHigh, 85},
	PoliticalFigure:    {"political_figure", "涉政人物", Political, RiskMedium, 70},
	PoliticalEntity:    {"political_entity", "涉政组织", Political, RiskMedium, 70},

	ViolentExtremist: {"violent_extremist", "极端组织", Violence, RiskHigh, 90},
	ViolentIncidents: {"violent_incidents", "暴力伤害", Violence, RiskMedium, 75},
	ViolentWeapons:   {"violent_weapons", "武器弹药", Violence, RiskHigh, 85},

	ContrabandDrug:     {"contraband_drug", "毒品", Contraband, RiskHigh, 90},
	ContrabandGambling: {"contraband_gambling", "赌博", Contraband, RiskHigh, 85},
	ContrabandAct:      {"contraband_act", "违禁行为", Contraband, RiskHigh, 85},
	ContrabandEntity:   {"contraband_entity", "违禁物品", Contraband, RiskMedium, 75},
	ContrabandFraud:    {"contraband_fraud", "诈骗话术", Contraband, RiskHigh, 85},
	ContrabandForgery:  {"contraband_forgery", "伪造证件", Contraband, RiskHigh, 85},
	ContrabandPrivacy:  {"contraband_privacy", "买卖个人信息", Contraband, RiskHigh, 85},

	InappropriateDiscrimination: {"inappropriate_discrimination", "偏见歧视", Inappropriate, RiskHigh, 85},
	InappropriateProfanity:      {"inappropriate_profanity", "攻击辱骂", Inappropriate, RiskMedium, 80},
	InappropriateOral:           {"inappropriate_oral", "低俗口头语", Inappropriate, RiskLow, 55},
	InappropriateEthics:         {"inappropriate_ethics", "不良价值观", Inappropriate, RiskMedium, 70},
	InappropriateSuperstition:   {"inappropriate_superstition", "封建迷信", Inappropriate, RiskLow, 60},
	InappropriateNonsense:       {"inappropriate_nonsense", "无意义灌水", Inappropriate, RiskLow, 50},

	PromotionToSites:     {"pt_to_sites", "站外引流", Promotion, RiskMedium, 75},
	PromotionRecruitment: {"pt_by_recruitment", "网赚兼职广告", Promotion, RiskMedium, 75},
	PromotionContact:     {"pt_to_contact", "引流联系方式", Promotion, RiskLow, 55},

	ReligionGeneral: {"religion_general", "宗教内容", Religion, RiskLow, 55},

	AdComplianceViolation: {"ad_compliance", "广告法违规", AdCompliance, RiskLow, 60},

	AIGCGenerated: {"aigc", "AI生成特征", AIGC, RiskLow, 55},

	Customized: {"customized", "自定义", Custom, RiskHigh, 100},
}

// String returns the label's stable identifier, e.g. "pornographic_adult".
// It is the name of the dictionary file the label is loaded from.
func (l Label) String() string {
	if int(l) < len(labelTable) && labelTable[l].name != "" {
		return labelTable[l].name
	}
	return fmt.Sprintf("Label(%d)", uint8(l))
}

// Chinese returns the label's Chinese name.
func (l Label) Chinese() string {
	if int(l) < len(labelTable) {
		return labelTable[l].zh
	}
	return l.String()
}

// Category returns the first-level category the label belongs to.
func (l Label) Category() Category {
	if int(l) < len(labelTable) {
		return labelTable[l].cat
	}
	return None
}

// Risk returns the label's default risk level.
func (l Label) Risk() Risk {
	if int(l) < len(labelTable) {
		return labelTable[l].risk
	}
	return RiskNone
}

// Labels returns every predefined label, in declaration order.
func Labels() []Label {
	out := make([]Label, 0, numLabels-1)
	for l := LabelNone + 1; l < numLabels; l++ {
		out = append(out, l)
	}
	return out
}

// labelByName maps a dictionary file name to its label.
var labelByName = func() map[string]Label {
	m := make(map[string]Label, numLabels)
	for l := LabelNone + 1; l < numLabels; l++ {
		if n := labelTable[l].name; n != "" {
			m[n] = l
		}
	}
	return m
}()

// confidenceOf scores how reliably a word of this label indicates a real
// violation, 0-100. Short words match by accident far more often than long
// ones — the measured false positive rate on ordinary Chinese text is
// dominated by two-character entries — so length adjusts the label's base.
func confidenceOf(l Label, runes int) uint8 {
	base := int(labelTable[l].conf)
	switch {
	case runes <= 2:
		base -= 25
	case runes == 3:
		base -= 12
	case runes == 4:
		base -= 4
	case runes >= 6:
		base += 5
	}
	switch {
	case base < 1:
		return 1
	case base > 100:
		return 100
	}
	return uint8(base)
}
