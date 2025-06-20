package main

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "git-stitch %s (%s, %s)\n", version, commit, date)
		fmt.Fprintf(os.Stderr, "Combines multiple repositories into a monorepo structure.\n\n")
		fmt.Fprintf(os.Stderr, "Usage: git-stitch [-no-fetch] ref1 [ref2...]\n")
		os.Exit(1)
	}

	noFetch := false
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "-no-fetch" {
		noFetch = true
		args = args[1:]
	}

	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Error: No refs specified\n")
		os.Exit(1)
	}

	refs := args

	// Parse remote/branch format and fetch if needed
	remoteCommits := make(map[string]string)
	maxTimestamp := int64(0)

	for _, ref := range refs {
		parts := strings.SplitN(ref, "/", 2)
		if len(parts) != 2 {
			fmt.Fprintf(os.Stderr, "Error: ref %s must be in format 'remote/branch'\n", ref)
			os.Exit(1)
		}
		remote := parts[0]
		_ = parts[1] // branch name, used in ref but not needed separately

		// Check if remote exists
		cmd := exec.Command("git", "remote", "get-url", remote)
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: remote '%s' does not exist\n", remote)
			os.Exit(1)
		}

		if !noFetch {
			fmt.Printf("Fetching %s... ", remote)
			cmd := exec.Command("git", "fetch", remote)
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Error fetching %s: %v\n", remote, err)
				os.Exit(1)
			}
		}

		// Get the commit hash
		cmd = exec.Command("git", "rev-parse", ref)
		output, err := cmd.Output()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting commit for %s: %v\n", ref, err)
			os.Exit(1)
		}
		commitHash := strings.TrimSpace(string(output))
		remoteCommits[remote] = commitHash
		fmt.Printf("%s is %s\n", ref, commitHash)

		// Get the commit timestamp to find the maximum
		cmd = exec.Command("git", "show", "-s", "--format=%ct", commitHash)
		output, err = cmd.Output()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting timestamp for %s: %v\n", commitHash, err)
			os.Exit(1)
		}
		timestamp, err := strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing timestamp for %s: %v\n", commitHash, err)
			os.Exit(1)
		}
		if timestamp > maxTimestamp {
			maxTimestamp = timestamp
		}
	}

	// Create the synthetic tree
	treeEntries := []string{}

	// Sort remotes for deterministic output
	remotes := make([]string, 0, len(remoteCommits))
	for remote := range remoteCommits {
		remotes = append(remotes, remote)
	}
	sort.Strings(remotes)

	for _, remote := range remotes {
		commitHash := remoteCommits[remote]
		// Get the tree hash for this commit
		cmd := exec.Command("git", "rev-parse", commitHash+"^{tree}")
		output, err := cmd.Output()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting tree for %s: %v\n", commitHash, err)
			os.Exit(1)
		}
		treeHash := strings.TrimSpace(string(output))
		treeEntries = append(treeEntries, fmt.Sprintf("040000 tree %s\t%s", treeHash, remote))
	}

	// Create the tree
	cmd := exec.Command("git", "mktree")
	cmd.Stdin = strings.NewReader(strings.Join(treeEntries, "\n") + "\n")
	output, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating tree: %v\n", err)
		os.Exit(1)
	}
	treeHash := strings.TrimSpace(string(output))

	// Prepare commit arguments
	commitArgs := []string{"commit-tree", treeHash, "-m", "git-stitch merge"}

	// Add parent commits (sorted for determinism)
	for _, remote := range remotes {
		commitHash := remoteCommits[remote]
		commitArgs = append(commitArgs, "-p", commitHash)
	}

	// Create the commit with deterministic timestamp and author
	cmd = exec.Command("git", commitArgs...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=git-stitch",
		"GIT_AUTHOR_EMAIL=git-stitch@localhost",
		"GIT_COMMITTER_NAME=git-stitch",
		"GIT_COMMITTER_EMAIL=git-stitch@localhost",
		fmt.Sprintf("GIT_AUTHOR_DATE=%d", maxTimestamp),
		fmt.Sprintf("GIT_COMMITTER_DATE=%d", maxTimestamp),
	)

	output, err = cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating commit: %v\n", err)
		os.Exit(1)
	}
	commitHash := strings.TrimSpace(string(output))

	fmt.Printf("Stitched %s into %s\n", strings.Join(remotes, " & "), commitHash)
	fmt.Printf("To check out the new commit, run:\n")
	fmt.Printf("  git checkout -b mono %s\n", commitHash)
	fmt.Printf("Or to update your current branch:\n")
	fmt.Printf("  git reset %s\n", commitHash)
}
