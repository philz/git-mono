package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func handleExplode(args []string) error {
	_ = args // Currently unused, could be used for future options

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

	// Get the base commit from config
	initCommit, err := gitOutput("config", "stitch.init-commit")
	if err != nil {
		return fmt.Errorf("failed to get base commit from config: %v", err)
	}
	initCommit = strings.TrimSpace(initCommit)

	fmt.Printf("Using base commit: %s\n", initCommit) // Get commits since the init commit
	commits, err := getCommitsSince(initCommit)
	if err != nil {
		return fmt.Errorf("failed to get commits since init: %v", err)
	}

	if len(commits) == 0 {
		fmt.Println("No commits to explode")
		return nil
	}

	fmt.Printf("Found %d commits to explode\n", len(commits))

	// For each commit, extract changes for each remote and apply them
	for _, commit := range commits {
		fmt.Printf("Processing commit: %s\n", commit)

		for _, spec := range remotes {
			if err := explodeCommitToRemote(commit, spec); err != nil {
				return fmt.Errorf("failed to explode commit %s to remote %s: %v", commit, spec.Remote, err)
			}
		}
	}

	fmt.Println("Explosion complete")
	return nil
}

// Remove this type since we now use the shared RemoteSpec

// Removed findInitCommit - now using config

func getCommitsSince(initCommit string) ([]string, error) {
	output, err := gitOutput("rev-list", "--reverse", initCommit+"..HEAD")
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(output) == "" {
		return []string{}, nil
	}

	return strings.Split(strings.TrimSpace(output), "\n"), nil
}

// Removed getRemoteSpecsFromCommit - now using config

func explodeCommitToRemote(commit string, spec RemoteSpec) error {
	// Get the parent of this commit
	parent, err := gitOutput("log", "-1", "--pretty=format:%P", commit)
	if err != nil {
		return err
	}
	parentHash := strings.Fields(strings.TrimSpace(parent))[0]

	// Check if this commit affects the directory for this remote
	diff, err := gitOutput("diff", "--name-only", parentHash, commit, "--", spec.Dir)
	if err != nil {
		return err
	}

	if strings.TrimSpace(diff) == "" {
		// No changes to this directory
		return nil
	}

	fmt.Printf("  Exploding changes in %s to remote %s\n", spec.Dir, spec.Remote)

	// Get the tree hash for this directory in the commit
	treeHash, err := gitOutput("rev-parse", commit+":"+spec.Dir)
	if err != nil {
		return err
	}
	treeHash = strings.TrimSpace(treeHash)

	// Get the full commit message (subject + body)
	commitMsg, err := gitOutput("log", "-1", "--pretty=format:%B", commit)
	if err != nil {
		return err
	}

	// Get the current HEAD of the remote branch
	remoteBranch := spec.Remote + "/" + spec.Branch
	remoteHead, err := gitOutput("rev-parse", remoteBranch)
	if err != nil {
		return err
	}
	remoteHead = strings.TrimSpace(remoteHead)

	// Create a new commit on the remote branch preserving original author and date
	// Get original commit info
	origAuthor, err := gitOutput("log", "-1", "--format=%aN <%aE>", commit)
	if err != nil {
		return err
	}
	origAuthor = strings.TrimSpace(origAuthor)
	
	origDate, err := gitOutput("log", "-1", "--format=%aI", commit)
	if err != nil {
		return err
	}
	origDate = strings.TrimSpace(origDate)
	
	// Create commit with original author info
	env := append(os.Environ(),
		"GIT_AUTHOR_NAME="+strings.Split(origAuthor, " <")[0],
		"GIT_AUTHOR_EMAIL="+strings.Trim(strings.Split(origAuthor, " <")[1], ">"),
		"GIT_AUTHOR_DATE="+origDate,
		"GIT_COMMITTER_NAME=git-stitch",
		"GIT_COMMITTER_EMAIL=git-stitch@deterministic",
		"GIT_COMMITTER_DATE="+origDate,
	)
	
	cmd := exec.Command("git", "commit-tree", treeHash, "-p", remoteHead, "-m", strings.TrimSpace(commitMsg))
	cmd.Env = env
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("git commit-tree failed: %v", err)
	}
	newCommit := strings.TrimSpace(string(output))

	// Update the remote branch reference
	if err := runGit("update-ref", "refs/remotes/"+remoteBranch, newCommit); err != nil {
		return err
	}

	fmt.Printf("    Created commit %s on %s\n", newCommit[:8], remoteBranch)

	// TODO: Push to the actual remote repository
	// This would require additional logic to handle authentication and push
	fmt.Printf("    NOTE: You need to manually push to %s\n", spec.Remote)

	return nil
}
