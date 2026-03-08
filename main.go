package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"unicode"
)

const redOpen = "\033[31m"
const redClose = "\033[0m"

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "%sUsage: './trl <image/text/file> <ui/cli> <command_for_call_ai> <data>'%s\n",
			redOpen, redClose)
		return
	}
	mode := os.Args[1]
	call := os.Args[2]
	data := os.Args[3]
	toLan := "английский"

	if mode == "file" {
		d, err := os.ReadFile(data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%sRead file error.\nPath:%s\nErr:%s%s",
				redOpen, data, err.Error(), redClose)
			return
		}
		data = string(d)
	}
	if !isRussian(data) {
		toLan = "русский"
	}

	promt := fmt.Sprintf("Ты — профессиональный переводчик. Твоя задача: перевести следующий текст на %s. Выводи ТОЛЬКО перевод, без лишних слов, без вступлений, без объяснений. Текст для перевода: {%s}", toLan, data)

	command := fmt.Sprintf("%s \"%s\"", call, promt)
	cmd := exec.Command("bash", "-c", command)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%Create pipe error.\nCommand:%s\nErr:%s%s",
			redOpen, command, err.Error(), redClose)
		return
	}
	cmd.Start()

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		fmt.Print(scanner.Text())
	}

	cmd.Wait()
	fmt.Println()
}

func isRussian(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}
