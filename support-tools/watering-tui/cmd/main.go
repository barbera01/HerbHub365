// Simple CLI client for the HerbHub watering relay API.
//
// Usage:
//
//	watering status
//	watering on <channel>
//	watering off <channel>
//	watering pulse <channel> <seconds>
//	watering combo <ch1,ch2,...> <seconds>
//	watering cycle <totalSeconds> <onSeconds>
//	watering all on|off|reset
//
// Target host defaults to http://hh-02:8181, override with -host, WATERING_HOST env var,
// or config.json "default.host".
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/barbera01/HerbHub365/wateringtui/internal/config"
	"github.com/barbera01/HerbHub365/wateringtui/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	// 1. Load config
	cfg, _ := config.LoadConfig("config.json")

	// 2. Resolve host: CLI flag > env var > config > hardcoded default
	host := cfg.Default.Host
	if h := os.Getenv("WATERING_HOST"); h != "" {
		host = h
	}
	flag.StringVar(&host, "host", host, "API base URL")
	flag.Usage = usage
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		if err := runTUI(host, &cfg); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	var err error

	switch args[0] {
	case "status":
		err = call(client, "GET", host+"/status", nil)
	case "on", "off":
		err = withArgs(args, 2, func() error {
			return call(client, "POST", fmt.Sprintf("%s/relay/%s/%s", host, url.PathEscape(args[1]), args[0]), nil)
		})
	case "pulse":
		err = withArgs(args, 3, func() error {
			return call(client, "POST", fmt.Sprintf("%s/relay/%s/pulse", host, url.PathEscape(args[1])),
				url.Values{"seconds": {args[2]}})
		})
	case "combo":
		err = withArgs(args, 3, func() error {
			return call(client, "POST", host+"/pulsecombo",
				url.Values{"channels": {args[1]}, "seconds": {args[2]}})
		})
	case "cycle":
		err = withArgs(args, 3, func() error {
			for _, a := range args[1:3] {
				if _, e := strconv.ParseFloat(a, 64); e != nil {
					return fmt.Errorf("%q is not a number", a)
				}
			}
			return call(client, "POST", host+"/cycle",
				url.Values{"total": {args[1]}, "on": {args[2]}})
		})
	case "all":
		err = withArgs(args, 2, func() error {
			return call(client, "POST", fmt.Sprintf("%s/all/%s", host, url.PathEscape(args[1])), nil)
		})
	default:
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runTUI(host string, cfg *config.RootConfig) error {
	_, err := tea.NewProgram(tui.NewModel(host, cfg)).Run()
	return err
}

func withArgs(args []string, n int, fn func() error) error {
	if len(args) != n {
		usage()
		os.Exit(2)
	}
	return fn()
}

func call(client *http.Client, method, rawURL string, query url.Values) error {
	if query != nil {
		rawURL += "?" + query.Encode()
	}
	req, err := http.NewRequest(method, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var pretty bytes.Buffer
	if json.Indent(&pretty, body, "", "  ") == nil {
		fmt.Println(pretty.String())
	} else {
		fmt.Println(string(body))
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

func usage() {
	fmt.Fprintf(os.Stderr, `usage: watering [-host URL] <command>

commands:
  status                       show relay states
  on <channel>                 switch channel on   (BASIL|CHILLI|OREGANO)
  off <channel>                switch channel off
  pulse <channel> <seconds>    pulse one channel
  combo <ch1,ch2> <seconds>    pulse multiple channels
  cycle <total> <on>           rotate all channels, <on>s each, for <total>s
  all on|off|reset             switch all / reset all

host defaults to $WATERING_HOST > -host flag > config.json > http://hh-02:8181
`+
		`press Enter in terminal for the interactive TUI\n`)
}
