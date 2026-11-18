# Larik Desktop

Larik Desktop is a native macOS terminal workspace built with [MyGo](https://mygo.run) and its Ghostty VT terminal plugin. It opens the user's login shell in a tab. Tabs and split panes stay alive in a local session process when the window closes, and the app restores their layout when it opens again.

## Build and install

Requires macOS 13 or later and Go 1.27.3 or later.

```sh
go build -o larik-desktop ./cmd
./larik-desktop desktop --install
open ~/Applications/larik-desktop.app
```

The installer copies the executable and `libghostty-vt.dylib` into `~/Applications/larik-desktop.app`, then signs the app ad hoc. It may download the native library on the first install. Repeating the command updates the app. The installed app runs without the build directory or a network connection.

`./larik-desktop desktop --end-sessions` stops all background shells. Closing a tab or pane ends only its own shells; closing the window leaves them running.

## Workspace controls

| Action | Shortcut |
| --- | --- |
| New tab | ⌘T |
| Split right / down | ⌘D / ⇧⌘D |
| Close pane / tab | ⌘W / ⇧⌘W |
| Zoom pane | ⇧⌘Return |
| Equalize panes | ⌃⌘= |
| Next / previous tab | ⇧⌘] / ⇧⌘[ |
| Focus adjacent pane | ⌥⌘Arrow |
| Move divider | ⌃⌘Arrow |
| Rename tab | ⇧⌘R |
| Command palette | ⇧⌘P |
| Clear terminal | ⌘K |
| Find in pane / next / previous match | ⌘F / ⌘G / ⇧⌘G |
| Change text size | ⌘+ / ⌘− / ⌘0 |
| Open Settings | ⌘, |

Drag tabs to reorder them, drag a pane header onto another to swap them, and drag or double-click a divider to adjust its size. The command palette also searches tabs and panes. The View menu offers light, dark, and system appearance.

Find searches the focused pane's screen and scrollback. Return or ⌘G moves to the next older match, ⇧⌘G moves toward newer output, and Escape closes Find.

Settings opens in its own native window. It controls appearance and blur, terminal font and input behavior, the shell and starting folder for new tabs, notifications, and whether closing the terminal window keeps its sessions. Appearance also lets you search Ghostty themes and choose separate light and dark terminal palettes; theme changes apply to open panes immediately. An updated app preserves shells still owned by an older server; a custom shell preference takes effect after those shells end and the server restarts.

Sessions are local to this Mac. The host chip shows the local session process; it does not connect to remote hosts.

## Check

```sh
go test ./...
go vet ./...
```
