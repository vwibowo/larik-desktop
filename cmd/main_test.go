package main

import "testing"

func TestParseCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want command
		bad  bool
	}{
		{"app launch", nil, openDesktop, false},
		{"desktop", []string{"desktop"}, openDesktop, false},
		{"install", []string{"desktop", "--install"}, installDesktop, false},
		{"end sessions", []string{"desktop", "--end-sessions"}, endSessions, false},
		{"conflicting flags", []string{"desktop", "--install", "--end-sessions"}, 0, true},
		{"unknown command", []string{"other"}, 0, true},
		{"unknown flag", []string{"desktop", "--foreground"}, 0, true},
		{"extra argument", []string{"desktop", "extra"}, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCommand(tt.args)
			if (err != nil) != tt.bad || !tt.bad && got != tt.want {
				t.Fatalf("parseCommand(%q) = %v, %v; want %v, error=%v", tt.args, got, err, tt.want, tt.bad)
			}
		})
	}
}
