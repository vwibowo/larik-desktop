package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"larik/internal/control"
	"larik/internal/desktop"
)

func runControl(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: larik-desktop control <launch|list|open|choose|focus|close|terminal|browser|database> [flags]")
	}
	if args[0] == "launch" {
		if len(args) != 1 {
			return errors.New("launch accepts no flags")
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		return exec.Command("/usr/bin/open", filepath.Join(home, "Applications", "larik-desktop.app")).Run()
	}
	op, rest := args[0], args[1:]
	if op == "terminal" || op == "browser" || op == "database" {
		if len(rest) == 0 {
			return fmt.Errorf("%s requires an operation", op)
		}
		op, rest = op+"."+rest[0], rest[1:]
	}
	valid := map[string]bool{"list": true, "open": true, "choose": true, "focus": true, "close": true,
		"terminal.send": true, "terminal.read": true,
		"browser.navigate": true, "browser.back": true, "browser.forward": true, "browser.reload": true,
		"browser.snapshot": true, "browser.click": true, "browser.type": true, "browser.eval": true, "browser.screenshot": true,
		"database.profiles": true, "database.schema": true, "database.rows": true, "database.query": true}
	if !valid[op] {
		return fmt.Errorf("unknown control operation %q", op)
	}
	fs := flag.NewFlagSet(op, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var req control.Request
	req.Op = op
	fs.IntVar(&req.Pane, "pane", 0, "exact pane ID")
	fs.IntVar(&req.Tab, "tab", 0, "exact tab ID")
	fs.StringVar(&req.Kind, "kind", "", "empty, terminal, browser, or database")
	fs.StringVar(&req.Split, "split", "", "right or down")
	fs.StringVar(&req.Dir, "dir", "", "starting directory")
	fs.StringVar(&req.URL, "url", "", "browser URL or search")
	fs.StringVar(&req.Profile, "profile", "", "enabled database profile")
	fs.StringVar(&req.Schema, "schema", "", "database schema")
	fs.StringVar(&req.Table, "table", "", "database table")
	fs.StringVar(&req.SQL, "sql", "", "read-only SQL")
	fs.StringVar(&req.Ref, "ref", "", "element ref from snapshot")
	fs.StringVar(&req.Text, "text", "", "terminal input, browser text, or JavaScript")
	fs.IntVar(&req.Lines, "lines", 0, "last terminal lines")
	fs.IntVar(&req.Offset, "offset", 0, "database row offset")
	fs.IntVar(&req.Limit, "limit", 0, "database row limit")
	fs.BoolVar(&req.Enter, "enter", false, "press Return after input")
	var out string
	fs.StringVar(&out, "out", "", "screenshot PNG output path")
	var jsonFlag bool
	fs.BoolVar(&jsonFlag, "json", false, "JSON output (default)")
	if len(rest) == 1 && (rest[0] == "-h" || rest[0] == "--help") {
		fs.SetOutput(os.Stdout)
		fs.PrintDefaults()
		return nil
	}
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		return errors.New("too many positional arguments")
	}
	if fs.NArg() == 1 {
		value := fs.Arg(0)
		switch op {
		case "terminal.send", "browser.type", "browser.eval":
			if req.Text == "" {
				req.Text = value
			} else {
				return errors.New("text supplied twice")
			}
		case "browser.navigate":
			if req.URL == "" {
				req.URL = value
			} else {
				return errors.New("URL supplied twice")
			}
		case "database.query":
			if req.SQL == "" {
				req.SQL = value
			} else {
				return errors.New("SQL supplied twice")
			}
		default:
			return errors.New("unexpected positional argument")
		}
	}
	if err := validateControl(req, out); err != nil {
		return err
	}
	agent := control.DetectAgent()
	req.Agent, req.Path = agent.Name, agent.Path
	res, err := control.Call(req)
	if errors.Is(err, control.ErrWindowClosed) {
		res, err = desktop.ClosedControl(req)
	}
	if err != nil {
		return err
	}
	if op == "browser.screenshot" {
		if err := os.WriteFile(out, res.PNG, 0o600); err != nil {
			return err
		}
		res.PNG = nil
		res.Value = map[string]string{"path": out}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(res)
}

func validateControl(req control.Request, out string) error {
	if strings.HasPrefix(req.Op, "terminal.") || strings.HasPrefix(req.Op, "browser.") || req.Op == "choose" || req.Op == "focus" || req.Op == "close" {
		if req.Pane <= 0 {
			return errors.New("--pane must be an exact positive pane ID from control list")
		}
	}
	if req.Op == "open" && req.Split != "" && req.Pane <= 0 {
		return errors.New("--pane is required with --split")
	}
	if req.Op == "choose" && req.Kind == "" {
		return errors.New("--kind is required")
	}
	if req.Op == "browser.screenshot" && out == "" {
		return errors.New("--out is required for screenshot")
	}
	if strings.HasPrefix(req.Op, "database.") && req.Op != "database.profiles" && req.Profile == "" {
		return errors.New("--profile is required")
	}
	if req.Lines < 0 || req.Offset < 0 || req.Limit < 0 {
		return errors.New("line, offset, and limit values must be nonnegative")
	}
	return nil
}
