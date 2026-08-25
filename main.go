package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"taskii/internal/export"
	"taskii/internal/ics"
	"taskii/internal/model"
	"taskii/internal/obsidian"
	"taskii/internal/para"
	"taskii/internal/ui"
)

func main() {
	// The ics subcommand is dispatched before flag parsing so it can define
	// its own flags.
	if len(os.Args) > 1 && os.Args[1] == "ics" {
		if err := runICS(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}

	mock := flag.Bool("mock", false, "run with generated sample data instead of loading/saving real data")
	simple := flag.Bool("simple", false, "run a single-pane view: greeting beside one combined list of tasks, overdue items and notes")
	vault := flag.String("vault", "", "path to the Obsidian vault to index (default: the vault currently open in Obsidian)")
	dataDir := flag.String("data-dir", "", "where taskii stores its own JSON (default: XDG data directory)")
	flag.Parse()

	if *dataDir != "" {
		model.SetDataDir(*dataDir)
	}

	settings, _ := model.LoadSettings()
	opts := ui.Options{
		Mock:     *mock,
		Simple:   *simple,
		Vault:    resolveVault(*vault, settings),
		Location: settings.Location(),
	}

	p := tea.NewProgram(ui.NewApp(opts), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// resolveVault prefers the flag, then the saved setting, then whichever vault
// Obsidian has open. An empty result is not fatal: the vault views report that
// no vault is configured instead of the app refusing to start.
func resolveVault(flagValue string, s model.Settings) string {
	if flagValue != "" {
		return flagValue
	}
	if s.VaultPath != "" {
		return s.VaultPath
	}
	detected, err := obsidian.DetectVault()
	if err != nil {
		return ""
	}
	return detected
}

// runICS regenerates the calendar export and exits.
//
// This path deliberately never touches the Obsidian CLI. It is meant to be
// driven by a timer, and the CLI launches the desktop app when it is not
// already running — which on a locked or headless session would pop a window
// open behind the user's back. Everything here is plain file I/O.
func runICS(args []string) error {
	fs := flag.NewFlagSet("ics", flag.ExitOnError)
	vault := fs.String("vault", "", "path to the Obsidian vault to read")
	out := fs.String("out", "", "path to write the .ics file (default: <vault>/Calendar/taskii.ics)")
	tz := fs.String("tz", "", "IANA timezone for date boundaries (default: the configured or system zone)")
	dataDir := fs.String("data-dir", "", "where taskii stores its own JSON")
	quiet := fs.Bool("quiet", false, "print nothing when the calendar is already up to date")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dataDir != "" {
		model.SetDataDir(*dataDir)
	}

	settings, _ := model.LoadSettings()
	loc := settings.Location()
	if *tz != "" {
		parsed, err := time.LoadLocation(*tz)
		if err != nil {
			return fmt.Errorf("unknown timezone %q: %w", *tz, err)
		}
		loc = parsed
	}

	vaultPath := resolveVault(*vault, settings)
	if vaultPath == "" {
		return fmt.Errorf("no vault: pass -vault, or set one in taskii's settings")
	}

	idx, err := para.Scan(vaultPath, loc)
	if err != nil {
		return fmt.Errorf("scanning %s: %w", vaultPath, err)
	}
	tasks, err := model.Load()
	if err != nil {
		return fmt.Errorf("loading tasks: %w", err)
	}
	events, err := model.LoadEvents()
	if err != nil {
		return fmt.Errorf("loading events: %w", err)
	}

	target := ICSPath(*out, settings, vaultPath)
	cal := export.Calendar(filepath.Base(vaultPath), idx, tasks, events, loc)
	wrote, err := ics.WriteIfChanged(target, cal.Render())
	if err != nil {
		return fmt.Errorf("writing %s: %w", target, err)
	}
	if wrote {
		fmt.Printf("wrote %d events to %s\n", len(cal.Events), target)
	} else if !*quiet {
		fmt.Printf("%s already up to date (%d events)\n", target, len(cal.Events))
	}
	return nil
}

// ICSPath resolves where the calendar is written.
func ICSPath(flagValue string, s model.Settings, vaultPath string) string {
	if flagValue != "" {
		return flagValue
	}
	if s.ICSOutput != "" {
		return s.ICSOutput
	}
	return filepath.Join(vaultPath, "Calendar", "taskii.ics")
}
