package model

import (
	"encoding/json"
	"os"
)

const tasksFile = "tasks.json"

func Load() ([]Task, error) {
	b, err := readData(tasksFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []Task{}, nil
		}
		return nil, err
	}
	if len(b) == 0 {
		return []Task{}, nil
	}
	var tasks []Task
	if err := json.Unmarshal(b, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func Save(tasks []Task) error {
	b, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return err
	}
	return writeData(tasksFile, b)
}
