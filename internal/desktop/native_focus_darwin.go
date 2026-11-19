//go:build darwin

package desktop

import (
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

var (
	contentViewSelector        = objc.RegisterName("contentView")
	subviewsSelector           = objc.RegisterName("subviews")
	countSelector              = objc.RegisterName("count")
	objectAtIndexSelector      = objc.RegisterName("objectAtIndex:")
	isKindOfClassSelector      = objc.RegisterName("isKindOfClass:")
	makeFirstResponderSelector = objc.RegisterName("makeFirstResponder:")
)

// MyGo's Element.Focus selects the UI control, but AppKit can keep a
// WKWebView as the first responder. Move the keyboard to MyGo's surface
// before the next frame focuses the Find input.
func focusNativeUI(win *mygo.Window) bool {
	if win == nil {
		return false
	}
	native := objc.ID(win.NativeHandle())
	if native == 0 {
		return false
	}
	content := native.Send(contentViewSelector)
	if content == 0 {
		return false
	}
	views := content.Send(subviewsSelector)
	surfaceClass := objc.GetClass("MyGoSurfaceView")
	if views == 0 || surfaceClass == 0 {
		return false
	}
	for i := uintptr(0); i < uintptr(views.Send(countSelector)); i++ {
		view := views.Send(objectAtIndexSelector, i)
		if objc.Send[bool](view, isKindOfClassSelector, surfaceClass) {
			return objc.Send[bool](native, makeFirstResponderSelector, view)
		}
	}
	return false
}

func nativeUIHasKeyboard(win *mygo.Window) bool {
	if win == nil || win.NativeHandle() == 0 {
		return false
	}
	native := objc.ID(win.NativeHandle())
	responder := native.Send(objc.RegisterName("firstResponder"))
	return responder != 0 && objc.Send[bool](responder, isKindOfClassSelector, objc.GetClass("MyGoSurfaceView"))
}
