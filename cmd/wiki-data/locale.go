package main

import (
	"bytes"
	"encoding/binary"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// localization reads the English column of the game's translation
// tables.
//
// Each table is a TextAsset in resources.assets: a CSV, one row per key,
// the languages as columns. A player build strips the type trees from
// resources.assets, so a table is found by its bytes: the header row is
// distinctive, and the length prefix of the string sits just before it
// (before the BOM, on the tables that carry one). The main table is the
// longest; the smaller ones (the console variants, the extra strings)
// fill in keys it lacks, which is what the game does when it loads them
// on top.
func localization(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	marker := []byte("\"\",\"English\",\"Swedish\"")
	bom := []byte("\xef\xbb\xbf")
	var tables [][]byte
	for at := 0; ; {
		i := bytes.Index(data[at:], marker)
		if i < 0 {
			break
		}
		i += at
		at = i + len(marker)
		start := i
		if start >= 3 && bytes.Equal(data[start-3:start], bom) {
			start -= 3
		}
		if start < 4 {
			continue
		}
		n := int(binary.LittleEndian.Uint32(data[start-4 : start]))
		if n <= 0 || start+n > len(data) {
			continue
		}
		tables = append(tables, data[i:start+n])
	}
	if len(tables) == 0 {
		return nil, fmt.Errorf("%s: no localization table found; its layout has changed", path)
	}
	sort.Slice(tables, func(i, j int) bool { return len(tables[i]) > len(tables[j]) })
	out := map[string]string{}
	for _, t := range tables {
		if err := readTable(t, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// readTable adds a table's English column for the keys not yet known.
func readTable(table []byte, out map[string]string) error {
	r := csv.NewReader(bytes.NewReader(table))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	header, err := r.Read()
	if err != nil {
		return fmt.Errorf("localization header: %w", err)
	}
	col := -1
	for i, h := range header {
		if h == "English" {
			col = i
			break
		}
	}
	if col < 0 {
		return fmt.Errorf("localization: no English column in %q", header)
	}
	for {
		row, err := r.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("localization: %w", err)
		}
		if len(row) <= col || row[0] == "" {
			continue
		}
		if _, known := out[row[0]]; !known {
			out[row[0]] = row[col]
		}
	}
}

// localize replaces every $token in s with its English text, the way
// Localization.Localize does, leaving a token it does not know as is.
func localize(strs map[string]string, s string) string {
	if !strings.Contains(s, "$") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '$' {
			b.WriteByte(s[i])
			i++
			continue
		}
		j := i + 1
		for j < len(s) && (isWordByte(s[j])) {
			j++
		}
		key := s[i+1 : j]
		if t, ok := strs[key]; ok {
			b.WriteString(t)
		} else {
			b.WriteString(s[i:j])
		}
		i = j
	}
	return b.String()
}

func isWordByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
