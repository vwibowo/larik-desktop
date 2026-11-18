package desktop

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/egoist/mygo/plugins/terminal"
)

const appName = "larik-desktop.app"

// Install copies this executable and Ghostty's native library into a macOS
// application. The installed app does not depend on the source executable.
func Install(bin string) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("desktop --install is supported on macOS only (got %s)", runtime.GOOS)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if err := terminal.Load(); err != nil {
		return "", fmt.Errorf("load libghostty-vt: %w", err)
	}
	lib, err := terminal.LibraryPath()
	if err != nil {
		return "", fmt.Errorf("find libghostty-vt: %w", err)
	}
	app := filepath.Join(home, "Applications", appName)
	if err := installBundle(app, bin, lib, signBundle); err != nil {
		return "", err
	}
	return app, nil
}

// installBundle builds in the destination directory so the final rename stays
// on one filesystem. A failed build or signature leaves the prior app intact.
func installBundle(app, bin, lib string, sign func(string) error) error {
	parent := filepath.Dir(app)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".larik-desktop-*.app")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := os.Chmod(stage, 0o755); err != nil {
		return err
	}
	if err := writeBundle(stage, bin, lib); err != nil {
		return err
	}
	if err := sign(stage); err != nil {
		return err
	}

	backup := stage + ".old"
	if _, err := os.Lstat(app); err == nil {
		if err := os.Rename(app, backup); err != nil {
			return fmt.Errorf("move old app: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(stage, app); err != nil {
		if _, oldErr := os.Lstat(backup); oldErr == nil {
			if restoreErr := os.Rename(backup, app); restoreErr != nil {
				return fmt.Errorf("install app: %w; restore old app: %v", err, restoreErr)
			}
		}
		return fmt.Errorf("install app: %w", err)
	}
	return os.RemoveAll(backup)
}

func writeBundle(app, bin, lib string) error {
	contents := filepath.Join(app, "Contents")
	macOS := filepath.Join(contents, "MacOS")
	resources := filepath.Join(contents, "Resources")
	for _, dir := range []string{macOS, resources} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if err := copyFile(bin, filepath.Join(macOS, "larik-desktop"), 0o755); err != nil {
		return fmt.Errorf("copy executable: %w", err)
	}
	if err := copyFile(lib, filepath.Join(resources, "libghostty-vt.dylib"), 0o755); err != nil {
		return fmt.Errorf("copy libghostty-vt: %w", err)
	}
	icon, err := icns()
	if err != nil {
		return fmt.Errorf("render icon: %w", err)
	}
	if err := os.WriteFile(filepath.Join(resources, "larik.icns"), icon, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(resources, "JetBrainsMono-LICENSE.txt"), fontLicense, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(contents, "Info.plist"), []byte(infoPlist), 0o644)
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func signBundle(app string) error {
	for _, path := range []string{
		filepath.Join(app, "Contents", "Resources", "libghostty-vt.dylib"),
		app,
	} {
		out, err := exec.Command("/usr/bin/codesign", "--force", "--sign", "-", path).CombinedOutput()
		if err != nil {
			return fmt.Errorf("codesign %s: %w: %s", filepath.Base(path), err, out)
		}
	}
	return nil
}

const infoPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>
	<string>larik-desktop</string>
	<key>CFBundleDisplayName</key>
	<string>larik-desktop</string>
	<key>CFBundleIdentifier</key>
	<string>dev.larik.terminal</string>
	<key>CFBundleInfoDictionaryVersion</key>
	<string>6.0</string>
	<key>CFBundleExecutable</key>
	<string>larik-desktop</string>
	<key>CFBundleIconFile</key>
	<string>larik</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleVersion</key>
	<string>1</string>
	<key>CFBundleShortVersionString</key>
	<string>0.0.1</string>
	<key>LSMinimumSystemVersion</key>
	<string>13.0</string>
	<key>NSPrincipalClass</key>
	<string>NSApplication</string>
	<key>NSHighResolutionCapable</key>
	<true/>
	<key>NSSupportsAutomaticGraphicsSwitching</key>
	<true/>
</dict>
</plist>
`
