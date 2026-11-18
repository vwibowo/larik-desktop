package desktop

import (
	"encoding/json"
	"testing"

	"github.com/egoist/mygo/ui"
	"larik/internal/session"
)

func TestSplitLayoutRoundTrip(t *testing.T) {
	left := &Pane{SID: "left"}
	right := &Pane{SID: "right"}
	root := &Node{Vertical: false, Ratio: 0.35}
	root.A = &Node{Parent: root, Pane: left}
	root.B = &Node{Parent: root, Pane: right}
	left.Node, right.Node = root.A, root.B
	tab := &Tab{Root: root, Focus: right}
	left.Tab, right.Tab = tab, tab
	a := &App{tabs: []*Tab{tab}, prefs: defaultSettings()}
	data, err := json.Marshal(a.snapshot())
	if err != nil {
		t.Fatal(err)
	}
	var restored savedLayout
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	got := restored.Tabs[0]
	if got.Focus != "right" || got.Root.Ratio != 0.35 || got.Root.A.SID != "left" || got.Root.B.SID != "right" {
		t.Fatalf("split layout lost a pane or focus: %+v", got)
	}
}

func TestSwapPanes(t *testing.T) {
	first := &Pane{SID: "first"}
	second := &Pane{SID: "second"}
	root := &Node{Ratio: 0.5}
	root.A = &Node{Parent: root, Pane: first}
	root.B = &Node{Parent: root, Pane: second}
	first.Node, second.Node = root.A, root.B
	tab := &Tab{Root: root, Focus: second}
	first.Tab, second.Tab = tab, tab
	a := &App{tabs: []*Tab{tab}}
	a.swapPanes(first, second)
	if root.A.Pane != second || root.B.Pane != first || first.Node != root.B || second.Node != root.A || tab.Focus != first {
		t.Fatal("pane swap did not preserve tree and focus")
	}
}

func TestWorkspaceRenders(t *testing.T) {
	pane := &Pane{ID: 3, Info: session.Info{Shell: "zsh", Dir: "/tmp", Idle: true}}
	root := &Node{ID: 2, Pane: pane}
	tab := &Tab{ID: 1, Root: root, Focus: pane}
	pane.Tab, pane.Node = tab, root
	a := &App{tabs: []*Tab{tab}, prefs: defaultSettings()}
	tester := ui.NewTester(a.view, 1000, 620)
	if got := tester.Image(); got.Bounds().Dx() != 1000 || got.Bounds().Dy() != 620 {
		t.Fatalf("unexpected image size: %v", got.Bounds())
	} else if alpha := got.RGBAAt(600, 8).A; alpha == 255 {
		t.Fatalf("chrome background is opaque: alpha=%d", alpha)
	}
	tester.SetDark(true)
	if alpha := tester.Image().RGBAAt(600, 8).A; alpha == 255 {
		t.Fatalf("dark chrome background is opaque: alpha=%d", alpha)
	}
	a.prefs.NativeBlur = false
	if alpha := ui.NewTester(a.view, 1000, 620).Image().RGBAAt(600, 8).A; alpha != 255 {
		t.Fatalf("solid chrome alpha = %d", alpha)
	}
	for _, want := range []string{"zsh", "/tmp", "This Mac"} {
		if !tester.HasText(want) {
			t.Errorf("workspace did not render %q", want)
		}
	}
	if err := tester.Click("Command palette"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Split right") || !tester.HasText("Restart shell") {
		t.Fatal("command palette omitted workspace actions")
	}
}
