package integration

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed skills/*/SKILL.md
var files embed.FS

var names = []string{"larik-workspace", "larik-native-browser", "larik-database"}

const marker = "<!-- Managed by Larik Desktop. -->"

func Names() []string { return append([]string(nil), names...) }

func skillsDir(home string) string { return filepath.Join(home, ".agents", "skills") }

func Status(home string) (bool, error) {
	for _, name := range names {
		dir := filepath.Join(skillsDir(home), name)
		if info, err := os.Lstat(dir); err == nil && !info.IsDir() {
			return false, fmt.Errorf("%s is not a skill directory", dir)
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
		path := filepath.Join(skillsDir(home), name, "SKILL.md")
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if !strings.Contains(string(data), marker) {
			return false, fmt.Errorf("%s belongs to another skill", path)
		}
		link := filepath.Join(home, ".claude", "skills", name)
		target, err := os.Readlink(link)
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if target != filepath.Dir(path) {
			return false, fmt.Errorf("%s belongs to another skill", link)
		}
	}
	return true, nil
}

func Install(home, bin string) error {
	if !filepath.IsAbs(bin) {
		return errors.New("installed executable path must be absolute")
	}
	for _, name := range names {
		dir := filepath.Join(skillsDir(home), name)
		if info, err := os.Lstat(dir); err == nil && !info.IsDir() {
			return fmt.Errorf("%s is not a skill directory", dir)
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		path := filepath.Join(skillsDir(home), name, "SKILL.md")
		if data, err := os.ReadFile(path); err == nil && !strings.Contains(string(data), marker) {
			return fmt.Errorf("%s belongs to another skill", path)
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		link := filepath.Join(home, ".claude", "skills", name)
		if _, err := os.Lstat(link); err == nil {
			target, e := os.Readlink(link)
			if e != nil || target != filepath.Dir(path) {
				return fmt.Errorf("%s belongs to another skill", link)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	for _, name := range names {
		dir := filepath.Join(skillsDir(home), name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		body, err := files.ReadFile("skills/" + name + "/SKILL.md")
		if err != nil {
			return err
		}
		body = []byte(strings.ReplaceAll(string(body), "{{BIN}}", shellQuote(bin)))
		path := filepath.Join(dir, "SKILL.md")
		tmp, err := os.CreateTemp(dir, ".SKILL-*")
		if err != nil {
			return err
		}
		if _, err := tmp.Write(body); err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
			return err
		}
		if err := tmp.Close(); err != nil {
			os.Remove(tmp.Name())
			return err
		}
		if err := os.Rename(tmp.Name(), path); err != nil {
			os.Remove(tmp.Name())
			return err
		}
		link := filepath.Join(home, ".claude", "skills", name)
		if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
			return err
		}
		if _, err := os.Lstat(link); errors.Is(err, os.ErrNotExist) {
			if err := os.Symlink(dir, link); err != nil {
				return err
			}
		}
	}
	return nil
}

func Remove(home string) error {
	for _, name := range names {
		dir := filepath.Join(skillsDir(home), name)
		path := filepath.Join(dir, "SKILL.md")
		data, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err == nil && !strings.Contains(string(data), marker) {
			return fmt.Errorf("%s belongs to another skill", path)
		}
		link := filepath.Join(home, ".claude", "skills", name)
		if target, err := os.Readlink(link); err == nil && target == dir {
			if err := os.Remove(link); err != nil {
				return err
			}
		}
		if err == nil {
			if err := os.Remove(path); err != nil {
				return err
			}
			_ = os.Remove(dir)
		}
	}
	return nil
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
