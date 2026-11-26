package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"deepClean/internal/config"
	"deepClean/internal/core"
)

type tgUpdate struct {
	UpdateID int `json:"update_id"`
	Message  struct {
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		Text string `json:"text"`
	} `json:"message"`
}

type tgResp struct {
	OK     bool       `json:"ok"`
	Result []tgUpdate `json:"result"`
}

var (
	cfg      config.Config
	lastMu   = make(chan struct{}, 1)
	lastPath string
)

func main() {
	cfg, _ = config.Load()
	if cfg.TelegramToken == "" || cfg.TelegramChat == "" {
		fmt.Println("telegram config not set (use deepclean --set-telegram-token/--set-telegram-chat)")
		os.Exit(1)
	}
	chatID, _ := strconv.ParseInt(cfg.TelegramChat, 10, 64)
	lastPath = filepath.Join(mustHome(), ".config", "deepclean", "last-scan.json")

	offset := 0
	fmt.Println("deepclean bot running (local). Press Ctrl+C to stop.")
	registerCommands(cfg.TelegramToken)
	for {
		updates, err := getUpdates(cfg.TelegramToken, offset)
		if err != nil {
			fmt.Println("update error:", err)
			time.Sleep(3 * time.Second)
			continue
		}
		for _, u := range updates {
			offset = u.UpdateID + 1
			if u.Message.Chat.ID != chatID {
				continue
			}
			handleMessage(u.Message.Text)
		}
		time.Sleep(1 * time.Second)
	}
}

func handleMessage(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	fields := strings.Fields(text)
	cmd := strings.ToLower(fields[0])
	_, _ = fmt.Printf("[bot] %s %v\n", cmd, fields[1:])
	args := fields[1:]

	switch cmd {
	case "/start", "/help":
		send("Commands:\n/scan [flags]\n/latest\n/top [n]\n/set path|days|min|risk VALUE\n/show\n/help")
	case "/show":
		cfg, _ := config.Load()
		send(fmt.Sprintf("path=%s\ndays=%d\nmin=%g\nrisk=%s", cfg.Path, cfg.Days, cfg.MinSizeMB, cfg.Risk))
	case "/set":
		if len(args) < 2 {
			send("usage: /set path|days|min|risk VALUE")
			return
		}
		cfg, _ := config.Load()
		key := strings.ToLower(args[0])
		val := strings.Join(args[1:], " ")
		switch key {
		case "path":
			cfg.Path = val
		case "days":
			if v, err := strconv.Atoi(val); err == nil && v > 0 {
				cfg.Days = v
			} else {
				send("invalid days")
				return
			}
		case "min":
			if v, err := strconv.ParseFloat(val, 64); err == nil && v > 0 {
				cfg.MinSizeMB = v
			} else {
				send("invalid min")
				return
			}
		case "risk":
			cfg.Risk = val
		default:
			send("unknown key")
			return
		}
		if err := config.Save(cfg); err != nil {
			send("save error: " + err.Error())
			return
		}
		send("saved")
	case "/latest":
		send(loadLatestSummary())
	case "/top":
		n := 10
		if len(args) > 0 {
			if v, err := strconv.Atoi(args[0]); err == nil && v > 0 {
				n = v
			}
		}
		send(loadTopSummary(n))
	case "/scan":
		go runScan(args)
	default:
		send("Unknown command. Use /help")
	}
}

func runScan(args []string) {
	select {
	case lastMu <- struct{}{}:
	default:
		send("Scan already running")
		return
	}
	defer func() { <-lastMu }()

	send("Scanning... (may take a while)")
	cfg, _ := config.Load()
	defaultPath := "/"
	if cfg.Path != "" {
		defaultPath = cfg.Path
	}
	defaultDays := 90
	if cfg.Days > 0 {
		defaultDays = cfg.Days
	}
	defaultMinSize := 1.0
	if cfg.MinSizeMB > 0 {
		defaultMinSize = cfg.MinSizeMB
	}
	defaultRisk := "all"
	if cfg.Risk != "" {
		defaultRisk = cfg.Risk
	}

	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	path := fs.String("path", defaultPath, "root")
	days := fs.Int("days", defaultDays, "days")
	minSize := fs.Float64("min-size-mb", defaultMinSize, "min size")
	risk := fs.String("risk", defaultRisk, "risk")
	topN := fs.Int("top", 200, "top")
	if err := fs.Parse(args); err != nil {
		send("parse error: " + err.Error())
		return
	}
	minSizeB := int64(*minSize * 1e6)
	older := time.Duration(*days) * 24 * time.Hour
	res, err := core.Scan(*path, minSizeB, older, *topN)
	if err != nil {
		send("scan error: " + err.Error())
		return
	}
	// risk filter
	if *risk != "" && *risk != "all" {
		filtered := res.Files[:0]
		var total int64
		for _, f := range res.Files {
			if f.Risk == *risk {
				filtered = append(filtered, f)
				total += f.Size
			}
		}
		res.Files = filtered
		res.TotalSize = total
	}
	// save
	_ = os.MkdirAll(filepath.Dir(lastPath), 0700)
	b, _ := json.MarshalIndent(res, "", "  ")
	_ = os.WriteFile(lastPath, b, 0644)
	send(fmt.Sprintf("<b>Found %d files, %s total</b>\n%s", len(res.Files), core.FormatSize(res.TotalSize), topSummary(res.Files, 10)))
}

func loadLatestSummary() string {
	res, err := loadReport()
	if err != nil {
		return "no report yet (run /scan)"
	}
	return fmt.Sprintf("<b>Last scan: %d files, %s total</b>\n%s", len(res.Files), core.FormatSize(res.TotalSize), topSummary(res.Files, 10))
}

func loadTopSummary(n int) string {
	res, err := loadReport()
	if err != nil {
		return "no report yet (run /scan)"
	}
	return topSummary(res.Files, n)
}

func topSummary(files []core.FileInfo, n int) string {
	if n <= 0 {
		n = 10
	}
	if len(files) < n {
		n = len(files)
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		f := files[i]
		last := f.Atime
		if last.IsZero() {
			last = f.ModTime
		}
		_, _ = fmt.Fprintf(&b, "<b>%s</b> %s %s <code>%s</code>\n\n", core.FormatSize(f.Size), f.Risk, last.Format("2006-01-02"), html.EscapeString(f.Path))
	}
	if len(files) == 0 {
		b.WriteString("(none)\n")
	}
	return b.String()
}

func loadReport() (core.ScanResult, error) {
	b, err := os.ReadFile(lastPath)
	if err != nil {
		return core.ScanResult{}, err
	}
	var res core.ScanResult
	if err := json.Unmarshal(b, &res); err != nil {
		return core.ScanResult{}, err
	}
	return res, nil
}

func send(text string) {
	url := "https://api.telegram.org/bot" + cfg.TelegramToken + "/sendMessage"
	body := map[string]string{"chat_id": cfg.TelegramChat, "text": text, "parse_mode": "HTML"}
	b, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		fmt.Println("send error:", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		buf := make([]byte, 256)
		n, _ := resp.Body.Read(buf)
		_, _ = fmt.Printf("send error: %d %s\n", resp.StatusCode, string(buf[:n]))
	}
}

func registerCommands(token string) {
	type cmd struct {
		Command     string `json:"command"`
		Description string `json:"description"`
	}
	cmds := []cmd{
		{"start", "start bot"},
		{"help", "show help"},
		{"scan", "run scan (flags optional)"},
		{"latest", "show last scan summary"},
		{"top", "show top files"},
	}
	b, _ := json.Marshal(cmds)
	url := "https://api.telegram.org/bot" + token + "/setMyCommands"
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		fmt.Println("register commands error:", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		buf := make([]byte, 256)
		n, _ := resp.Body.Read(buf)
		_, _ = fmt.Printf("register commands error: %d %s\n", resp.StatusCode, string(buf[:n]))
	}
}

func getUpdates(token string, offset int) ([]tgUpdate, error) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=25", token, offset)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var r tgResp
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	if !r.OK {
		return nil, fmt.Errorf("telegram api error")
	}
	return r.Result, nil
}

func mustHome() string {
	h, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("home dir error:", err)
		os.Exit(1)
	}
	return h
}
