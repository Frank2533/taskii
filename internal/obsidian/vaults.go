package obsidian

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
)

// Vault is one vault registered with the desktop app.
type Vault struct {
	ID   string
	Path string
	Open bool
	TS   int64
}

// Name is the vault's folder name, which is what the app displays.
func (v Vault) Name() string { return filepath.Base(v.Path) }

// configPath locates the desktop app's vault registry.
func configPath() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", "obsidian", "obsidian.json"), nil
	case "windows":
		if dir := os.Getenv("APPDATA"); dir != "" {
			return filepath.Join(dir, "obsidian", "obsidian.json"), nil
		}
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "obsidian", "obsidian.json"), nil
}

// Vaults lists registered vaults, most recently opened first.
func Vaults() ([]Vault, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Vaults map[string]struct {
			Path string `json:"path"`
			TS   int64  `json:"ts"`
			Open bool   `json:"open"`
		} `json:"vaults"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	out := make([]Vault, 0, len(doc.Vaults))
	for id, v := range doc.Vaults {
		out = append(out, Vault{ID: id, Path: v.Path, Open: v.Open, TS: v.TS})
	}
	sort.Slice(out, func(i, j int) bool {
		// The vault currently open is the one the user means.
		if out[i].Open != out[j].Open {
			return out[i].Open
		}
		return out[i].TS > out[j].TS
	})
	return out, nil
}

// DetectVault picks the vault to work with: the open one, else the most
// recently used.
func DetectVault() (string, error) {
	vs, err := Vaults()
	if err != nil {
		return "", err
	}
	for _, v := range vs {
		if _, err := os.Stat(v.Path); err == nil {
			return v.Path, nil
		}
	}
	return "", os.ErrNotExist
}
