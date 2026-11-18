//go:build !darwin

package desktop

import "github.com/egoist/mygo"

func browserDataSession() *mygo.Session { return mygo.NewSession("larik-desktop-browser") }
