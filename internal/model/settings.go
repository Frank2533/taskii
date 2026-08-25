package model

import (
	"encoding/json"
	"os"
	"time"
)

const settingsFile = "settings.json"

// DefaultICSInterval is how often the background exporter rewrites the .ics
// when Settings.ICSInterval is unset.
const DefaultICSInterval = 15 * time.Minute

type Settings struct {
	Theme  string `json:"theme,omitempty"`
	Layout string `json:"layout,omitempty"`

	// Timezone is an IANA name (e.g. "Asia/Kolkata", "Europe/Berlin"). Empty
	// means "use the system zone". Every date boundary in the app and the
	// VTIMEZONE in exported calendars resolve through this, so a user in any
	// zone gets correct today/overdue math — nothing is hardcoded to the
	// machine this was first written on.
	Timezone string `json:"timezone,omitempty"`

	// VaultPath is the Obsidian vault to index. Empty means "detect".
	VaultPath string `json:"vault_path,omitempty"`

	// ICSOutput is where the calendar export is written. Empty means
	// <vault>/Calendar/taskii.ics.
	ICSOutput string `json:"ics_output,omitempty"`

	// ICSInterval is a Go duration string ("15m"). "0" disables the
	// background exporter.
	ICSInterval string `json:"ics_interval,omitempty"`

	// JiraDisabled turns off every Jira-backed action and hides them from the
	// UI. It is stored inverted so that an existing settings file, and a
	// fresh install, both start with Jira available — the zero value of a new
	// bool would otherwise silently switch it off for anyone upgrading.
	JiraDisabled bool `json:"jira_disabled,omitempty"`

	// ObsidianSync opts in to taskii writing local tasks and notes into the
	// vault. It defaults off: everything else in the app reads the vault or
	// edits notes the user pointed at, whereas this creates and moves notes
	// on its own, which should never begin without being asked for.
	ObsidianSync bool `json:"obsidian_sync,omitempty"`

	// ProjectFolder is the vault folder local task notes are written to.
	ProjectFolder string `json:"project_folder,omitempty"`

	// PushEnabled opts in to sending reminders to a phone through ntfy. Off
	// by default: it is the only thing in the app that sends anything off the
	// machine.
	PushEnabled bool `json:"push_enabled,omitempty"`

	// NtfyTopic is the channel reminders are published to. An ntfy topic is a
	// shared channel rather than an account, so on the public server anyone
	// who knows or guesses it can read every message — it should be long and
	// unguessable.
	NtfyTopic string `json:"ntfy_topic,omitempty"`

	// NtfyServer is the ntfy base URL. Empty means the public instance.
	NtfyServer string `json:"ntfy_server,omitempty"`

	// WorklogPushToJira opts in to sending tracked time to Jira. Off by
	// default: time is accrued locally and written to the note's freeform
	// work-log section, and nothing reaches Jira unless this is enabled.
	WorklogPushToJira bool `json:"worklog_push_to_jira,omitempty"`
}

// Location resolves Timezone, falling back to the system zone. A persisted
// name can stop resolving later (a container without tzdata, a zone renamed
// between releases), so this never fails — a wrong-but-working clock beats a
// startup crash.
func (s Settings) Location() *time.Location {
	if s.Timezone == "" {
		return time.Local
	}
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		return time.Local
	}
	return loc
}

// ValidTimezone reports whether name resolves, for validating input in the
// settings overlay before it is persisted.
func ValidTimezone(name string) bool {
	if name == "" {
		return true
	}
	_, err := time.LoadLocation(name)
	return err == nil
}

// ICSEvery is the export interval. A malformed or absent value falls back to
// the default; an explicit zero disables the exporter.
func (s Settings) ICSEvery() time.Duration {
	if s.ICSInterval == "" {
		return DefaultICSInterval
	}
	d, err := time.ParseDuration(s.ICSInterval)
	if err != nil {
		return DefaultICSInterval
	}
	if d < 0 {
		return 0
	}
	return d
}

// JiraEnabled reports whether Jira-backed features are available.
func (s Settings) JiraEnabled() bool { return !s.JiraDisabled }

// DefaultProjectFolder is where local task notes go when nothing is set.
const DefaultProjectFolder = "Projects"

// Projects is the folder local task notes are written to.
func (s Settings) Projects() string {
	if s.ProjectFolder == "" {
		return DefaultProjectFolder
	}
	return s.ProjectFolder
}

// ArchiveLocalTasks is where a finished local task's note is moved.
const ArchiveLocalTasks = "Archive/Local Tasks"

func LoadSettings() (Settings, error) {
	b, err := readData(settingsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return Settings{}, nil
		}
		return Settings{}, err
	}
	if len(b) == 0 {
		return Settings{}, nil
	}
	var s Settings
	if err := json.Unmarshal(b, &s); err != nil {
		return Settings{}, err
	}
	return s, nil
}

func SaveSettings(s Settings) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeData(settingsFile, b)
}
