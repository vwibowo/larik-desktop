package main

import (
	"testing"

	"larik/internal/control"
)

func TestControlArgumentValidation(t *testing.T) {
	for _, tc := range []struct {
		req control.Request
		out string
		bad bool
	}{
		{control.Request{Op: "terminal.read", Pane: 0}, "", true},
		{control.Request{Op: "terminal.read", Pane: 4}, "", false},
		{control.Request{Op: "browser.screenshot", Pane: 4}, "", true},
		{control.Request{Op: "browser.screenshot", Pane: 4}, "/tmp/page.png", false},
		{control.Request{Op: "database.query", SQL: "SELECT 1"}, "", true},
		{control.Request{Op: "database.query", Profile: "local", SQL: "SELECT 1"}, "", false},
		{control.Request{Op: "open", Split: "right"}, "", true},
		{control.Request{Op: "open", Split: "right", Pane: 3}, "", false},
	} {
		if err := validateControl(tc.req, tc.out); (err != nil) != tc.bad {
			t.Errorf("%+v: err=%v, bad=%v", tc.req, err, tc.bad)
		}
	}
}
