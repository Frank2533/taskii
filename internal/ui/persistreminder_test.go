package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"taskii/internal/model"
)

// A reminder set in the picker has to reach disk, or it is gone the moment the
// app restarts. The other picker tests run in mock mode, which never persists,
// so this exercises the real save path.
func TestReminderSetInThePickerReachesDisk(t *testing.T) {
	a, _ := dashApp(t)
	a.noPersist = false
	// dashApp points persistence at its own scratch directory.
	dir := model.DataDir()
	fixed := time.Now()
	a.now = func() time.Time { return fixed }

	a = press(t, a, "a")
	a = typeInto(a, "call the bank")
	a = press(t, a, "enter")

	a.todaySelected = 0
	a = press(t, a, "D", "c")
	a = press(t, a, "tab", "2") // 2 hours
	a = press(t, a, "enter")

	if len(a.tasks) != 1 || a.tasks[0].RemindAt == nil {
		t.Fatalf("the reminder is not in memory: %+v", a.tasks)
	}

	b, err := os.ReadFile(filepath.Join(dir, "tasks.json"))
	if err != nil {
		t.Fatalf("tasks were never written: %v", err)
	}
	var stored []model.Task
	if err := json.Unmarshal(b, &stored); err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 {
		t.Fatalf("stored %d tasks, want 1", len(stored))
	}
	if stored[0].RemindAt == nil {
		t.Errorf("the reminder was not persisted:\n%s", b)
	}
	if !stored[0].RemindAt.Equal(fixed.Add(2 * time.Hour)) {
		t.Errorf("stored reminder = %v, want %v", stored[0].RemindAt, fixed.Add(2*time.Hour))
	}
}

// The tick mutates the app while its result is being batched. Evaluation order
// for a plain operand beside a call is not specified, so the state the tick
// produced must be returned explicitly rather than relying on it.
func TestReminderTickKeepsWhatItMarked(t *testing.T) {
	now := time.Now()
	a, _ := dashApp(t)
	a.noPersist = false
	a.now = func() time.Time { return now }

	past := now.Add(-30 * time.Second)
	a.tasks = []model.Task{{
		ID: "t1", Title: "start the thing", Date: now.Format(dateFormat),
		CreatedAt: now, RemindAt: &past,
	}}

	next, _ := a.Update(reminderTickMsg(now))
	got := next.(App)
	if len(got.tasks) != 1 {
		t.Fatalf("tasks = %d", len(got.tasks))
	}
	if !got.tasks[0].Reminded {
		t.Error("the tick's marking was lost from the returned model, so it will fire again")
	}
}
