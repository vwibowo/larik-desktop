//go:build !darwin

package desktop

import "github.com/egoist/mygo"

func focusNativeUI(*mygo.Window) bool { return false }
