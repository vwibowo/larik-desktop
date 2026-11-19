package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"larik/internal/session"
)

const targetRegistryVersion = 1

type targetRegistry struct {
	Version int                          `json:"version"`
	Targets map[string]map[string]string `json:"targets"`
}

func registryPath() string { return filepath.Join(session.Dir(), "agent-skill-targets.json") }

func loadTargets() (targetRegistry, error) {
	r := targetRegistry{Version: targetRegistryVersion, Targets: make(map[string]map[string]string)}
	data, err := os.ReadFile(registryPath())
	if errors.Is(err, os.ErrNotExist) {
		return r, nil
	}
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return r, fmt.Errorf("custom skill targets: %w", err)
	}
	if r.Version != targetRegistryVersion {
		return r, fmt.Errorf("unsupported custom skill target version %d", r.Version)
	}
	if r.Targets == nil {
		r.Targets = make(map[string]map[string]string)
	}
	return r, nil
}

func saveTargets(r targetRegistry) error {
	path := registryPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".agent-skill-targets-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// CustomTargets lists the skills roots registered by either the CLI or Settings.
func CustomTargets() ([]string, error) {
	r, err := loadTargets()
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(r.Targets))
	for path := range r.Targets {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

func resolveTarget(path string, requireExists bool) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errors.New("skills directory must be an absolute path")
	}
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && !requireExists {
			return canonicalMissingPath(path)
		}
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", path)
	}
	return filepath.EvalSymlinks(path)
}

func canonicalMissingPath(path string) (string, error) {
	parent := filepath.Dir(path)
	if parent == path {
		return path, nil
	}
	resolved, err := filepath.EvalSymlinks(parent)
	if errors.Is(err, os.ErrNotExist) {
		resolved, err = canonicalMissingPath(parent)
	}
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, filepath.Base(path)), nil
}

func renderedSkills(bin string) (map[string][]byte, map[string]string, error) {
	if !filepath.IsAbs(bin) {
		return nil, nil, errors.New("installed executable path must be absolute")
	}
	bodies := make(map[string][]byte, len(names))
	hashes := make(map[string]string, len(names))
	for _, name := range names {
		body, err := files.ReadFile("skills/" + name + "/SKILL.md")
		if err != nil {
			return nil, nil, err
		}
		body = []byte(strings.ReplaceAll(string(body), "{{BIN}}", shellQuote(bin)))
		bodies[name] = body
		hashes[name] = digest(body)
	}
	return bodies, hashes, nil
}

func digest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

type skillBackup struct {
	name    string
	body    []byte
	existed bool
	created bool
}

func preflightTarget(root string, recorded map[string]string, registered bool) ([]skillBackup, error) {
	backups := make([]skillBackup, 0, len(names))
	for _, name := range names {
		dir := filepath.Join(root, name)
		info, err := os.Lstat(dir)
		if err == nil {
			if !info.IsDir() || !registered {
				return nil, fmt.Errorf("%s belongs to another skill or is not managed by Larik Desktop", dir)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		backup := skillBackup{name: name, created: errors.Is(err, os.ErrNotExist)}
		path := filepath.Join(dir, "SKILL.md")
		info, err = os.Lstat(path)
		if err == nil {
			if !info.Mode().IsRegular() || recorded[name] == "" {
				return nil, fmt.Errorf("%s is not a managed skill file", path)
			}
			backup.body, err = os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			if digest(backup.body) != recorded[name] {
				return nil, fmt.Errorf("%s was edited; leaving it unchanged", path)
			}
			backup.existed = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		backups = append(backups, backup)
	}
	return backups, nil
}

func writeSkill(path string, body []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".SKILL-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func restoreTarget(root string, backups []skillBackup) {
	for _, backup := range backups {
		dir := filepath.Join(root, backup.name)
		path := filepath.Join(dir, "SKILL.md")
		if backup.existed {
			_ = os.MkdirAll(dir, 0o755)
			_ = writeSkill(path, backup.body)
		} else {
			_ = os.Remove(path)
		}
		if backup.created {
			_ = os.Remove(dir)
		}
	}
}

// InstallAt copies the three portable skills into an existing skills root.
// The registry records exact file hashes so edited skills cannot be replaced.
func InstallAt(root, bin string) (err error) {
	root, err = resolveTarget(root, true)
	if err != nil {
		return err
	}
	r, err := loadTargets()
	if err != nil {
		return err
	}
	previous, registered := r.Targets[root]
	backups, err := preflightTarget(root, previous, registered)
	if err != nil {
		return err
	}
	bodies, hashes, err := renderedSkills(bin)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			restoreTarget(root, backups)
		}
	}()
	for _, backup := range backups {
		dir := filepath.Join(root, backup.name)
		if backup.created {
			if err = os.Mkdir(dir, 0o755); err != nil {
				return err
			}
		}
		if err = writeSkill(filepath.Join(dir, "SKILL.md"), bodies[backup.name]); err != nil {
			return err
		}
	}
	r.Targets[root] = hashes
	err = saveTargets(r)
	return err
}

func StatusAt(root string) (bool, error) {
	root, err := resolveTarget(root, false)
	if err != nil {
		return false, err
	}
	r, err := loadTargets()
	if err != nil {
		return false, err
	}
	recorded, ok := r.Targets[root]
	if !ok {
		return false, nil
	}
	for _, name := range names {
		dir := filepath.Join(root, name)
		info, err := os.Lstat(dir)
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if !info.IsDir() {
			return false, fmt.Errorf("%s is not a managed skill directory", dir)
		}
		path := filepath.Join(dir, "SKILL.md")
		info, err = os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if !info.Mode().IsRegular() {
			return false, fmt.Errorf("%s is not a managed skill file", path)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return false, err
		}
		if digest(body) != recorded[name] {
			return false, fmt.Errorf("%s was edited", path)
		}
	}
	return true, nil
}

func RemoveAt(root string) (err error) {
	root, err = resolveTarget(root, false)
	if err != nil {
		return err
	}
	r, err := loadTargets()
	if err != nil {
		return err
	}
	recorded, ok := r.Targets[root]
	if !ok {
		return fmt.Errorf("%s is not a Larik Desktop custom skill target", root)
	}
	if _, err = os.Stat(root); errors.Is(err, os.ErrNotExist) {
		delete(r.Targets, root)
		return saveTargets(r)
	} else if err != nil {
		return err
	}
	backups, err := preflightTarget(root, recorded, true)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			restoreTarget(root, backups)
		}
	}()
	for _, backup := range backups {
		if backup.existed {
			if err = os.Remove(filepath.Join(root, backup.name, "SKILL.md")); err != nil {
				return err
			}
			_ = os.Remove(filepath.Join(root, backup.name))
		}
	}
	delete(r.Targets, root)
	err = saveTargets(r)
	return err
}

// ForgetAt removes tracking only. It never changes the chosen skills root.
func ForgetAt(root string) error {
	var err error
	root, err = resolveTarget(root, false)
	if err != nil {
		return err
	}
	r, err := loadTargets()
	if err != nil {
		return err
	}
	delete(r.Targets, root)
	return saveTargets(r)
}
