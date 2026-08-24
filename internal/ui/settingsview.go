package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"taskii/internal/model"
)

// settingsField identifies one editable setting.
type settingsField int

const (
	fieldTimezone settingsField = iota
	fieldVaultPath
	fieldICSOutput
	fieldICSInterval
	fieldWorklogPush
	fieldTheme
	fieldLayout

	settingsFieldCount = 7
)

// settingsState is the overlay's state. Theme and layout were previously only
// cyclable blind with t and L; they are listed here too so every persisted
// preference has one visible home.
type settingsState struct {
	open    bool
	sel     int
	editing bool
	input   textinput.Model
	err     string
}

func (f settingsField) label() string {
	switch f {
	case fieldTimezone:
		return "Timezone"
	case fieldVaultPath:
		return "Vault path"
	case fieldICSOutput:
		return "Calendar file"
	case fieldICSInterval:
		return "Export every"
	case fieldWorklogPush:
		return "Send worklog to Jira"
	case fieldTheme:
		return "Theme"
	default:
		return "Layout"
	}
}

func (f settingsField) help() string {
	switch f {
	case fieldTimezone:
		return "IANA name, e.g. Europe/Berlin. Blank uses this machine's zone."
	case fieldVaultPath:
		return "Blank detects the vault Obsidian currently has open."
	case fieldICSOutput:
		return "Blank writes <vault>/Calendar/taskii.ics."
	case fieldICSInterval:
		return "A duration like 15m. Set 0 to stop exporting in the background."
	case fieldWorklogPush:
		return "Off keeps tracked time local; on also sends it to the Jira issue."
	default:
		return "Enter to cycle."
	}
}

// settingsValue renders a field's current value.
func (a App) settingsValue(f settingsField) string {
	s := a.settings()
	switch f {
	case fieldTimezone:
		if s.Timezone == "" {
			return fmt.Sprintf("(system: %s)", time.Local)
		}
		return s.Timezone
	case fieldVaultPath:
		if s.VaultPath == "" {
			if a.vaultPath != "" {
				return "(detected: " + a.vaultPath + ")"
			}
			return "(none detected)"
		}
		return s.VaultPath
	case fieldICSOutput:
		if a.icsOut == "" {
			return "(no vault)"
		}
		return a.icsOut
	case fieldICSInterval:
		if a.icsEvery <= 0 {
			return "off"
		}
		return a.icsEvery.String()
	case fieldWorklogPush:
		if s.WorklogPushToJira {
			return "on"
		}
		return "off"
	case fieldTheme:
		return currentTheme().Name
	default:
		return a.layout.String()
	}
}

// settings rebuilds the persisted settings from live state.
func (a App) settings() model.Settings {
	return model.Settings{
		Theme:             currentTheme().Name,
		Layout:            a.layout.String(),
		Timezone:          a.tzSetting,
		VaultPath:         a.vaultSetting,
		ICSOutput:         a.icsSetting,
		ICSInterval:       a.icsIntervalSetting,
		WorklogPushToJira: a.worklogPush,
	}
}

func (a App) renderSettings() string {
	width := a.width - 8
	if width > 76 {
		width = 76
	}
	if width < 30 {
		width = 30
	}

	muted := lipgloss.NewStyle().Foreground(colorMuted).Background(colorPaneBg)
	text := lipgloss.NewStyle().Foreground(colorText).Background(colorPaneBg)

	var rows []string
	for i := 0; i < settingsFieldCount; i++ {
		f := settingsField(i)
		value := a.settingsValue(f)
		style := text
		prefix := "  "
		if i == a.settingsUI.sel {
			prefix = "> "
			style = text.Foreground(colorAccent).Bold(true)
		}
		if a.settingsUI.editing && i == a.settingsUI.sel {
			value = a.settingsUI.input.View()
		}
		label := fmt.Sprintf("%-22s", f.label())
		rows = append(rows, style.Render(fitToWidth(prefix+label+value, width-4)))
	}

	rows = append(rows, "")
	rows = append(rows, muted.Render(fitToWidth(settingsField(a.settingsUI.sel).help(), width-4)))
	if a.settingsUI.err != "" {
		rows = append(rows, lipgloss.NewStyle().Foreground(colorDanger).Background(colorPaneBg).
			Render(fitToWidth(a.settingsUI.err, width-4)))
	}

	return renderPane("Settings", strings.Join(rows, "\n"), true, width, len(rows)+2)
}

// updateSettings handles keys while the overlay is open.
func (a App) updateSettings(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.settingsUI.editing {
		switch msg.String() {
		case "esc":
			a.settingsUI.editing = false
			a.settingsUI.err = ""
			return a, nil
		case "enter":
			return a.commitSetting(a.settingsUI.input.Value())
		}
		var cmd tea.Cmd
		a.settingsUI.input, cmd = a.settingsUI.input.Update(msg)
		return a, cmd
	}

	switch msg.String() {
	case "esc", ",", "q":
		a.settingsUI.open = false
		a.settingsUI.err = ""
		return a, nil
	case "up", "k":
		a.settingsUI.sel = clamp(a.settingsUI.sel-1, 0, settingsFieldCount-1)
		return a, nil
	case "down", "j":
		a.settingsUI.sel = clamp(a.settingsUI.sel+1, 0, settingsFieldCount-1)
		return a, nil
	case "enter":
		return a.beginEditSetting()
	}
	return a, nil
}

// beginEditSetting either cycles a choice field or opens a text input.
func (a App) beginEditSetting() (tea.Model, tea.Cmd) {
	switch settingsField(a.settingsUI.sel) {
	case fieldWorklogPush:
		a.worklogPush = !a.worklogPush
		a.saveSettings()
		return a, nil
	case fieldTheme:
		cycleTheme()
		a.saveSettings()
		return a, nil
	case fieldLayout:
		a.layout = a.layout.next()
		a.saveSettings()
		return a, nil
	}

	in := textinput.New()
	in.CharLimit = 200
	in.Width = 40
	switch settingsField(a.settingsUI.sel) {
	case fieldTimezone:
		in.SetValue(a.tzSetting)
		in.Placeholder = "Asia/Kolkata"
	case fieldVaultPath:
		in.SetValue(a.vaultSetting)
		in.Placeholder = a.vaultPath
	case fieldICSOutput:
		in.SetValue(a.icsSetting)
		in.Placeholder = a.icsOut
	case fieldICSInterval:
		in.SetValue(a.icsIntervalSetting)
		in.Placeholder = "15m"
	}
	in.Focus()
	a.settingsUI.input = in
	a.settingsUI.editing = true
	a.settingsUI.err = ""
	return a, textinput.Blink
}

// commitSetting validates and applies an edited value.
//
// Validation happens here rather than at save time so a bad timezone is
// rejected while the user is still looking at the field, instead of silently
// falling back to the system zone later.
func (a App) commitSetting(raw string) (tea.Model, tea.Cmd) {
	value := strings.TrimSpace(raw)
	var cmds []tea.Cmd

	switch settingsField(a.settingsUI.sel) {
	case fieldTimezone:
		if !model.ValidTimezone(value) {
			a.settingsUI.err = fmt.Sprintf("unknown timezone %q", value)
			return a, nil
		}
		a.tzSetting = value
		a.loc = model.Settings{Timezone: value}.Location()
		// Dates are re-derived from the new zone, so the index must be rebuilt.
		cmds = append(cmds, loadIndex(a.vaultPath, a.loc))

	case fieldVaultPath:
		a.vaultSetting = value
		if value != "" {
			a.vaultPath = value
		}
		a.icsOut = a.resolveICSPath()
		cmds = append(cmds, loadIndex(a.vaultPath, a.loc))

	case fieldICSOutput:
		a.icsSetting = value
		a.icsOut = a.resolveICSPath()

	case fieldICSInterval:
		if value != "" {
			if _, err := time.ParseDuration(value); err != nil {
				a.settingsUI.err = fmt.Sprintf("not a duration: %q (try 15m)", value)
				return a, nil
			}
		}
		a.icsIntervalSetting = value
		a.icsEvery = model.Settings{ICSInterval: value}.ICSEvery()
		if c := icsTick(a.icsEvery); c != nil {
			cmds = append(cmds, c)
		}
	}

	a.settingsUI.editing = false
	a.settingsUI.err = ""
	a.saveSettings()
	return a, tea.Batch(cmds...)
}
