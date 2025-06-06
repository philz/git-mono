package main

import (
	"fmt"
	"strings"
)

func handleRebase(args []string) error {
	// Parse optional subdir/ref pairs
	if len(args)%2 != 0 {
		return fmt.Errorf("rebase requires subdir/ref pairs: <subdir1> <ref1> [<subdir2> <ref2> ...]")
	}

	// Build override map by subdir
	overrides := make(map[string]string)
	for i := 0; i < len(args); i += 2 {
		overrides[args[i]] = args[i+1]
	}

	// Verify we're in a git repository
	if err := runGit("rev-parse", "--git-dir"); err != nil {
		return fmt.Errorf("not in a git repository")
	}

	// Load configuration
	remotes, err := loadRemoteSpecs()
	if err != nil {
		return fmt.Errorf("failed to load mono configuration: %v\nHave you run 'git mono init'?", err)
	}

	if len(remotes) == 0 {
		return fmt.Errorf("no mono configuration found\nHave you run 'git mono init'?")
	}

	// Get the previous base commit
	prevBase, err := gitOutput("config", "stitch.init-commit")
	if err != nil {
		return fmt.Errorf("failed to get previous base commit: %v", err)
	}
	prevBase = strings.TrimSpace(prevBase)

	// If overrides specified, only rebase those subdirs
	var remotesToRebase []RemoteSpec
	if len(overrides) > 0 {
		// Only include remotes that have subdir overrides
		for _, remote := range remotes {
			if newRef, hasOverride := overrides[remote.Dir]; hasOverride {
				// Create a copy with the new ref
				newRemote := remote
				newRemote.Branch = newRef
				remotesToRebase = append(remotesToRebase, newRemote)
			}
		}
	} else {
		// Use all remotes with their default branches
		remotesToRebase = remotes
	}

	// Fetch involved remotes
	fmt.Println("Fetching remotes...")
	for _, remote := range remotesToRebase {
		fmt.Printf("  Fetching %s...\n", remote.Remote)
		if err := runGit("fetch", remote.Remote); err != nil {
			return fmt.Errorf("failed to fetch remote %s: %v", remote.Remote, err)
		}
	}

	// Get current HEAD
	currentHead, err := gitOutput("rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("failed to get current HEAD: %v", err)
	}
	currentHead = strings.TrimSpace(currentHead)

	// Build tree entries
	var treeEntries []string

	// Include existing files from current HEAD (excluding directories we're about to update)
	existingEntries, err := gitOutput("ls-tree", "HEAD")
	if err == nil {
		updatedDirs := make(map[string]bool)
		for _, remote := range remotesToRebase {
			updatedDirs[remote.Dir] = true
		}

		for _, line := range strings.Split(strings.TrimSpace(existingEntries), "\n") {
			if line == "" {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) >= 2 {
				name := parts[1]
				if !updatedDirs[name] {
					treeEntries = append(treeEntries, line)
				}
			}
		}
	}

	// Add entries for remotes being rebased
	var parentCommits []string
	parentCommits = append(parentCommits, prevBase) // Use previous base as parent

	for _, remote := range remotesToRebase {
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
			hash, err := gitOutput("rev-parse", commitRef+":"+remote.Subdir)
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
	newBaseCommit, err := createDeterministicCommit(treeHash, "Monorepo rebase", parentCommits)
	if err != nil {
		return fmt.Errorf("failed to create commit: %v", err)
	}
	newBaseCommit = strings.TrimSpace(newBaseCommit)

	// Update the stitch.init-commit config
	runGit("config", "stitch.init-commit", newBaseCommit)

	fmt.Printf("Created new base commit: %s\n", newBaseCommit)
	fmt.Printf("Previous base: %s\n", prevBase)

	// Check if we need to rebase
	if currentHead == prevBase {
		fmt.Println("Already at base commit, no rebase needed")
		fmt.Printf("To check out the new base, run:\n")
		fmt.Printf("  git reset --hard %s\n", newBaseCommit)
		return nil
	}

	// Perform rebase
	fmt.Printf("Rebasing current branch onto %s...\n", newBaseCommit)
	if err := runGit("rebase", "--onto", newBaseCommit, prevBase); err != nil {
		return fmt.Errorf("rebase failed: %v\nYou may need to resolve conflicts and continue manually", err)
	}

	fmt.Println("Rebase completed successfully")
	return nil
}

func loadRemoteSpecs() ([]RemoteSpec, error) {
	// Get list of remotes from stitch.remotes
	remotesStr, err := gitOutput("config", "stitch.remotes")
	if err != nil {
		return nil, err
	}

	remoteNames := strings.Fields(strings.TrimSpace(remotesStr))
	if len(remoteNames) == 0 {
		return nil, fmt.Errorf("no remotes configured")
	}

	var remotes []RemoteSpec
	for _, remoteName := range remoteNames {
		prefix := fmt.Sprintf("remote.%s", remoteName)

		branch, err := gitOutput("config", prefix+".stitch-branch")
		if err != nil {
			return nil, fmt.Errorf("missing config %s.stitch-branch", prefix)
		}

		subdir, err := gitOutput("config", prefix+".stitch-subdir")
		if err != nil {
			return nil, fmt.Errorf("missing config %s.stitch-subdir", prefix)
		}

		dir, err := gitOutput("config", prefix+".stitch-dir")
		if err != nil {
			return nil, fmt.Errorf("missing config %s.stitch-dir", prefix)
		}

		remotes = append(remotes, RemoteSpec{
			Remote: remoteName,
			Branch: strings.TrimSpace(branch),
			Subdir: strings.TrimSpace(subdir),
			Dir:    strings.TrimSpace(dir),
		})
	}

	return remotes, nil
}
