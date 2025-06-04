package main

import (
	"fmt"
	"strings"
)

type RemoteSpec struct {
	Remote string // remote name (e.g., "origin")
	Branch string // branch name (e.g., "main")
	Subdir string // subdirectory in remote (. for root)
	Dir    string // directory in monorepo
}

func handleInit(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("init requires at least two remote names")
	}

	// Build remote specifications from remote names
	var remotes []RemoteSpec
	for _, remoteName := range args {
		// Check if remote exists
		if err := runGit("remote", "get-url", remoteName); err != nil {
			return fmt.Errorf("remote %s does not exist", remoteName)
		}

		// Get the default branch using symbolic-ref
		defaultBranch, err := getDefaultBranch(remoteName)
		if err != nil {
			return fmt.Errorf("failed to get default branch for %s: %v", remoteName, err)
		}

		remotes = append(remotes, RemoteSpec{
			Remote: remoteName,
			Branch: defaultBranch,
			Subdir: ".",        // Default to root
			Dir:    remoteName, // Use remote name as directory
		})
	}

	// Verify we're in a git repository
	if err := runGit("rev-parse", "--git-dir"); err != nil {
		return fmt.Errorf("not in a git repository")
	}

	// No need to get current HEAD - we'll create a pure merge of remotes

	// Fetch all remotes
	for _, remote := range remotes {
		fmt.Printf("Fetching %s (branch: %s)...\n", remote.Remote, remote.Branch)
		if err := runGit("fetch", remote.Remote); err != nil {
			return fmt.Errorf("failed to fetch remote %s: %v", remote.Remote, err)
		}
	}

	// Build tree entries - just the remote directories
	var treeEntries []string

	// Add entries for each remote
	var parentCommits []string

	for _, remote := range remotes {
		// Build commit reference
		commitRef := remote.Remote + "/" + remote.Branch

		// Get tree hash for the subdirectory
		var treeHash string
		if remote.Subdir == "." {
			// Use the entire tree
			hash, err := gitOutput("rev-parse", commitRef+"^{tree}")
			if err != nil {
				return fmt.Errorf("failed to get tree for %s: %v", commitRef, err)
			}
			treeHash = strings.TrimSpace(hash)
		} else {
			// Get tree hash for subdirectory
			hash, err := gitOutput("rev-parse", commitRef+":"+remote.Subdir+"^{tree}")
			if err != nil {
				return fmt.Errorf("failed to get tree for %s:%s: %v", commitRef, remote.Subdir, err)
			}
			treeHash = strings.TrimSpace(hash)
		}

		// Add tree entry
		treeEntries = append(treeEntries, fmt.Sprintf("040000 tree %s\t%s", treeHash, remote.Dir))

		// Add parent commit
		commitHash, err := gitOutput("rev-parse", commitRef)
		if err != nil {
			return fmt.Errorf("failed to get commit hash for %s: %v", commitRef, err)
		}
		parentCommits = append(parentCommits, strings.TrimSpace(commitHash))
	}

	// Create tree object
	treeInput := strings.Join(treeEntries, "\n")
	treeHash, err := gitOutputWithInput(treeInput, "mktree")
	if err != nil {
		return fmt.Errorf("failed to create tree: %v", err)
	}
	treeHash = strings.TrimSpace(treeHash)

	// Create commit object with deterministic author and date
	commitHash, err := createDeterministicCommit(treeHash, "Monorepo initialization", parentCommits)
	if err != nil {
		return fmt.Errorf("failed to create commit: %v", err)
	}
	commitHash = strings.TrimSpace(commitHash)

	// Store configuration in remote sections
	for _, remote := range remotes {
		prefix := fmt.Sprintf("remote.%s", remote.Remote)
		runGit("config", prefix+".mono-branch", remote.Branch)
		runGit("config", prefix+".mono-subdir", remote.Subdir)
		runGit("config", prefix+".mono-dir", remote.Dir)
	}

	// Store global mono config
	remoteNames := make([]string, len(remotes))
	for i, remote := range remotes {
		remoteNames[i] = remote.Remote
	}
	runGit("config", "mono.remotes", strings.Join(remoteNames, " "))
	runGit("config", "mono.init-commit", commitHash)

	fmt.Printf("Created monorepo commit: %s\n", commitHash)
	fmt.Printf("\nTo check out the new commit, run:\n")
	fmt.Printf("  git checkout -b mono %s\n", commitHash)
	fmt.Printf("\nOr to update your current branch:\n")
	fmt.Printf("  git reset --hard %s\n", commitHash)

	return nil
}

// getDefaultBranch gets the default branch for a remote using symbolic-ref
func getDefaultBranch(remoteName string) (string, error) {
	// First, fetch the remote to ensure we have the refs
	if err := runGit("fetch", remoteName); err != nil {
		return "", fmt.Errorf("failed to fetch remote %s: %v", remoteName, err)
	}

	// Try to get the symbolic ref
	symbolicRef, err := gitOutput("symbolic-ref", fmt.Sprintf("refs/remotes/%s/HEAD", remoteName))
	if err != nil {
		// If symbolic ref doesn't exist, try to set it
		if err := runGit("remote", "set-head", remoteName, "--auto"); err != nil {
			return "", fmt.Errorf("failed to set remote HEAD: %v", err)
		}

		// Try again
		symbolicRef, err = gitOutput("symbolic-ref", fmt.Sprintf("refs/remotes/%s/HEAD", remoteName))
		if err != nil {
			return "", err
		}
	}

	// Extract branch name from refs/remotes/origin/main -> main
	symbolicRef = strings.TrimSpace(symbolicRef)
	parts := strings.Split(symbolicRef, "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid symbolic ref format: %s", symbolicRef)
	}

	return parts[len(parts)-1], nil
}
