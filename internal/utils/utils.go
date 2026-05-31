// Package utils utils.go contains helper functions
package utils

import (
	"fmt"
	"os"
	"slices"
	"unsafe"

	"gotrl/internal/parser"

	gurlf "github.com/Votline/Gurlf"
	gscan "github.com/Votline/Gurlf/pkg/scanner"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func parseArgs(args []string) (*parser.UserData, error) {
	const op = "main.parseArgs"

	ud := parser.UserData{}

	for _, arg := range args {
		argByte := unsafe.Slice(unsafe.StringData(arg), len(arg))
		parser.RangeByByte(argByte, ' ', func(key, val []byte) {
			keyStr := unsafe.String(unsafe.SliceData(key), len(key))
			valStr := unsafe.String(unsafe.SliceData(val), len(val))
			switch keyStr {
			case "-trl", "--translationURL":
				ud.TrlURL = valStr
			case "-stt", "--speechToTextURL":
				ud.SttURL = valStr
			case "-tts", "--textToSpeechURL":
				ud.TtsURL = valStr
			case "-inf", "--inflectionURL":
				ud.InfURL = valStr
			case "-itt", "--imageToTextURL":
				ud.IttURL = valStr
			}
		})
	}

	if ud.TrlURL == "" && ud.SttURL == "" && ud.TtsURL == "" && ud.InfURL == "" && ud.IttURL == "" {
		return nil, fmt.Errorf("%s: invalid args", op)
	}

	return &ud, nil
}

func HandleCfgPath(args []string) (*parser.UserData, bool, error) {
	const op = "main.handleCfgPath"

	dbg := slices.Contains(args, "-d") || slices.Contains(args, "--debug")

	if len(args) >= 5 {
		ud, err := parseArgs(args)
		if err != nil {
			return nil, dbg, fmt.Errorf("%s: parse args: %w", op, err)
		}
		return ud, dbg, nil
	}

	var gData []gscan.Data
	var err error
	arg := args[0]

	if _, err := os.Stat(arg); err == nil {
		gData, err = gurlf.ScanFile(arg)
		if err != nil {
			return nil, dbg, fmt.Errorf("%s: scan file: %w", op, err)
		}
	} else if os.IsNotExist(err) {
		argBytes := unsafe.Slice(unsafe.StringData(arg), len(arg))
		gData, err = gurlf.Scan(argBytes)
		if err != nil {
			return nil, dbg, fmt.Errorf("%s: scan string: %w", op, err)
		}
	}

	ud, err := parser.Parse(gData)
	if err != nil {
		return nil, dbg, fmt.Errorf("%s: parse: %w", op, err)
	}
	return ud, dbg, nil
}

func InitLog(dbg bool) *zap.Logger {
	cfg := zap.NewDevelopmentConfig()
	cfg.Encoding = "console"
	cfg.EncoderConfig.TimeKey = ""
	cfg.DisableStacktrace = true
	cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	cfg.EncoderConfig.ConsoleSeparator = " | "
	cfg.Level.SetLevel(zap.ErrorLevel)

	if dbg {
		cfg.Level.SetLevel(zap.DebugLevel)
	}
	log, _ := cfg.Build()

	return log
}

// TrimSpaceBytes trims spaces in slice by pointer
func TrimSpaceBytes(b *[]byte) {
	tempB := *b

	start := 0
	end := len(tempB) - 1
	for start < end && IsSpace(tempB[start]) {
		start++
	}
	for end > start && IsSpace(tempB[end]) {
		end--
	}

	*b = tempB[start : end+1]
}

// IsSpace is a helper function to check if a byte is a space.
func IsSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\x00'
}
