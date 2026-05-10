package testutil

import (
	"fmt"
	"os"
	"path/filepath"
)

func MoveToProjectRoot() error {
	dir, err := FindProjectRoot()
	if err != nil {
		return err
	}
	return os.Chdir(dir)
}
func FindProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found")
		}
		dir = parent
	}
}
