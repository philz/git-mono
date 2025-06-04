package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// runGit executes a git command and returns an error if it fails
func runGit(args ...string) error {
	cmd := exec.Command("git", args...)
	return cmd.Run()
}

// gitOutput executes a git command and returns its output
func gitOutput(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// gitOutputWithInput executes a git command with stdin input and returns its output
func gitOutputWithInput(input string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Stdin = strings.NewReader(input)

	var out bytes.Buffer
	cmd.Stdout = &out

	err := cmd.Run()
	if err != nil {
		return "", err
	}

	return out.String(), nil
}

// getCommitDate returns the commit date for a given commit hash in ISO format
func getCommitDate(commitHash string) (string, error) {
	return gitOutput("log", "-1", "--format=%aI", commitHash)
}

// getMaxCommitDate returns the maximum date from a list of commits in ISO format
func getMaxCommitDate(commits []string) (string, error) {
	if len(commits) == 0 {
		return "", fmt.Errorf("no commits provided")
	}
	
	var maxDate string
	for _, commit := range commits {
		if commit == "" {
			continue
		}
		date, err := getCommitDate(commit)
		if err != nil {
			return "", fmt.Errorf("failed to get date for commit %s: %v", commit, err)
		}
		date = strings.TrimSpace(date)
		if maxDate == "" || date > maxDate {
			maxDate = date
		}
	}
	return maxDate, nil
}

// createDeterministicCommit creates a commit with deterministic author and date
func createDeterministicCommit(treeHash, message string, parents []string) (string, error) {
	// Get the maximum date from all parent commits
	commitDate, err := getMaxCommitDate(parents)
	if err != nil {
		return "", fmt.Errorf("failed to determine commit date: %v", err)
	}
	
	// Build commit-tree command with explicit author and date
	commitArgs := []string{"commit-tree", treeHash, "-m", message}
	for _, parent := range parents {
		if parent != "" {
			commitArgs = append(commitArgs, "-p", parent)
		}
	}
	
	// Set environment variables for deterministic author and date
	env := append(os.Environ(),
		"GIT_AUTHOR_NAME=git-stitch",
		"GIT_AUTHOR_EMAIL=git-stitch@deterministic",
		"GIT_AUTHOR_DATE="+commitDate,
		"GIT_COMMITTER_NAME=git-stitch",
		"GIT_COMMITTER_EMAIL=git-stitch@deterministic",
		"GIT_COMMITTER_DATE="+commitDate,
	)
	
	cmd := exec.Command("git", commitArgs...)
	cmd.Env = env
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git commit-tree failed: %v", err)
	}
	return strings.TrimSpace(string(output)), nil
}