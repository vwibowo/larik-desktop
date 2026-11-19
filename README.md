# Larik Desktop

Larik Desktop is a native macOS workspace built with [MyGo](https://mygo.run). Each new tab and split starts with a chooser for a Ghostty terminal, a native browser, or a database pane. Terminal shells stay alive in a local session process when the window closes, and the app restores the pane layout when it opens again.

## Build and install

Requires macOS 13 or later and Go 1.27.3 or later.

```sh
go build -o larik-desktop ./cmd
./larik-desktop desktop --install
open ~/Applications/larik-desktop.app
```

The installer copies the executable and `libghostty-vt.dylib` into `~/Applications/larik-desktop.app`, then signs the app ad hoc. It may download the native library on the first install. Repeating the command updates the app. The installed app runs without the build directory or a network connection.

`./larik-desktop desktop --end-sessions` stops all background shells. Closing a terminal pane ends its shell; closing the window leaves shells running when Keep Sessions is enabled.

Browser panes use macOS WKWebView with an address/search field, back, forward, reload, and stop. Site cookies and storage persist in a separate Larik Desktop browser session on macOS 14 and later; on macOS 13 they use the app's persistent default WebView store. A user-opened link requesting a new window opens another browser pane; script popups are blocked. Browser pages do not receive the app's Go bridge. Their last URLs restore with the workspace.

Use the star in a browser pane to save or remove its current page. The book button opens a searchable bookmark list where saved pages can be opened in that pane or removed. Bookmarks are stored separately from session layouts and remain when Keep Sessions is off.

Downloads from browser panes ask where to save the file. The pane shows completion or failure, and a completed download can be revealed in Finder.

Database panes provide read-only SQLite, PostgreSQL, MySQL/MariaDB, and SQL Server access. Create a connection in the pane, browse schema objects and rows, run bounded read-only SQL, inspect results, and export loaded rows as CSV. Connection profiles are stored only for this app in its data directory; direct passwords use a separate macOS Keychain service. Closing a database pane cancels its active work.

## Workspace controls

| Action | Shortcut |
| --- | --- |
| New tab with pane chooser | ⌘T |
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
| Browser address / reload | ⌘L / ⌘R |
| Browser back / forward | ⌘[ / ⌘] |

Drag tabs to reorder them, drag a pane header onto another to swap them, and drag or double-click a divider to adjust its size. The command palette also searches tabs and panes. The View menu offers light, dark, and system appearance.

Find searches the focused terminal's screen and scrollback or the focused browser page. Return or ⌘G moves to the next match, ⇧⌘G moves to the previous match, and Escape closes Find.

Settings opens in its own native window. It controls appearance, blur, background color and opacity, terminal font and input behavior, the shell and starting folder for new tabs, notifications, and whether closing the terminal window keeps its sessions. Pane backgrounds can use Larik's surface, follow the active Ghostty terminal theme, or use a separate custom color; pane changes apply to open tabs immediately. The background is solid when Native Blur is off. Appearance also lets you search Ghostty themes and choose separate light and dark terminal palettes; theme changes apply to open panes immediately. An updated app preserves shells still owned by an older server; a custom shell preference takes effect after those shells end and the server restarts.

Sessions are local to this Mac. The host chip shows the local session process; it does not connect to remote hosts.

## Check

```sh
go test ./...
go vet ./...
```

The macOS integration test starts a real shell and `vi` in an isolated session server, replaces its temporary app bundle, and verifies that the same shell and full-screen view survive reconnection.

With a logged-in macOS desktop session, run `go test -tags larik_e2e ./internal/desktop -run '^TestMixedPaneWindowRestore$' -v` for the native mixed-pane check. It opens a temporary window with a terminal, local browser page, SQLite database, and empty chooser; closes and reopens it to verify their layout and state; then checks that disabling Keep Sessions clears the shells and layout. Its app data, web page, database, and session server are isolated and removed afterward.

The database suite also has opt-in integration checks. Use disposable databases with a read-only account and a `sample_items(id, label)` table containing `(1, 'one')` and `(2, 'two')`; fixture SQL for MySQL/MariaDB and SQL Server is in `internal/database/testdata`. Set `LARIK_TEST_POSTGRES_URL`, `LARIK_TEST_MYSQL_DSN`, or `LARIK_TEST_MARIADB_DSN` for those engines. SQL Server uses `LARIK_TEST_SQLSERVER_HOST`, `LARIK_TEST_SQLSERVER_PORT`, `LARIK_TEST_SQLSERVER_DATABASE`, `LARIK_TEST_SQLSERVER_USER`, and `LARIK_TEST_SQLSERVER_PASSWORD`; set `LARIK_TEST_SQLSERVER_TRUST_CERT=true` only for a disposable server with a self-signed certificate. Run `go test ./internal/database -run TestLive -v` after setting the relevant variables. Set `LARIK_TEST_SYSTEM_KEYCHAIN=1` on macOS to check a uniquely named test password round trip through Keychain; the test removes its entry afterward. A regular test run skips these checks.
