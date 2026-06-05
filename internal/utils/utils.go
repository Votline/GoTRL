// Package utils utils.go contains helper functions
package utils

import (
	"fmt"
	"os"
	"slices"
	"strings"

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
		if arg == "-ui" || arg == "--ui" {
			ud.Mode = parser.UImode
			continue
		}

		key, val, found := strings.Cut(arg, "=")
		if !found {
			continue
		}

		switch key {
		case "-trl", "--translationURL":
			ud.TrlURL = val
		case "-stt", "--speechToTextURL":
			ud.SttURL = val
		case "-tts", "--textToSpeechURL":
			ud.TtsURL = val
		case "-inf", "--inflectionURL":
			ud.InfURL = val
		case "-itt", "--imageToTextURL":
			ud.IttURL = val
		}
	}

	if ud.Mode != parser.UImode && ud.TrlURL == "" && ud.SttURL == "" && ud.TtsURL == "" && ud.InfURL == "" && ud.IttURL == "" {
		return nil, fmt.Errorf("%s: invalid args (no URLs provided)", op)
	}

	return &ud, nil
}

func HandleCfgPath(args []string) (*parser.UserData, bool, error) {
	const op = "main.handleCfgPath"

	dbg := slices.Contains(args, "-d") || slices.Contains(args, "--debug")
	uiMode := slices.Contains(args, "-ui") || slices.Contains(args, "--ui")

	var ud *parser.UserData
	var err error

	if len(args) >= 5 {
		ud, err = parseArgs(args)
	} else if len(args) > 0 {
		arg := args[0]
		var gData []gscan.Data

		if _, errStat := os.Stat(arg); errStat == nil {
			gData, err = gurlf.ScanFile(arg)
		} else {
			gData, err = gurlf.Scan([]byte(arg))
		}

		if err == nil {
			ud, err = parser.Parse(gData)
		}
	}

	if err != nil {
		return nil, dbg, fmt.Errorf("%s: %w", op, err)
	}

	if uiMode && ud != nil {
		ud.Mode = parser.UImode
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
