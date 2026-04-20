package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// These tests exercise the Update/View logic directly — they don't spin up a
// real tea.Program, which would need a TTY.

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestTextModelEmptyRejected(t *testing.T) {
	m := textModel{prompt: "x"}
	updated, _ := m.Update(keyMsg("enter"))
	tm := updated.(textModel)
	if tm.done {
		t.Error("empty enter should not complete")
	}
	if tm.err == "" {
		t.Error("expected error message")
	}
}

func TestTextModelCancel(t *testing.T) {
	m := textModel{}
	updated, _ := m.Update(keyMsg("esc"))
	if !updated.(textModel).cancelled {
		t.Error("esc should cancel")
	}
}

func TestMultiModelToggleAndEnter(t *testing.T) {
	m := multiModel{items: []string{"a", "b", "c"}, selected: map[int]bool{}}
	// toggle index 0
	next, _ := m.Update(keyMsg("space"))
	m = next.(multiModel)
	if !m.selected[0] {
		t.Error("space should select index 0")
	}
	// move down and toggle index 1
	next, _ = m.Update(keyMsg("down"))
	m = next.(multiModel)
	next, _ = m.Update(keyMsg("space"))
	m = next.(multiModel)
	if !m.selected[1] {
		t.Error("index 1 should be selected")
	}
	// enter should quit with no error
	next, cmd := m.Update(keyMsg("enter"))
	m = next.(multiModel)
	if m.err != "" {
		t.Errorf("unexpected err: %q", m.err)
	}
	if cmd == nil {
		t.Error("enter with selection should return Quit cmd")
	}
}

func TestMultiModelEmptyEnterRejected(t *testing.T) {
	m := multiModel{items: []string{"a"}, selected: map[int]bool{}}
	next, _ := m.Update(keyMsg("enter"))
	m = next.(multiModel)
	if m.err == "" {
		t.Error("empty selection should produce error")
	}
}

func TestMultiModelToggleAll(t *testing.T) {
	m := multiModel{items: []string{"a", "b"}, selected: map[int]bool{}}
	next, _ := m.Update(keyMsg("a"))
	m = next.(multiModel)
	if !m.selected[0] || !m.selected[1] {
		t.Error("'a' should select all")
	}
	next, _ = m.Update(keyMsg("a"))
	m = next.(multiModel)
	if m.selected[0] || m.selected[1] {
		t.Error("second 'a' should unselect all")
	}
}

func TestSingleModelNav(t *testing.T) {
	m := singleModel{items: []string{"a", "b", "c"}}
	next, _ := m.Update(keyMsg("down"))
	m = next.(singleModel)
	if m.cursor != 1 {
		t.Errorf("cursor = %d", m.cursor)
	}
	next, _ = m.Update(keyMsg("up"))
	m = next.(singleModel)
	if m.cursor != 0 {
		t.Errorf("cursor = %d", m.cursor)
	}
	// cursor can't go below 0
	next, _ = m.Update(keyMsg("up"))
	m = next.(singleModel)
	if m.cursor != 0 {
		t.Errorf("cursor = %d", m.cursor)
	}
}

func TestViewRenders(t *testing.T) {
	// Smoke test: View() must not panic on initial state.
	_ = multiModel{items: []string{"a"}, selected: map[int]bool{}}.View()
	_ = singleModel{items: []string{"a"}}.View()
	_ = textModel{prompt: "p"}.View()
}
