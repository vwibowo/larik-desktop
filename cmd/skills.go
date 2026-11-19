package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"larik/internal/integration"
)

func runSkills(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: larik-desktop skills <install|remove|status|forget> [--dir ABSOLUTE_SKILLS_ROOT]")
	}
	fs := flag.NewFlagSet("skills", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dir := fs.String("dir", "", "existing absolute skills root for another agent harness")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected skills argument")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	switch args[0] {
	case "install":
		bin := filepath.Join(home, "Applications", "larik-desktop.app", "Contents", "MacOS", "larik-desktop")
		if _, err := os.Stat(bin); err != nil {
			return fmt.Errorf("install Larik Desktop first: %w", err)
		}
		if *dir == "" {
			if err := integration.Install(home, bin); err != nil {
				return err
			}
		} else if err := integration.InstallAt(*dir, bin); err != nil {
			return err
		}
		fmt.Println("installed Larik Desktop agent skills")
	case "remove":
		if *dir == "" {
			if err := integration.Remove(home); err != nil {
				return err
			}
		} else if err := integration.RemoveAt(*dir); err != nil {
			return err
		}
		fmt.Println("removed Larik Desktop agent skills")
	case "status":
		var ok bool
		if *dir == "" {
			ok, err = integration.Status(home)
		} else {
			ok, err = integration.StatusAt(*dir)
		}
		if err != nil {
			return err
		}
		if ok {
			fmt.Println("installed")
		} else {
			fmt.Println("not installed")
		}
	case "forget":
		if *dir == "" {
			return errors.New("--dir is required for skills forget")
		}
		if err := integration.ForgetAt(*dir); err != nil {
			return err
		}
		fmt.Println("forgot Larik Desktop custom skills target")
	default:
		return fmt.Errorf("unknown skills command %q", args[0])
	}
	return nil
}
