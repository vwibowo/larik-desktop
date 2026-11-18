package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"larik/internal/desktop"
	"larik/internal/session"
)

type command int

const (
	openDesktop command = iota
	installDesktop
	endSessions
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "larik-desktop:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 && args[0] == "server" {
		if len(args) != 1 {
			return errors.New("server accepts no arguments")
		}
		return session.Serve()
	}
	cmd, err := parseCommand(args)
	if err != nil {
		return err
	}
	if cmd == installDesktop {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		app, err := desktop.Install(exe)
		if err != nil {
			return err
		}
		fmt.Println("installed", app)
		return nil
	}
	if cmd == endSessions {
		return desktop.EndSessions()
	}
	return desktop.Run()
}

func parseCommand(args []string) (command, error) {
	if len(args) == 0 {
		return openDesktop, nil
	}
	if args[0] != "desktop" {
		return 0, fmt.Errorf("unknown command %q (usage: larik-desktop desktop [--install | --end-sessions])", args[0])
	}
	fs := flag.NewFlagSet("desktop", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	install := fs.Bool("install", false, "install larik-desktop.app in ~/Applications")
	end := fs.Bool("end-sessions", false, "stop all background shell sessions")
	if err := fs.Parse(args[1:]); err != nil {
		return 0, err
	}
	if len(fs.Args()) != 0 {
		return 0, errors.New("desktop accepts no positional arguments")
	}
	if *install {
		if *end {
			return 0, errors.New("--install and --end-sessions cannot be combined")
		}
		return installDesktop, nil
	}
	if *end {
		return endSessions, nil
	}
	return openDesktop, nil
}
