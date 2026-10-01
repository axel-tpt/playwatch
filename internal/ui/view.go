// Package ui implements the k9s-style terminal UI: a stack of views, a command
// bar (:), a generic filter (/) and a status bar. Resources are declared
// elsewhere (internal/resource) and registered on the App.
package ui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// View is a screen of the navigation stack. Views are mutable and only ever
// updated from the Bubble Tea event loop.
type View interface {
	Init() tea.Cmd
	Update(msg tea.Msg) tea.Cmd
	// View renders the view's body; it must fit the size given by SetSize.
	View() string
	// Title is shown in the frame and the breadcrumbs.
	Title() string
	// KeyMap lists the view's own bindings, for the help screen.
	KeyMap() []key.Binding
	SetSize(width, height int)
}

// Filterable views accept the generic / filter.
type Filterable interface {
	SetFilter(filter string)
	Filter() string
}

// Interrupter views handle ctrl+c themselves (e.g. a run cancels its
// process). It returns false when ctrl+c should quit the application.
type Interrupter interface {
	Interrupt() (tea.Cmd, bool)
}

// InputCapturer views take every key while they capture input (forms):
// :, / and ? are then typed instead of opening the command bar, the filter or
// the help. ctrl+c keeps its global meaning.
type InputCapturer interface {
	CapturesInput() bool
}

// Targeted messages are delivered to their target view, even when it is not
// on top of the stack (e.g. the result of an asynchronous load). They are
// dropped if the view is gone.
type Targeted interface {
	Target() View
}

// Navigation and status messages, emitted by views through the helpers below.
type (
	pushMsg    struct{ view View }
	replaceMsg struct{ view View }
	popMsg     struct{}
	switchMsg  struct{ resource string }
	statusMsg  struct {
		text string
		err  bool
	}
	confirmMsg struct {
		prompt string
		onYes  tea.Cmd
	}
	keepAliveMsg struct{ view View }
	refreshMsg   struct{}
)

// Push stacks a view on top of the current one.
func Push(v View) tea.Cmd { return func() tea.Msg { return pushMsg{v} } }

// Replace swaps the current view for v (e.g. a form for what it submitted).
func Replace(v View) tea.Cmd { return func() tea.Msg { return replaceMsg{v} } }

// Pop goes back to the previous view.
func Pop() tea.Cmd { return func() tea.Msg { return popMsg{} } }

// SwitchTo replaces the stack with the list view of a resource.
func SwitchTo(resource string) tea.Cmd { return func() tea.Msg { return switchMsg{resource} } }

// Info shows a message in the status bar.
func Info(text string) tea.Cmd { return func() tea.Msg { return statusMsg{text: text} } }

// Error shows an error in the status bar.
func Error(err error) tea.Cmd {
	return func() tea.Msg { return statusMsg{text: err.Error(), err: true} }
}

// Confirm asks a yes/no question and runs onYes if the user accepts.
func Confirm(prompt string, onYes tea.Cmd) tea.Cmd {
	return func() tea.Msg { return confirmMsg{prompt: prompt, onYes: onYes} }
}

// Refresh reloads the current view if it is a resource list.
func Refresh() tea.Cmd { return func() tea.Msg { return refreshMsg{} } }

// KeepAlive makes the app deliver Targeted messages to v even after it has
// been popped, for the rest of the session (used by runs, which keep going in
// the background).
func KeepAlive(v View) tea.Cmd { return func() tea.Msg { return keepAliveMsg{v} } }
