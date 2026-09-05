package swd

import (
	_ "embed"
	"strings"
)

//go:embed dict/pornography.txt
var dictPornography string

//go:embed dict/political.txt
var dictPolitical string

//go:embed dict/violence.txt
var dictViolence string

//go:embed dict/gambling.txt
var dictGambling string

//go:embed dict/drugs.txt
var dictDrugs string

//go:embed dict/profanity.txt
var dictProfanity string

//go:embed dict/discrimination.txt
var dictDiscrimination string

//go:embed dict/scam.txt
var dictScam string

//go:embed dict/all.txt
var dictAll string

var defaultDict = [...]struct {
	data string
	cat  Category
}{
	{dictPornography, Pornography},
	{dictPolitical, Political},
	{dictViolence, Violence},
	{dictGambling, Gambling},
	{dictDrugs, Drugs},
	{dictProfanity, Profanity},
	{dictDiscrimination, Discrimination},
	{dictScam, Scam},
	{dictAll, None},
}

// parseLines calls fn for every non-empty, non-comment line of data.
func parseLines(data string, fn func(word string)) {
	for len(data) > 0 {
		line := data
		if i := strings.IndexByte(data, '\n'); i >= 0 {
			line, data = data[:i], data[i+1:]
		} else {
			data = ""
		}
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		fn(line)
	}
}
