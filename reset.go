package main

import (
	"fmt"
	"strings"
)

func handleReset(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("reset requires exactly 2 arguments: <subdir> <ref>")
	}

	subdir := args[0]
	ref := args[1]

	// Verify we're in a git repository
	if err := runGit("rev-parse", "--git-dir"); err != nil {
		return fmt.Errorf("not in a git repository")
	}

	// Load configuration to find which remote corresponds to this subdir
	remotes, err := loadRemoteSpecs()
	if err != nil {
		return fmt.Errorf("failed to load mono configuration: %v\nHave you run 'git mono init'?", err)
	}

	if len(remotes) == 0 {
		return fmt.Errorf("no mono configuration found\nHave you run 'git mono init'?")
	}

	// Find the remote that manages this subdir
	var targetRemote *RemoteSpec
	for i, remote := range remotes {
		if remote.Dir == subdir {
			targetRemote = &remotes[i]
			break
		}
	}

	if targetRemote == nil {
		return fmt.Errorf("subdir '%s' not found in mono configuration", subdir)
	}

	// Get the previous base commit
	prevBase, err := gitOutput("config", "stitch.init-commit")
	if err != nil {
		return fmt.Errorf("failed to get previous base commit: %v", err)
	}
	prevBase = strings.TrimSpace(prevBase)

	// Fetch the remote to ensure we have the ref
	fmt.Printf("Fetching %s...\n", targetRemote.Remote)
	if err := runGit("fetch", targetRemote.Remote); err != nil {
		return fmt.Errorf("failed to fetch remote %s: %v", targetRemote.Remote, err)
	}

	// Verify the ref exists
	refCommit, err := gitOutput("rev-parse", ref)
	if err != nil {
		return fmt.Errorf("ref '%s' not found: %v", ref, err)
	}
	refCommit = strings.TrimSpace(refCommit)

	// Build tree entries
	var treeEntries []string

	// Include existing files from current HEAD (excluding the subdir we're resetting)
	existingEntries, err := gitOutput("ls-tree", "HEAD")
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(existingEntries), "\n") {
			if line == "" {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) >= 2 {
				name := parts[1]
				if name != subdir {
					treeEntries = append(treeEntries, line)
				}
			}
		}
	}

	// Get tree hash for the new ref
	var treeHash string
	if targetRemote.Subdir == "." {
		// Use the entire tree
		hash, err := gitOutput("rev-parse", refCommit+"^{tree}")
		if err != nil {
			return fmt.Errorf("failed to get tree for %s: %v", refCommit, err)
		}
		treeHash = strings.TrimSpace(hash)
	} else {
		// Get tree hash for subdirectory
		hash, err := gitOutput("rev-parse", refCommit+":"+targetRemote.Subdir)
		if err != nil {
			return fmt.Errorf("failed to get tree for %s:%s: %v", refCommit, targetRemote.Subdir, err)
		}
		treeHash = strings.TrimSpace(hash)
	}

	// Add tree entry for the reset subdir
	treeEntries = append(treeEntries, fmt.Sprintf("040000 tree %s\t%s", treeHash, subdir))

	// Create tree object
	treeInput := strings.Join(treeEntries, "\n")
	newTreeHash, err := gitOutputWithInput(treeInput, "mktree")
	if err != nil {
		return fmt.Errorf("failed to create tree: %v", err)
	}
	newTreeHash = strings.TrimSpace(newTreeHash)

	// Get current HEAD
	currentHead, err := gitOutput("rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("failed to get current HEAD: %v", err)
	}
	currentHead = strings.TrimSpace(currentHead)

	// Create commit object with both current HEAD and target ref as parents
	commitMessage := fmt.Sprintf("Reset %s to %s", subdir, ref)
	parents := []string{currentHead, refCommit}
	newCommit, err := createDeterministicCommit(newTreeHash, commitMessage, parents)
	if err != nil {
		return fmt.Errorf("failed to create commit: %v", err)
	}
	newCommit = strings.TrimSpace(newCommit)

	fmt.Printf("Created reset commit: %s\n", newCommit)
	fmt.Printf("Reset %s to %s\n", subdir, ref)
	fmt.Printf("\nTo check out the new commit, run:\n")
	fmt.Printf("  git checkout %s\n", newCommit)
	fmt.Printf("\nOr to update your current branch:\n")
	fmt.Printf("  git reset --hard %s\n", newCommit)

	return nil
}
