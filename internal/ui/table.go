package ui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Column of a Table.
type Column struct {
	Title string
	// MaxWidth caps the column width; zero means no cap.
	MaxWidth int
}

// Row of a Table. Cells may contain ANSI styles. ID identifies the row across
// updates so that the cursor stays on it.
type Row struct {
	ID    string
	Cells []string
}

var tableKeys = struct {
	Up, Down, PageUp, PageDown, Top, Bottom, Left, Right key.Binding
}{
	Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	PageUp:   key.NewBinding(key.WithKeys("pgup", "ctrl+b"), key.WithHelp("pgup", "page up")),
	PageDown: key.NewBinding(key.WithKeys("pgdown", "ctrl+f"), key.WithHelp("pgdn", "page down")),
	Top:      key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("g", "top")),
	Bottom:   key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("G", "bottom")),
	Left:     key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "previous column")),
	Right:    key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "next column")),
}

// Table is a scrollable, filterable table with a row cursor and, optionally,
// a column cursor (cell selection).
type Table struct {
	cols     []Column
	rows     []Row
	visible  []int // indexes into rows matching the filter
	filter   string
	cursor   int  // index into visible
	col      int  // selected column, in cell mode
	cellMode bool // left/right move a column cursor
	offset   int  // first visible row on screen
	width    int
	height   int
}

// EnableCellCursor turns on cell selection: left/right move between columns
// and the selected cell, rather than the whole row, is highlighted.
func (t *Table) EnableCellCursor() { t.cellMode = true }

// SelectedCol returns the column under the cursor (always 0 when the cell
// cursor is disabled).
func (t *Table) SelectedCol() int { return t.col }

// NewTable returns an empty table with the given columns.
func NewTable(cols ...Column) *Table {
	return &Table{cols: cols}
}

// SetColumns replaces the columns.
func (t *Table) SetColumns(cols ...Column) {
	t.cols = cols
	t.clamp()
}

// SetRows replaces the rows, keeping the cursor on the same row ID if it is
// still present.
func (t *Table) SetRows(rows []Row) {
	selected, hadSelection := t.Selected()
	t.rows = rows
	t.refilter()
	if hadSelection {
		t.selectID(selected.ID)
	}
}

// Rows returns all rows, ignoring the filter.
func (t *Table) Rows() []Row { return t.rows }

// Len returns the number of rows matching the filter.
func (t *Table) Len() int { return len(t.visible) }

// SetFilter keeps only rows containing filter (case-insensitive).
func (t *Table) SetFilter(filter string) {
	selected, hadSelection := t.Selected()
	t.filter = filter
	t.refilter()
	if hadSelection {
		t.selectID(selected.ID)
	}
}

// Filter returns the current filter.
func (t *Table) Filter() string { return t.filter }

// Selected returns the row under the cursor.
func (t *Table) Selected() (Row, bool) {
	if t.cursor < 0 || t.cursor >= len(t.visible) {
		return Row{}, false
	}
	return t.rows[t.visible[t.cursor]], true
}

// SetSize sets the size of the table, header included.
func (t *Table) SetSize(width, height int) {
	t.width, t.height = width, height
	t.clamp()
}

// Update handles navigation keys.
func (t *Table) Update(msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	page := max(t.bodyHeight()-1, 1)
	switch {
	case key.Matches(k, tableKeys.Up):
		t.cursor--
	case key.Matches(k, tableKeys.Down):
		t.cursor++
	case key.Matches(k, tableKeys.PageUp):
		t.cursor -= page
	case key.Matches(k, tableKeys.PageDown):
		t.cursor += page
	case key.Matches(k, tableKeys.Top):
		t.cursor = 0
	case key.Matches(k, tableKeys.Bottom):
		t.cursor = len(t.visible) - 1
	case t.cellMode && key.Matches(k, tableKeys.Left):
		t.col--
	case t.cellMode && key.Matches(k, tableKeys.Right):
		t.col++
	}
	t.clamp()
	return nil
}

// KeyMap returns the navigation bindings.
func (t *Table) KeyMap() []key.Binding {
	keys := []key.Binding{tableKeys.Up, tableKeys.Down}
	if t.cellMode {
		keys = append(keys, tableKeys.Left, tableKeys.Right)
	}
	return append(keys, tableKeys.PageUp, tableKeys.PageDown, tableKeys.Top, tableKeys.Bottom)
}

// View renders the header and the visible rows.
func (t *Table) View() string {
	if t.width <= 0 || t.height <= 0 {
		return ""
	}
	widths := t.columnWidths()
	var b strings.Builder

	titles := make([]string, len(t.cols))
	for i, c := range t.cols {
		titles[i] = strings.ToUpper(c.Title)
	}
	b.WriteString(styleColHeader.Render(t.renderLine(titles, widths, -1)))

	end := min(t.offset+t.bodyHeight(), len(t.visible))
	for i := t.offset; i < end; i++ {
		b.WriteByte('\n')
		row := t.rows[t.visible[i]]
		switch {
		case i == t.cursor && t.cellMode:
			b.WriteString(t.renderLine(row.Cells, widths, t.col))
		case i == t.cursor:
			plain := make([]string, len(row.Cells))
			for j, c := range row.Cells {
				plain[j] = ansi.Strip(c)
			}
			b.WriteString(styleSelected.Render(t.renderLine(plain, widths, -1)))
		default:
			b.WriteString(t.renderLine(row.Cells, widths, -1))
		}
	}
	return b.String()
}

// renderLine lays out cells in their columns. The cell at index sel (if any)
// is highlighted.
func (t *Table) renderLine(cells []string, widths []int, sel int) string {
	var b strings.Builder
	for i, w := range widths {
		if w <= 0 {
			continue
		}
		var cell string
		if i < len(cells) {
			cell = cells[i]
		}
		cell = ansi.Truncate(cell, w, "…")
		padded := " " + cell + strings.Repeat(" ", w-ansi.StringWidth(cell)) + " "
		if i == sel {
			padded = styleSelected.Render(ansi.Strip(padded))
		}
		b.WriteString(padded)
	}
	line := b.String()
	if pad := t.width - ansi.StringWidth(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	return ansi.Truncate(line, t.width, "")
}

// columnWidths fits each column to its content. When the table is too wide,
// the widest columns are shrunk first.
func (t *Table) columnWidths() []int {
	widths := make([]int, len(t.cols))
	for i, c := range t.cols {
		widths[i] = ansi.StringWidth(c.Title)
	}
	for _, idx := range t.visible {
		for i, cell := range t.rows[idx].Cells {
			if i < len(widths) {
				widths[i] = max(widths[i], ansi.StringWidth(cell))
			}
		}
	}
	for i, c := range t.cols {
		if c.MaxWidth > 0 {
			widths[i] = min(widths[i], c.MaxWidth)
		}
	}
	avail := t.width - 2*len(widths) // one space of padding on each side
	for total(widths) > avail && avail > 0 {
		widest := 0
		for i := range widths {
			if widths[i] > widths[widest] {
				widest = i
			}
		}
		if widths[widest] <= 4 {
			break
		}
		widths[widest]--
	}
	return widths
}

func total(ws []int) int {
	n := 0
	for _, w := range ws {
		n += w
	}
	return n
}

func (t *Table) bodyHeight() int { return max(t.height-1, 0) }

func (t *Table) refilter() {
	t.visible = t.visible[:0]
	needle := strings.ToLower(t.filter)
	for i, row := range t.rows {
		if needle == "" || strings.Contains(strings.ToLower(ansi.Strip(strings.Join(row.Cells, " "))), needle) {
			t.visible = append(t.visible, i)
		}
	}
	t.clamp()
}

func (t *Table) selectID(id string) {
	for i, idx := range t.visible {
		if t.rows[idx].ID == id {
			t.cursor = i
			t.clamp()
			return
		}
	}
}

func (t *Table) clamp() {
	t.cursor = min(max(t.cursor, 0), max(len(t.visible)-1, 0))
	t.col = min(max(t.col, 0), max(len(t.cols)-1, 0))
	body := t.bodyHeight()
	if t.cursor < t.offset {
		t.offset = t.cursor
	}
	if body > 0 && t.cursor >= t.offset+body {
		t.offset = t.cursor - body + 1
	}
	t.offset = min(t.offset, max(len(t.visible)-body, 0))
	t.offset = max(t.offset, 0)
}
