package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func GetCurrentModuleRoot() (string, string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("unable to get current working directory: %w", err)
	}
	current, err := filepath.Abs(cwd)
	if err != nil {
		return "", "", fmt.Errorf("unable to get absolute path of path %v: %w", cwd, err)
	}
	for {
		entries, err := os.ReadDir(current)
		if err != nil {
			return "", "", fmt.Errorf("unable to read directory %v: %w", current, err)
		}
		for _, entry := range entries {
			if entry.Name() == "go.mod" && entry.Type().IsRegular() {
				return current, cwd, nil
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return "", "", fmt.Errorf("unable to find go.mod file")
}

func BuildCurrentTest(target string, targetOs, targetArch string) error {
	cmd := exec.Command("go", "test", "-c", "-o", target)
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("unable to get current working directory: %w", err)
	}
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "GOOS="+targetOs, "GOARCH="+targetArch, "CGO_ENABLED=0")
	_, err = cmd.Output()
	if err != nil {
		return fmt.Errorf("unable to build test: %w", err)
	}
	return nil
}
