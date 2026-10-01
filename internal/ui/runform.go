package ui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/axel-tpt/playwatch/internal/ansible"
)

var (
	keyFormRun    = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "run"))
	keyFormNext   = key.NewBinding(key.WithKeys("tab", "down"), key.WithHelp("tab/↓", "next field"))
	keyFormPrev   = key.NewBinding(key.WithKeys("shift+tab", "up"), key.WithHelp("shift+tab/↑", "previous field"))
	keyFormToggle = key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "toggle"))
	keyFormCancel = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel"))
)

type fieldKind int

const (
	fieldText fieldKind = iota
	fieldToggle
	fieldPassword
)

type formField struct {
	label string
	hint  string
	kind  fieldKind
	input textinput.Model // text and password fields
	on    bool            // toggles
}

// Field indexes, in display order.
const (
	fieldLimit = iota
	fieldTags
	fieldSkipTags
	fieldExtraVars
	fieldCheck
	fieldDiff
	fieldVaultPassword
	fieldVaultFile
)

// RunForm edits the options of a run before starting it. It replaces itself
// with the run view on submit.
type RunForm struct {
	ctx      context.Context
	env      RunEnv
	playbook string
	// base keeps the options the form does not edit (vault IDs, raw args).
	base   ansible.RunOptions
	fields []*formField
	focus  int
	hosts  []string // inventory names, for limit completion
	width  int
}

type formNamesMsg struct {
	form  *RunForm
	names []string
}

func (m formNamesMsg) Target() View { return m.form }

// NewRunForm returns a form to run playbook, prefilled with opts.
func NewRunForm(ctx context.Context, env RunEnv, playbook string, opts ansible.RunOptions) *RunForm {
	f := &RunForm{ctx: ctx, env: env, playbook: playbook, base: opts}
	text := func(label, hint, value string) *formField {
		in := textinput.New()
		in.Prompt = ""
		in.SetValue(value)
		return &formField{label: label, hint: hint, kind: fieldText, input: in}
	}
	f.fields = []*formField{
		fieldLimit:     text("Limit", "hosts or groups, e.g. web,!web2 (tab completes)", opts.Limit),
		fieldTags:      text("Tags", "only run tasks tagged with these", opts.Tags),
		fieldSkipTags:  text("Skip tags", "skip tasks tagged with these", opts.SkipTags),
		fieldExtraVars: text("Extra vars", "key=value …, or JSON, or @file", strings.Join(opts.ExtraVars, " ")),
		fieldCheck:     {label: "Check mode", hint: "--check: report changes without making them", kind: fieldToggle, on: opts.Check},
		fieldDiff:      {label: "Diff", hint: "--diff: show what changes in files and templates", kind: fieldToggle, on: opts.Diff},
		fieldVaultPassword: func() *formField {
			fld := text("Vault password", "kept in memory, handed to Ansible through a pipe", "")
			fld.kind = fieldPassword
			fld.input.EchoMode = textinput.EchoPassword
			return fld
		}(),
		fieldVaultFile: text("Vault pass file", "--vault-password-file", opts.VaultPasswordFile),
	}
	f.fields[fieldLimit].input.ShowSuggestions = true
	if opts.AskVaultPass {
		f.fields[fieldVaultPassword].hint = "the previous run used a typed-in password: type it again"
	}
	return f
}

func (f *RunForm) Init() tea.Cmd {
	client := f.env.Client
	return tea.Batch(f.focusField(fieldLimit), func() tea.Msg {
		inv, err := client.Inventory(f.ctx)
		if err != nil {
			return nil // completion is a convenience: ignore errors
		}
		return formNamesMsg{form: f, names: append(inv.GroupNames(), inv.HostNames()...)}
	})
}

func (f *RunForm) CapturesInput() bool { return true }

func (f *RunForm) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case formNamesMsg:
		f.hosts = msg.names
		f.updateSuggestions()
		return nil
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, keyFormCancel):
			return Pop()
		case key.Matches(msg, keyFormRun):
			return f.submit()
		case key.Matches(msg, keyFormNext):
			// tab accepts a limit completion before moving on.
			if f.focus == fieldLimit && msg.String() == "tab" && f.acceptSuggestion() {
				return nil
			}
			return f.focusField((f.focus + 1) % len(f.fields))
		case key.Matches(msg, keyFormPrev):
			return f.focusField((f.focus + len(f.fields) - 1) % len(f.fields))
		}
		fld := f.fields[f.focus]
		if fld.kind == fieldToggle {
			if key.Matches(msg, keyFormToggle) || msg.String() == "x" {
				fld.on = !fld.on
			}
			return nil
		}
		var cmd tea.Cmd
		fld.input, cmd = fld.input.Update(msg)
		if f.focus == fieldLimit {
			f.updateSuggestions()
		}
		return cmd
	}
	if fld := f.fields[f.focus]; fld.kind != fieldToggle {
		var cmd tea.Cmd
		fld.input, cmd = fld.input.Update(msg)
		return cmd
	}
	return nil
}

func (f *RunForm) focusField(i int) tea.Cmd {
	if fld := f.fields[f.focus]; fld.kind != fieldToggle {
		fld.input.Blur()
	}
	f.focus = i
	if fld := f.fields[i]; fld.kind != fieldToggle {
		return fld.input.Focus()
	}
	return nil
}

// updateSuggestions completes the last item of the comma-separated limit.
func (f *RunForm) updateSuggestions() {
	in := &f.fields[fieldLimit].input
	value := in.Value()
	prefix := ""
	if i := strings.LastIndexAny(value, ",:"); i >= 0 {
		prefix = value[:i+1]
	}
	last := strings.TrimLeft(value[len(prefix):], "!&")
	prefix = value[:len(value)-len(last)]
	suggestions := make([]string, 0, len(f.hosts))
	for _, name := range f.hosts {
		if strings.HasPrefix(name, last) {
			suggestions = append(suggestions, prefix+name)
		}
	}
	in.SetSuggestions(suggestions)
}

// acceptSuggestion completes the limit field if a suggestion is shown.
func (f *RunForm) acceptSuggestion() bool {
	in := &f.fields[fieldLimit].input
	s := in.CurrentSuggestion()
	if s == "" || s == in.Value() {
		return false
	}
	in.SetValue(s)
	in.CursorEnd()
	f.updateSuggestions()
	return true
}

// Options returns the options as currently filled in.
func (f *RunForm) Options() ansible.RunOptions {
	opts := f.base
	value := func(i int) string { return strings.TrimSpace(f.fields[i].input.Value()) }
	opts.Limit = value(fieldLimit)
	opts.Tags = value(fieldTags)
	opts.SkipTags = value(fieldSkipTags)
	opts.ExtraVars = nil
	if ev := value(fieldExtraVars); ev != "" {
		opts.ExtraVars = []string{ev}
	}
	opts.Check = f.fields[fieldCheck].on
	opts.Diff = f.fields[fieldDiff].on
	opts.VaultPasswordFile = value(fieldVaultFile)
	opts.AskVaultPass = false
	return opts
}

func (f *RunForm) submit() tea.Cmd {
	pw := f.fields[fieldVaultPassword].input
	var password []byte
	if pw.Value() != "" {
		password = []byte(pw.Value())
	}
	f.fields[fieldVaultPassword].input.SetValue("")
	return Replace(NewRunView(f.ctx, f.env, f.playbook, f.Options(), password))
}

// Command returns the ansible-playbook command line the form would run.
func (f *RunForm) Command() string {
	args := []string{"ansible-playbook"}
	for _, inv := range f.env.Client.Inventories {
		args = append(args, "-i", inv)
	}
	args = append(args, f.Options().Args()...)
	if f.fields[fieldVaultPassword].input.Value() != "" {
		args = append(args, "--vault-password-file", "<typed password>")
	}
	args = append(args, f.playbook)
	for i, a := range args {
		args[i] = shellQuote(a)
	}
	return strings.Join(args, " ")
}

func shellQuote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\n'\"\\$`!*?[]{}()<>|&;#~") {
		return s
	}
	if s == "<typed password>" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (f *RunForm) View() string {
	labelWidth := 0
	for _, fld := range f.fields {
		labelWidth = max(labelWidth, len(fld.label))
	}
	inputWidth := min(max(f.width-labelWidth-8, 10), 50)

	var b strings.Builder
	fmt.Fprintf(&b, " %s %s\n\n", Section("Run"), f.playbook)
	for i, fld := range f.fields {
		marker := "  "
		label := fmt.Sprintf("%-*s", labelWidth, fld.label)
		if i == f.focus {
			marker = styleKey.Render("› ")
			label = styleKey.Render(label)
		}
		var value string
		switch fld.kind {
		case fieldToggle:
			box := "[ ]"
			if fld.on {
				box = StatusStyle(ansible.StatusOK).Render("[x]")
			}
			value = box
		default:
			fld.input.SetWidth(inputWidth)
			value = fld.input.View()
		}
		value = fit(value, inputWidth) // styled values and long unfocused ones
		fmt.Fprintf(&b, " %s%s  %s  %s\n", marker, label, value, Muted(fld.hint))
	}
	if extra := slices.Concat(vaultIDArgs(f.base.VaultIDs), f.base.Extra); len(extra) > 0 {
		fmt.Fprintf(&b, "\n   %s %s\n", Muted("kept from the previous run:"), strings.Join(extra, " "))
	}
	command := lipgloss.NewStyle().Width(max(f.width-2, 20)).Render(Section("Command") + " " + f.Command())
	b.WriteString("\n" + indent(command, " ") + "\n")
	return b.String()
}

func indent(s, prefix string) string {
	return prefix + strings.ReplaceAll(s, "\n", "\n"+prefix)
}

func vaultIDArgs(ids []string) []string {
	var args []string
	for _, id := range ids {
		args = append(args, "--vault-id", id)
	}
	return args
}

func (f *RunForm) Title() string { return "run " + f.playbook }

func (f *RunForm) KeyMap() []key.Binding {
	return []key.Binding{keyFormRun, keyFormNext, keyFormPrev, keyFormToggle, keyFormCancel}
}

func (f *RunForm) SetSize(width, _ int) { f.width = width }
