package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGitMonoWorkflow tests the complete git-stitch workflow
func TestGitMonoWorkflow(t *testing.T) {
	// Create a temporary directory for testing
	testDir, err := os.MkdirTemp("", "git-stitch-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(testDir)

	// Change to test directory
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current dir: %v", err)
	}
	defer os.Chdir(originalDir)

	// Create test repositories
	if err := createTestRepos(testDir); err != nil {
		t.Fatalf("Failed to create test repos: %v", err)
	}

	// Test the monorepo workflow
	monoDir := filepath.Join(testDir, "mono")
	os.Chdir(monoDir)

	// Test init command
	t.Run("init", func(t *testing.T) {
		aRepoPath := filepath.Join(testDir, "a-repo")
		bRepoPath := filepath.Join(testDir, "b-repo")

		// Test minimum 2 remotes requirement
		err := handleInit([]string{"a-remote"})
		if err == nil {
			t.Error("Expected error for single remote, but got none")
		}

		// Test non-existent remotes
		err = handleInit([]string{"a-remote", "b-remote"})
		if err == nil {
			t.Error("Expected error for non-existent remotes, but got none")
		}

		// Add the remotes first
		runCommand(t, "git", "remote", "add", "a-remote", aRepoPath)
		runCommand(t, "git", "remote", "add", "b-remote", bRepoPath)

		// Set up symbolic refs for the remotes
		runCommand(t, "git", "fetch", "a-remote")
		runCommand(t, "git", "fetch", "b-remote")
		runCommand(t, "git", "remote", "set-head", "a-remote", "--auto")
		runCommand(t, "git", "remote", "set-head", "b-remote", "--auto")

		// Now init should work
		err = handleInit([]string{"a-remote", "b-remote"})
		if err != nil {
			t.Fatalf("Init failed: %v", err)
		}

		// Verify config was stored in remote sections
		remotes, err := gitOutput("config", "stitch.remotes")
		if err != nil {
			t.Fatalf("Failed to get stitch.remotes: %v", err)
		}
		if !strings.Contains(remotes, "a-remote") || !strings.Contains(remotes, "b-remote") {
			t.Errorf("Expected stitch.remotes to contain both remotes, got %s", strings.TrimSpace(remotes))
		}

		// Verify remote-specific config was stored
		branch, err := gitOutput("config", "remote.a-remote.stitch-branch")
		if err != nil {
			t.Fatalf("Failed to get remote.a-remote.stitch-branch: %v", err)
		}
		if strings.TrimSpace(branch) != "master" {
			t.Errorf("Expected master, got %s", strings.TrimSpace(branch))
		}

		dir, err := gitOutput("config", "remote.a-remote.stitch-dir")
		if err != nil {
			t.Fatalf("Failed to get remote.a-remote.stitch-dir: %v", err)
		}
		if strings.TrimSpace(dir) != "a-remote" {
			t.Errorf("Expected a-remote, got %s", strings.TrimSpace(dir))
		}
	})

	// Test rebase command
	t.Run("rebase", func(t *testing.T) {
		// First, check out the monorepo commit
		initCommit, err := gitOutput("config", "stitch.init-commit")
		if err != nil {
			t.Fatalf("Failed to get init commit: %v", err)
		}
		initCommit = strings.TrimSpace(initCommit)

		runCommand(t, "git", "checkout", "-b", "test-branch", initCommit)

		// Test rebase
		err = handleRebase([]string{})
		if err != nil {
			t.Fatalf("Rebase failed: %v", err)
		}

		// Verify new base commit was created and stored
		newInitCommit, err := gitOutput("config", "stitch.init-commit")
		if err != nil {
			t.Fatalf("Failed to get new init commit: %v", err)
		}
		newInitCommit = strings.TrimSpace(newInitCommit)

		if newInitCommit == initCommit {
			t.Error("Expected new base commit to be different from original")
		}
	})

	// Test explode command
	t.Run("explode", func(t *testing.T) {
		// Make a change in the monorepo
		aRemoteFile := filepath.Join("a-remote", "new-file.txt")
		err := os.WriteFile(aRemoteFile, []byte("new content"), 0644)
		if err != nil {
			t.Fatalf("Failed to write test file: %v", err)
		}

		runCommand(t, "git", "add", aRemoteFile)
		runCommand(t, "git", "commit", "-m", "Add new file to a-remote")

		// Test explode
		err = handleExplode([]string{})
		if err != nil {
			t.Fatalf("Explode failed: %v", err)
		}

		// Verify that a new commit was created on the remote branch
		remoteCommits, err := gitOutput("log", "--oneline", "a-remote/master")
		if err != nil {
			t.Fatalf("Failed to get remote commits: %v", err)
		}

		if !strings.Contains(remoteCommits, "Add new file to a-remote") {
			t.Error("Expected commit message to be preserved in remote branch")
		}
	})
}

// createTestRepos creates test repositories for testing
func createTestRepos(testDir string) error {
	// Create repo A
	aRepoDir := filepath.Join(testDir, "a-repo")
	if err := os.MkdirAll(aRepoDir, 0755); err != nil {
		return err
	}

	if err := runCommandInDir(aRepoDir, "git", "init"); err != nil {
		return err
	}

	aFile := filepath.Join(aRepoDir, "lib.js")
	if err := os.WriteFile(aFile, []byte("console.log('hello from A');"), 0644); err != nil {
		return err
	}

	if err := runCommandInDir(aRepoDir, "git", "add", "lib.js"); err != nil {
		return err
	}

	if err := runCommandInDir(aRepoDir, "git", "commit", "-m", "Initial A commit"); err != nil {
		return err
	}

	// Create repo B
	bRepoDir := filepath.Join(testDir, "b-repo")
	if err := os.MkdirAll(bRepoDir, 0755); err != nil {
		return err
	}

	if err := runCommandInDir(bRepoDir, "git", "init"); err != nil {
		return err
	}

	bFile := filepath.Join(bRepoDir, "lib.py")
	if err := os.WriteFile(bFile, []byte("def hello(): print('hello from B')"), 0644); err != nil {
		return err
	}

	if err := runCommandInDir(bRepoDir, "git", "add", "lib.py"); err != nil {
		return err
	}

	if err := runCommandInDir(bRepoDir, "git", "commit", "-m", "Initial B commit"); err != nil {
		return err
	}

	// Create monorepo
	monoDir := filepath.Join(testDir, "mono")
	if err := os.MkdirAll(monoDir, 0755); err != nil {
		return err
	}

	if err := runCommandInDir(monoDir, "git", "init"); err != nil {
		return err
	}

	readmeFile := filepath.Join(monoDir, "README.md")
	if err := os.WriteFile(readmeFile, []byte("# Monorepo"), 0644); err != nil {
		return err
	}

	if err := runCommandInDir(monoDir, "git", "add", "README.md"); err != nil {
		return err
	}

	if err := runCommandInDir(monoDir, "git", "commit", "-m", "Initial monorepo commit"); err != nil {
		return err
	}

	return nil
}

// runCommand runs a command and fails the test if it returns an error
func runCommand(t *testing.T, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Command %s %v failed: %v", name, args, err)
	}
}

// runCommandInDir runs a command in a specific directory
func runCommandInDir(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	return cmd.Run()
}
