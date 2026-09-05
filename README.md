# go-swd

![banner](./README.assets/banner.png)

[![Go Reference](https://pkg.go.dev/badge/github.com/kirklin/go-swd.svg)](https://pkg.go.dev/github.com/kirklin/go-swd)
[![CI](https://github.com/kirklin/go-swd/actions/workflows/ci.yml/badge.svg)](https://github.com/kirklin/go-swd/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/kirklin/go-swd/branch/main/graph/badge.svg)](https://codecov.io/gh/kirklin/go-swd)
[![Go Report Card](https://goreportcard.com/badge/github.com/kirklin/go-swd)](https://goreportcard.com/report/github.com/kirklin/go-swd)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

go-swd 是一个 Go 语言的敏感词检测与过滤库。它基于 Aho-Corasick 自动机，内置约四万词的中文词库，支持自定义词库、按分类过滤、白名单、文本替换，以及大小写、全半角、数字样式等字符变体的自动归一。

## 安装

```bash
go get github.com/kirklin/go-swd
```

需要 Go 1.25 或更高版本，无第三方依赖。

## 使用

```go
import "github.com/kirklin/go-swd"

engine, err := swd.New()
if err != nil {
	return err
}

engine.Detect(text)              // 是否包含敏感词
engine.MatchAll(text)            // 全部命中，含位置与分类
engine.ReplaceWithAsterisk(text) // 用 * 遮盖命中
```

### 自定义词库

```go
engine, err := swd.New(
	swd.WithWords(map[string]swd.Category{
		"示例词":  swd.Custom,
		"多分类词": swd.Gambling | swd.Scam,
	}),
)

err = engine.AddWord("新增词", swd.Political) // 返回后即对查询可见
err = engine.RemoveWord("示例词")
```

不加载内置词库：`swd.New(swd.WithoutDefaultDict())`。批量修改使用 `AddWords` 和 `RemoveWords`。

### 按分类

```go
engine.DetectIn(text, swd.Pornography, swd.Political)
engine.MatchAllIn(text, swd.All)
engine.ReplaceWithAsteriskIn(text, swd.Profanity)
```

分类为位掩码，一个词可以同时属于多个分类。内置分类：`Pornography`、`Political`、`Violence`、`Gambling`、`Drugs`、`Profanity`、`Discrimination`、`Scam`、`Custom`；`All` 为全部。通用词库 `dict/all.txt` 中的词不带分类，只有不带 `In` 后缀的方法会返回它们。

### 替换

```go
engine.Replace(text, '#')
engine.ReplaceWithStrategy(text, func(m swd.Match) string {
	return "[" + m.Category.String() + "]"
})
```

重叠的命中会先合并为一个区间再替换。

### 字符归一化

词库和文本经过同一套折叠规则，以下写法在匹配时等价，无需配置：

| 类型 | 示例 |
|---|---|
| 大小写、全半角 | `Fuck`、`ｆｕｃｋ` |
| 数字样式 | `1`、`１`、`①`、`⁹`、`一`、`壹`、`𝟙` |
| 带圈字母、数学字母 | `Ⓐ`、`𝐚`、`🅰` |
| 带变音符的拉丁字母 | `é`、`ñ`、`ł` |

零宽字符、组合标记和变体选择符在匹配时被忽略。

可选的宽松匹配：

```go
swd.New(swd.WithMaxGap(1))             // 容忍词内的分隔符：f*u*c*k、法 轮 功
swd.New(swd.WithCollapseRepeats(true)) // 折叠重复字符：fuuuck
```

### 白名单

```go
engine, err := swd.New(swd.WithAllowWords("特色情怀"))
engine.Detect("特色情怀") // false：落在允许短语内部的命中被抑制
```

运行期使用 `AddAllowWords` 和 `RemoveAllowWords` 调整。

### 命中结果

```go
type Match struct {
	Word      string   // 词库中的原始写法
	StartPos  int      // rune 下标
	EndPos    int      // rune 下标，开区间
	ByteStart int      // 字节偏移
	ByteEnd   int      // 字节偏移，开区间
	Category  Category
}
```

`text[m.ByteStart:m.ByteEnd]` 是命中的原文片段。`Match` 返回最早结束的命中；`MatchAll` 按结束位置排序，包含重叠的命中；`Matches` 返回迭代器，适合大量结果。

## 并发

`Engine` 可以在多个 goroutine 中共享。查询不加锁；更新词库时重建自动机，期间的查询不受影响。

## 词库

[dict/](dict/) 目录下为内置词库：纯文本，一行一个词，`#` 开头为注释。文件名对应分类，`all.txt` 为不带分类的通用词库。
[dict/strict/](dict/strict/) 是一份更严格的词库，可自行读取后通过 `AddWords` 加载。

## 文档

- API 参考：[pkg.go.dev/github.com/kirklin/go-swd](https://pkg.go.dev/github.com/kirklin/go-swd)


## 赞助

如果这个项目对你有帮助，欢迎通过 [GitHub Sponsors](https://github.com/sponsors/kirklin)、[Patreon](https://www.patreon.com/kirklin) 或 [Buy Me a Coffee](https://www.buymeacoffee.com/linkirk) 支持。

## 许可证

Apache License 2.0，见 [LICENSE](LICENSE)。
