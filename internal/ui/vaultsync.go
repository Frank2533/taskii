package ui

import (
	"path/filepath"

	"taskii/internal/model"
	"taskii/internal/vault"
)

// syncEnabled reports whether taskii may write into the vault.
//
// Writing is opt-in and needs somewhere to write to: everything else in the
// app either reads the vault or edits a note the user pointed at, whereas this
// creates and moves notes on its own.
func (a App) syncEnabled() bool {
	return a.obsidianSync && a.vaultPath != "" && !a.noPersist
}

// projectDir is where local task notes live.
func (a App) projectDir() string {
	folder := a.projectFolder
	if folder == "" {
		folder = model.DefaultProjectFolder
	}
	return filepath.Join(a.vaultPath, filepath.FromSlash(folder))
}

// archiveDir is where finished local task notes are moved.
func (a App) archiveDir() string {
	return filepath.Join(a.vaultPath, filepath.FromSlash(model.ArchiveLocalTasks))
}

// resourcesDir is where the dated notes go.
func (a App) resourcesDir() string {
	return filepath.Join(a.vaultPath, "Resources")
}

// syncLocalTask mirrors one local task into the vault.
//
// Tickets are skipped: their note already exists and is owned by the Jira
// plugin, which rewrites parts of it on every fetch.
func (a *App) syncLocalTask(id string) {
	if !a.syncEnabled() {
		return
	}
	for i := range a.tasks {
		t := &a.tasks[i]
		if t.ID != id || t.IsTicket() {
			continue
		}

		if t.Done {
			a.archiveLocalTask(t)
			return
		}

		n := vault.TaskNote{
			ID:      t.ID,
			Title:   t.Title,
			Done:    t.Done,
			Created: t.CreatedAt,
		}
		if t.HasDue() {
			due := t.Deadline()
			n.Due = &due
		}
		path, err := vault.WriteTaskNote(a.projectDir(), n)
		if err != nil {
			a.err = "vault sync: " + err.Error()
			return
		}
		t.NotePath = path
		a.persist()
		return
	}
}

// archiveLocalTask moves a finished task's note out of the working folder.
//
// taskii does this itself because the vault's own note mover only ever looks
// at the tickets folder, so a note anywhere else would sit there forever.
func (a *App) archiveLocalTask(t *model.Task) {
	path := t.NotePath
	if path == "" {
		// The cached path can be stale or absent; taskii_id is the identity.
		found, err := vault.FindNoteByID(a.projectDir(), t.ID)
		if err != nil {
			a.err = "vault sync: " + err.Error()
			return
		}
		path = found
	}
	if path == "" {
		return
	}
	moved, err := vault.ArchiveTaskNote(path, a.archiveDir())
	if err != nil {
		a.err = "vault archive: " + err.Error()
		return
	}
	t.NotePath = moved
	a.persist()
}

// retireDeletedTask takes a deleted task's note out of circulation.
//
// The note is moved to the archive and marked deleted rather than removed from
// disk. Deleting a task in taskii is a one-keystroke action, while the note may
// carry subtasks and notes written in Obsidian that taskii never saw — losing
// those to a keystroke would be a poor trade for tidiness. The move is what
// matters: an archived note no longer appears among the open local tasks.
func (a *App) retireDeletedTask(t model.Task) {
	if !a.syncEnabled() || t.ID == "" || t.IsTicket() {
		return
	}
	path := t.NotePath
	if path == "" {
		found, err := vault.FindNoteByID(a.projectDir(), t.ID)
		if err != nil {
			a.err = "vault sync: " + err.Error()
			return
		}
		path = found
	}
	if path == "" {
		return
	}
	// Marked before the move so the status is right wherever it ends up, and
	// so a note that fails to move still reads as deleted.
	if err := vault.SetProperty(path, "status", "deleted"); err != nil {
		a.err = "vault sync: " + err.Error()
	}
	if _, err := vault.ArchiveTaskNote(path, a.archiveDir()); err != nil {
		a.err = "vault archive: " + err.Error()
	}
}

// syncDailyNote writes the notes board to today's dated note.
func (a *App) syncDailyNote() {
	if !a.syncEnabled() {
		return
	}
	bodies := make([]string, 0, len(a.notes))
	for _, n := range a.notes {
		bodies = append(bodies, n.Body)
	}
	if _, err := vault.WriteDailyNote(a.resourcesDir(), a.now(), bodies); err != nil {
		a.err = "daily note: " + err.Error()
	}
}

// syncAllLocalTasks mirrors every local task, used when sync is switched on so
// the vault catches up with what is already in taskii.
func (a *App) syncAllLocalTasks() {
	if !a.syncEnabled() {
		return
	}
	ids := make([]string, 0, len(a.tasks))
	for _, t := range a.tasks {
		if !t.IsTicket() {
			ids = append(ids, t.ID)
		}
	}
	for _, id := range ids {
		a.syncLocalTask(id)
	}
	a.syncDailyNote()
}
