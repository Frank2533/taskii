package model

import (
	"os"
	"path/filepath"
	"sync"
)

// legacyDataDir is the working-directory-relative folder older builds wrote
// to. Those builds resolved "data/tasks.json" against the process CWD, so
// launching taskii from anywhere but its source tree silently started an empty
// task list — and launching it from inside an Obsidian vault dropped taskii's
// private state into a folder that gets committed and pushed. Both are fixed by
// resolving an absolute directory once, at startup.
const legacyDataDir = "data"

var (
	dataDirMu sync.RWMutex
	dataDir   = DefaultDataDir()
)

// DefaultDataDir is the XDG data location for taskii's JSON files.
func DefaultDataDir() string {
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "taskii")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		// No home directory to anchor to; fall back to the old behaviour
		// rather than writing to an arbitrary absolute path.
		return legacyDataDir
	}
	return filepath.Join(home, ".local", "share", "taskii")
}

// SetDataDir overrides where taskii reads and writes its JSON. Empty restores
// the default. Call before loading anything.
func SetDataDir(dir string) {
	dataDirMu.Lock()
	defer dataDirMu.Unlock()
	if dir == "" {
		dataDir = DefaultDataDir()
		return
	}
	dataDir = dir
}

// DataDir is the directory currently in use.
func DataDir() string {
	dataDirMu.RLock()
	defer dataDirMu.RUnlock()
	return dataDir
}

// readData reads name from the data directory, falling back to the legacy
// CWD-relative folder so an existing install keeps its tasks across the
// upgrade. The next write lands in the new location, so the fallback is
// load-bearing exactly once.
func readData(name string) ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(DataDir(), name))
	if err == nil || !os.IsNotExist(err) {
		return b, err
	}
	legacy, legacyErr := os.ReadFile(filepath.Join(legacyDataDir, name))
	if legacyErr != nil {
		// Report the original miss: the legacy path is a fallback, not the
		// location the user configured, so its error would be misleading.
		return nil, err
	}
	return legacy, nil
}

// writeData writes name into the data directory, creating it if needed.
func writeData(name string, b []byte) error {
	dir := DataDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), b, 0o644)
}
