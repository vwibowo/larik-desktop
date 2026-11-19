---
name: larik-workspace
description: Control Larik Desktop tabs and panes, or read and send text to its terminal panes when the user asks to work in Larik Desktop.
---

<!-- Managed by Larik Desktop. -->

# Larik Desktop workspace

Use `{{BIN}} control list --json` to get exact tab and pane IDs. Use exact `--pane` IDs for later commands; focus may change while an agent works. If the window is closed, `control launch` opens it. A first control request asks the user to grant this local agent access in Larik Desktop. Respect a denial.

```sh
{{BIN}} control list --json
{{BIN}} control open --kind terminal --dir .
{{BIN}} control open --kind browser --url http://localhost:3000
{{BIN}} control open --kind database
{{BIN}} control open --kind empty --split right --pane 12
{{BIN}} control choose --pane 13 --kind terminal
{{BIN}} control focus --pane 13
{{BIN}} control terminal read --pane 13 --lines 50
{{BIN}} control terminal send --pane 13 --text 'go test ./...' --enter
{{BIN}} control close --pane 13
```

Opening or sending terminal input runs commands in the user's real shell; closing a terminal pane ends its shell. Follow the agent's normal execution permissions and the user's instructions; the Larik grant does not replace them. Terminal send/read and list can reach existing shells after the window closes. Browser and database operations require the window. Use `control <operation> -h` for flags.
