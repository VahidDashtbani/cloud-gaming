package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	tokenFile := filepath.Join(exeDir, "token.txt")
	agentPath := filepath.Join(exeDir, "windows-agent.exe")

	if _, err := os.Stat(agentPath); err != nil {
		fmt.Println("خطا: فایل windows-agent.exe در کنار این برنامه پیدا نشد.")
		fmt.Println("اینتر بزنید تا پنجره بسته شود...")
		bufio.NewReader(os.Stdin).ReadBytes('\n')
		return
	}

	var token string
	if len(os.Args) > 1 {
		token = strings.TrimSpace(os.Args[1])
	}
	if token == "" {
		if b, err := os.ReadFile(tokenFile); err == nil {
			token = strings.TrimSpace(string(b))
		}
	}

	fmt.Println("==========================================")
	fmt.Println("   Cloud Gaming Windows Connect")
	fmt.Println("==========================================")
	if token != "" {
		fmt.Printf("توکن ذخیره‌شده یافت شد. برای استفاده از همین توکن فقط Enter بزنید\nیا توکن جدید cg1_... را Paste کنید:\n> ")
	} else {
		fmt.Print("توکن cg1_... را Paste کنید و Enter بزنید:\n> ")
	}

	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	input := strings.TrimSpace(line)
	if input != "" {
		token = input
		_ = os.WriteFile(tokenFile, []byte(token+"\n"), 0600)
	}

	if token == "" {
		fmt.Println("خطا: توکن خالی است.")
		reader.ReadBytes('\n')
		return
	}

	fmt.Println("در حال اتصال به سرور...")
	cmd := exec.Command(agentPath, "-token", token)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	_ = cmd.Run()

	fmt.Println("اتصال خاتمه یافت. برای بستن اینتر بزنید...")
	reader.ReadBytes('\n')
}
