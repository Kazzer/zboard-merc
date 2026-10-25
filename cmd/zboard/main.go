package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Kazzer/zboard-merc/internal/browser"
	"github.com/Kazzer/zboard-merc/internal/version"
)

func configDir() string {
	if d := os.Getenv("ZBOARD_CONFIG"); d != "" {
		return d
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "zboard")
	}
	if d := os.Getenv("APPDATA"); d != "" {
		return filepath.Join(d, "zboard")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "zboard")
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: zboard status | mods | mod [NAME] | flag NAME {on|off} | reload | ui | version")
		os.Exit(2)
	}
	if os.Args[1] == "version" {
		fmt.Println("zboard", version.String)
		return
	}
	dir := configDir()
	token, err := os.ReadFile(filepath.Join(dir, "control.token"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "no control token; is zengine running?", err)
		os.Exit(1)
	}
	addr := "127.0.0.1:47319"
	if b, err := os.ReadFile(filepath.Join(dir, "config.yaml")); err == nil {
		var c struct {
			Control string `yaml:"control"`
		}
		if yaml.Unmarshal(b, &c) == nil && c.Control != "" {
			addr = c.Control
		}
	}
	method, path, form := "GET", "/status", url.Values{}
	switch os.Args[1] {
	case "status", "mods":
	case "ui":
		link := "http://" + addr + "/ui?token=" + strings.TrimSpace(string(token))
		fmt.Println(link)
		if err := browser.Open(link); err != nil {
			fmt.Fprintln(os.Stderr, "could not open a browser:", err)
		}
		return
	case "mod":
		method, path = "POST", "/mod"
		if len(os.Args) > 2 {
			form.Set("name", os.Args[2])
		}
	case "flag":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "flag needs a name and on|off")
			os.Exit(2)
		}
		method, path = "POST", "/flag"
		form.Set("name", os.Args[2])
		form.Set("on", map[bool]string{true: "1", false: "0"}[strings.EqualFold(os.Args[3], "on")])
	case "reload":
		method, path = "POST", "/reload"
	default:
		fmt.Fprintln(os.Stderr, "unknown command", os.Args[1])
		os.Exit(2)
	}
	req, _ := http.NewRequest(method, "http://"+addr+path, strings.NewReader(form.Encode()))
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "zengine not reachable:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		fmt.Fprintf(os.Stderr, "%s: %s", resp.Status, body)
		os.Exit(1)
	}
	var st struct {
		Active, Pinned, Process, Version, Driver string
		Mods                                     []string
		Flags                                    map[string]bool
	}
	if json.Unmarshal(body, &st) != nil {
		os.Stdout.Write(body)
		return
	}
	if os.Args[1] == "mods" {
		for _, m := range st.Mods {
			marker := "  "
			if m == st.Active {
				marker = "* "
			}
			fmt.Println(marker + m)
		}
		return
	}
	fmt.Printf("agent: zengine %s\n", st.Version)
	if st.Driver != "" {
		fmt.Printf("driver: %s\n", st.Driver)
	}
	fmt.Printf("active: %s\n", orNone(st.Active))
	if st.Pinned != "" {
		fmt.Printf("pinned: %s\n", st.Pinned)
	}
	fmt.Printf("process: %s\n", orNone(st.Process))
	for name, on := range st.Flags {
		fmt.Printf("flag %s: %v\n", name, on)
	}
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}
