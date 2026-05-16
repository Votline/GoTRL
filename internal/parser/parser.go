// Package parser parses config file and returns UserData.
// Also it replaces macros with data.
package parser

import (
	"bytes"
	"fmt"
	"os"
	"unsafe"

	"github.com/Votline/Gurlf"
	gscan "github.com/Votline/Gurlf/pkg/scanner"
)

var macros = [][]byte{
	[]byte("ENVIRONMENT"),
}

type UserData struct {
	InpType string `gurlf:"InputType"`
	OutType string `gurlf:"OutputType"`
	TrlURL  string `gurlf:"TranslationURL"`
	SpttURL string `gurlf:"SpeechToTextURL"`
	TtsURL  string `gurlf:"TextToSpeechURL"`
	InfURL  string `gurlf:"InflectorURL"`
}

// Parse config file and return UserData.
func Parse(gData []gscan.Data) (*UserData, error) {
	const op = "parser.Parse"

	if len(gData) == 0 {
		return nil, fmt.Errorf("%s: no data", op)
	}

	keyBuf := make([]byte, 0, 36)
	fromBuf := make([]byte, 0, 36)
	newRaw := make([]byte, 0, int(float32(len(gData[0].RawData))*1.5))

	if err := parseMacros(&gData[0], &keyBuf, &fromBuf, &newRaw); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	ud := UserData{}
	if err := gurlf.Unmarshal(gData[0], &ud); err != nil {
		return nil, fmt.Errorf("%s: unmarshal: %w", op, err)
	}

	if len(gData) > 1 {
		return &ud, fmt.Errorf("%s: too many data: used only first", op)
	}

	return &ud, nil
}

// Replace macros with data.
func parseMacros(data *gscan.Data, keyBuf, fromBuf, newRaw *[]byte) error {
	const op = "parser.parseMacros"

	for i := len(data.Entries) - 1; i >= 0; i-- {
		e := &data.Entries[i]
		valRaw := data.RawData[e.ValStart:e.ValEnd]
		if err := handleEntry(&valRaw, keyBuf, fromBuf); err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}

		if !bytes.Equal(valRaw, data.RawData[e.ValStart:e.ValEnd]) {
			*newRaw = (*newRaw)[:0]
			oldLen := e.ValEnd - e.ValStart
			newLen := len(valRaw)
			delta := newLen - oldLen

			*newRaw = append(*newRaw, data.RawData[:e.ValStart]...)
			*newRaw = append(*newRaw, valRaw...)
			*newRaw = append(*newRaw, data.RawData[e.ValEnd:]...)
			data.RawData = *newRaw

			e.ValEnd = e.ValStart + newLen

			for j := i + 1; j < len(data.Entries); j++ {
				data.Entries[j].KeyStart += delta
				data.Entries[j].KeyEnd += delta
				data.Entries[j].ValStart += delta
				data.Entries[j].ValEnd += delta
			}
		}
	}

	return nil
}

// Find macro in data and replace it to valid data.
func handleEntry(data *[]byte, keyBuf *[]byte, fromBuf *[]byte) error {
	const op = "parser.handleEntry"

	valRaw := *data

	firstOpen := bytes.IndexByte(valRaw, '{')
	if firstOpen == -1 {
		return nil // no macros
	}
	firstOpen++ // skip '{'

	start := -1
	for _, m := range macros {
		start = bytes.Index(valRaw[firstOpen:], m)
		if start == -1 {
			continue
		}
		break
	}

	if start == -1 {
		return nil // no macros
	}
	start += firstOpen

	end := bytes.IndexByte(valRaw[start:], '}')
	if end == -1 {
		return fmt.Errorf("%s: no closing brace", op)
	}
	end += start

	macro := valRaw[start:end]
	trimSpaceBytes(&macro)

	if err := handleMacro(&macro, keyBuf, fromBuf); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	start-- // remove '{'
	end++   // remove '}'
	applyMacro(&valRaw, macro, start, end)

	if err := handleEntry(&valRaw, keyBuf, fromBuf); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	*data = valRaw

	return nil
}

// Find data in macro and copy it to keyBuf and fromBuf.
// Call correct handler for data.
func handleMacro(macro, keyBuf, fromBuf *[]byte) error {
	const op = "parser.handleMacro"

	sep := bytes.IndexByte(*macro, ' ')
	macroType := (*macro)[:sep]
	trimSpaceBytes(&macroType)

	end, err := findData(*macro, keyBuf)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if _, err := findData((*macro)[end:], fromBuf); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if bytes.Equal(macroType, []byte("ENVIRONMENT")) {
		if err := handleEnvironment(macro, keyBuf, *fromBuf); err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
		return nil
	}

	return fmt.Errorf("%s: no such key: %q", op, macroType)
}

// Find value after '=' and copy it to buf.
func findData(data []byte, buf *[]byte) (int, error) {
	const op = "parser.findData"

	start := bytes.IndexByte(data, '=')
	if start == -1 {
		return -1, fmt.Errorf("%s: no '=' in macro", op)
	}
	start++ // skip '='

	end := bytes.IndexByte(data[start:], ' ')
	if end == -1 {
		end = len(data)
	} else {
		end += start
	}

	if start >= end {
		return -1, fmt.Errorf("%s: invalid macro data: %q", op, data)
	}

	val := data[start:end]
	trimSpaceBytes(&val)

	copyBytes(buf, val)

	return end, nil
}

// Have 'os' and 'path' modes
// Get value from environment variable when 'os' mode
// Read 'path' file and find 'key' in file when 'path' mode
// For path mode used everything except 'os'
func handleEnvironment(macro, key *[]byte, from []byte) error {
	const op = "parser.handleEnvironment"

	keyStr := unsafe.String(unsafe.SliceData(*key), len(*key))
	fromStr := unsafe.String(unsafe.SliceData(from), len(from))

	if fromStr == "os" {
		val := os.Getenv(keyStr)
		valBytes := unsafe.Slice(unsafe.StringData(val), len(val))

		if len(valBytes) == 0 {
			*macro = nil
			return nil
		}

		copyBytes(macro, valBytes)
		return nil
	}

	if _, err := os.Stat(fromStr); err != nil {
		return fmt.Errorf("%s: no such file: %q", op, fromStr)
	}

	data, err := os.ReadFile(fromStr)
	if err != nil {
		return fmt.Errorf("%s: read file: %w", op, err)
	}

	val := key
	if err := getByKey(data, *key, val); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	copyBytes(macro, *val)

	return nil
}

// Find key in data and copy value to buf.
func getByKey(data, key []byte, buf *[]byte) error {
	const op = "parser.getByKey"

	if len(data) == 0 {
		*buf = nil
		return fmt.Errorf("%s: empty data", op)
	}

	idx := bytes.Index(data, key)
	if idx == -1 {
		return fmt.Errorf("%s: no such key: %q", op, key)
	}
	idx += len(key)

	start := bytes.IndexByte(data[idx:], '=')
	if start == -1 {
		return fmt.Errorf("%s: no '=' in key", op)
	}
	start += idx + 1 // skip '='

	var end int
	if data[start] == '"' {
		start++ // skip '"'

		searchFrom := start
		for {
			relIdx := bytes.IndexByte(data[searchFrom:], '"')
			if relIdx == -1 {
				end = len(data)
				break
			}

			absIdx := searchFrom + relIdx
			if absIdx > start {
				if absIdx-1 >= 0 && data[absIdx-1] == '\\' {
					if absIdx-2 >= 0 && data[absIdx-2] != '\\' {
						searchFrom = absIdx + 1
						continue
					}
				}
			}

			end = absIdx
			break
		}
	} else {
		relIdx := bytes.IndexByte(data[start:], '\n')
		if relIdx == -1 {
			end = len(data)
		} else {
			end = start + relIdx
		}
	}

	val := data[start:end]
	trimSpaceBytes(&val)

	copyBytes(buf, val)

	return nil
}

// Replace macro in original data to with valid data.
func applyMacro(valRaw *[]byte, val []byte, start, end int) {
	const op = "parser.applyMacro"

	newVal := make([]byte, 0, len(*valRaw)+len(val))
	newVal = append(newVal, (*valRaw)[:start]...)
	newVal = append(newVal, val...)
	newVal = append(newVal, (*valRaw)[end:]...)

	*valRaw = newVal
}

// Trim spaces from start and end of slice.
func trimSpaceBytes(b *[]byte) {
	t := *b
	start, end := 0, len(t)

	for start < end && isSpace(t[start]) {
		start++
	}

	for end > start && isSpace(t[end-1]) {
		end--
	}

	*b = t[start:end]
}

// Copy bytes to dst. If dst is too small, make new slice.
func copyBytes(dst *[]byte, src []byte) {
	if len(*dst) < len(src) {
		*dst = make([]byte, len(src))
		copy(*dst, src)
		return
	}
	*dst = src
}

// Check if byte is space.
func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// RangeByByte Split data by byte and call yield for each key-value pair.
func RangeByByte(b []byte, sep byte, yield func(key, val []byte)) {
	kS, kE := 0, 0
	vS, vE := 0, 0
	start, sepIdx := 0, 0

	for start < len(b) {
		sepIdx = bytes.IndexByte(b[start:], sep)
		if sepIdx == -1 {
			break
		}
		sepIdx += start

		// go to start of flag
		kS = sepIdx
		for kS > 0 && !isSpace(b[kS-1]) {
			kS--
		}
		kE = sepIdx + 1 // go to '='
		key := b[kS:kE]

		vS = sepIdx + 1

		// go to end of flag
		vE = vS
		for vE < len(b) && !isSpace(b[vE]) {
			vE++
		}

		val := b[vS:vE]

		yield(key, val)

		start = vE
	}
}
