package swd

import (
	"embed"
	"path"
	"strings"
)

// dictFS holds the built-in dictionary. Every file is named after the Label
// its words carry, so the taxonomy lives in the file layout: adding a file
// named after a label is all it takes to extend the dictionary.
//
//go:embed dict/*.txt
var dictFS embed.FS

// entry is one dictionary word together with the label it was loaded under.
type entry struct {
	word  string
	label Label
}

// defaultDict reads the embedded dictionary. Files whose name does not match
// a known label are ignored, so an unknown file can never silently become an
// uncategorised word.
func defaultDict() ([]entry, error) {
	files, err := dictFS.ReadDir("dict")
	if err != nil {
		return nil, err
	}
	out := make([]entry, 0, 1<<14)
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".txt") {
			continue
		}
		label, ok := labelByName[strings.TrimSuffix(f.Name(), ".txt")]
		if !ok {
			continue
		}
		data, err := dictFS.ReadFile(path.Join("dict", f.Name()))
		if err != nil {
			return nil, err
		}
		parseLines(string(data), func(w string) {
			out = append(out, entry{w, label})
		})
	}
	return out, nil
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
