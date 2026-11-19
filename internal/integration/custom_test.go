package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func customFixture(t *testing.T) string {
	t.Helper()
	t.Setenv("LARIK_DESKTOP_DIR", t.TempDir())
	return t.TempDir()
}

func TestCustomSkillsLifecycle(t *testing.T) {
	root := customFixture(t)
	bin := "/Applications/larik desktop.app/Contents/MacOS/larik-desktop"
	if err := InstallAt(root, bin); err != nil {
		t.Fatal(err)
	}
	if ok, err := StatusAt(root); err != nil || !ok {
		t.Fatalf("status after install: %v, %v", ok, err)
	}
	targets, err := CustomTargets()
	canonical, e := filepath.EvalSymlinks(root)
	if e != nil {
		t.Fatal(e)
	}
	if err != nil || len(targets) != 1 || targets[0] != canonical {
		t.Fatalf("tracked targets: %v, %v", targets, err)
	}
	for _, name := range Names() {
		body, err := os.ReadFile(filepath.Join(root, name, "SKILL.md"))
		if err != nil || !strings.Contains(string(body), shellQuote(bin)) {
			t.Fatalf("rendered %s: %v", name, err)
		}
	}
	updatedBin := "/Applications/new-larik-desktop.app/Contents/MacOS/larik-desktop"
	if err := InstallAt(root, updatedBin); err != nil {
		t.Fatalf("repeat install: %v", err)
	}
	if ok, err := StatusAt(root); err != nil || !ok {
		t.Fatalf("status after update: %v, %v", ok, err)
	}
	foreign := filepath.Join(root, "unrelated-skill")
	if err := os.Mkdir(foreign, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RemoveAt(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("skills root removed: %v", err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatalf("unrelated skill removed: %v", err)
	}
	for _, name := range Names() {
		if _, err := os.Stat(filepath.Join(root, name, "SKILL.md")); !os.IsNotExist(err) {
			t.Fatalf("%s retained after removal: %v", name, err)
		}
	}
	targets, err = CustomTargets()
	if err != nil || len(targets) != 0 {
		t.Fatalf("targets after removal: %v, %v", targets, err)
	}
}

func TestCustomSkillPathAndForeignConflict(t *testing.T) {
	root := customFixture(t)
	bin := "/Applications/larik-desktop.app/Contents/MacOS/larik-desktop"
	if err := InstallAt("relative/skills", bin); err == nil {
		t.Fatal("relative path accepted")
	}
	if err := InstallAt(filepath.Join(root, "missing"), bin); err == nil {
		t.Fatal("missing skills root accepted")
	}
	conflict := filepath.Join(root, names[0])
	if err := os.Mkdir(conflict, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(conflict, "SKILL.md"), []byte("foreign"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := InstallAt(root, bin); err == nil {
		t.Fatal("foreign skill overwritten")
	}
	if _, err := os.Stat(filepath.Join(root, names[1])); !os.IsNotExist(err) {
		t.Fatal("conflict caused a partial install")
	}
	if err := os.RemoveAll(conflict); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), conflict); err != nil {
		t.Fatal(err)
	}
	if err := InstallAt(root, bin); err == nil {
		t.Fatal("skill-directory symlink followed")
	}
}

func TestCustomSkillEditAndForget(t *testing.T) {
	root := customFixture(t)
	bin := "/Applications/larik-desktop.app/Contents/MacOS/larik-desktop"
	if err := InstallAt(root, bin); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, names[0], "SKILL.md")
	if err := os.WriteFile(path, []byte("user edited skill"), 0o600); err != nil {
		t.Fatal(err)
	}
	if ok, err := StatusAt(root); ok || err == nil {
		t.Fatalf("edited skill status: %v, %v", ok, err)
	}
	if err := InstallAt(root, bin); err == nil {
		t.Fatal("edited skill overwritten")
	}
	if err := RemoveAt(root); err == nil {
		t.Fatal("edited skill removed")
	}
	body, err := os.ReadFile(path)
	if err != nil || string(body) != "user edited skill" {
		t.Fatalf("edited file changed: %q, %v", body, err)
	}
	if err := ForgetAt(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("forget deleted a skill: %v", err)
	}
	targets, err := CustomTargets()
	if err != nil || len(targets) != 0 {
		t.Fatalf("forget retained target: %v, %v", targets, err)
	}
}

func TestCustomSkillRegistryFailureRollsBack(t *testing.T) {
	appDir := t.TempDir()
	t.Setenv("LARIK_DESKTOP_DIR", appDir)
	root := t.TempDir()
	if err := os.Chmod(appDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(appDir, 0o700) })
	if err := InstallAt(root, "/Applications/larik-desktop.app/Contents/MacOS/larik-desktop"); err == nil {
		t.Fatal("registry write failure ignored")
	}
	for _, name := range Names() {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("%s left after failed install: %v", name, err)
		}
	}
}

func TestCustomSkillFailedUpdateKeepsPriorFiles(t *testing.T) {
	appDir := t.TempDir()
	t.Setenv("LARIK_DESKTOP_DIR", appDir)
	root := t.TempDir()
	oldBin := "/Applications/old-larik.app/Contents/MacOS/larik-desktop"
	if err := InstallAt(root, oldBin); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, names[0], "SKILL.md")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(appDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(appDir, 0o700) })
	if err := InstallAt(root, "/Applications/new-larik.app/Contents/MacOS/larik-desktop"); err == nil {
		t.Fatal("registry write failure ignored")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("prior skill changed after failed update: %v", err)
	}
	if ok, err := StatusAt(root); err != nil || !ok {
		t.Fatalf("prior registry state lost: %v, %v", ok, err)
	}
}

func TestCustomSkillFileSymlink(t *testing.T) {
	root := customFixture(t)
	bin := "/Applications/larik-desktop.app/Contents/MacOS/larik-desktop"
	if err := InstallAt(root, bin); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, names[0], "SKILL.md")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(t.TempDir(), "foreign.md")
	if err := os.WriteFile(foreign, []byte("foreign"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, path); err != nil {
		t.Fatal(err)
	}
	if err := InstallAt(root, bin); err == nil {
		t.Fatal("skill-file symlink overwritten")
	}
	if err := RemoveAt(root); err == nil {
		t.Fatal("skill-file symlink removed")
	}
	if body, err := os.ReadFile(foreign); err != nil || string(body) != "foreign" {
		t.Fatalf("symlink target changed: %q, %v", body, err)
	}
}

func TestCustomTargetTrackingAfterFolderRemoved(t *testing.T) {
	root := customFixture(t)
	alias := filepath.Join(t.TempDir(), "skills-link")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	if err := InstallAt(alias, "/Applications/larik-desktop.app/Contents/MacOS/larik-desktop"); err != nil {
		t.Fatal(err)
	}
	if ok, err := StatusAt(alias); err != nil || !ok {
		t.Fatalf("symlinked root status: %v, %v", ok, err)
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	if ok, err := StatusAt(root); err != nil || ok {
		t.Fatalf("missing root status: %v, %v", ok, err)
	}
	if err := ForgetAt(root); err != nil {
		t.Fatal(err)
	}
	targets, err := CustomTargets()
	if err != nil || len(targets) != 0 {
		t.Fatalf("removed root still tracked: %v, %v", targets, err)
	}
}
