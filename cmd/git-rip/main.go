package main

import (
	"bufio"
	"debug/buildinfo"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

type CommitInfo struct {
	Hash               string
	Message            string
	AuthorName         string
	AuthorEmail        string
	AuthorTimestamp    int64
	CommitterName      string
	CommitterEmail     string
	CommitterTimestamp int64
}

func getBuildInfo() string {
	if info, err := buildinfo.ReadFile(os.Args[0]); err == nil {
		if info.Main.Sum != "" {
			return fmt.Sprintf("%s (%s)", info.Main.Version, info.Main.Sum)
		}
		return info.Main.Version
	}
	return "dev (unknown)"
}

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "-h" || os.Args[1] == "--help") {
		fmt.Printf("git-rip %s\n", getBuildInfo())
		fmt.Printf("Splits monorepo commits back into separate repository branches.\n\n")
		fmt.Printf("Usage: git-rip [prefix]\n")
		fmt.Printf("\nIf no prefix is specified, 'rip-<timestamp>' is used.\n")
		return
	}
	prefix := ""
	if len(os.Args) > 1 {
		prefix = os.Args[1]
	} else {
		// Use timestamp-based prefix
		prefix = fmt.Sprintf("rip-%d", time.Now().Unix())
	}

	// Find the base merge commit (look for commits with message "Monorepo merge")
	baseCommit, err := findBaseMergeCommit()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding base commit: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Found base commit: %s\n", baseCommit)

	// Get list of commits since the base commit
	commits, err := getCommitsSince(baseCommit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting commits: %v\n", err)
		os.Exit(1)
	}

	if len(commits) == 0 {
		fmt.Println("No commits to rip since base commit")
		return
	}

	// Get the remotes from the base commit (subdirectories)
	remotes, err := getRemotesFromBaseCommit(baseCommit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting remotes from base commit: %v\n", err)
		os.Exit(1)
	}

	// Initialize branches for each remote at their original commit
	branchHeads := make(map[string]string)
	for _, remote := range remotes {
		// Get the original commit for this remote from the base merge commit parents
		originalCommit, err := getOriginalCommitForRemote(baseCommit, remote)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting original commit for %s: %v\n", remote, err)
			os.Exit(1)
		}
		branchHeads[remote] = originalCommit
	}

	// Process each commit
	for _, commit := range commits {
		fmt.Printf("Processing commit: %s\n", commit.Hash)

		// Get the files changed in this commit
		changedFiles, err := getChangedFiles(commit.Hash)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting changed files for %s: %v\n", commit.Hash, err)
			os.Exit(1)
		}

		// Group files by remote (directory)
		filesByRemote := make(map[string][]string)
		for _, file := range changedFiles {
			parts := strings.SplitN(file, "/", 2)
			if len(parts) == 2 {
				remote := parts[0]
				filePath := parts[1]
				if slices.Contains(remotes, remote) {
					filesByRemote[remote] = append(filesByRemote[remote], filePath)
				}
			}
		}

		// Create a commit for each remote that has changed files
		for _, remote := range remotes {
			files, hasChanges := filesByRemote[remote]
			if !hasChanges {
				continue
			}

			// Create a tree with changes for this remote
			newCommit, err := createCommitForRemote(commit, remote, files, branchHeads[remote])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error creating commit for %s: %v\n", remote, err)
				os.Exit(1)
			}

			branchHeads[remote] = newCommit
			fmt.Printf("Created commit %s for %s\n", newCommit, remote)
		}
	}

	// Create branches
	fmt.Println("Branches created:")
	for _, remote := range remotes {
		branchName := fmt.Sprintf("%s-%s", prefix, remote)
		cmd := exec.Command("git", "branch", branchName, branchHeads[remote])
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating branch %s: %v\n", branchName, err)
			os.Exit(1)
		}
		fmt.Printf("  %s\n", branchName)
	}
}

func findBaseMergeCommit() (string, error) {
	cmd := exec.Command("git", "log", "--grep=git-stitch merge", "--format=%H", "-1")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	commitHash := strings.TrimSpace(string(output))
	if commitHash == "" {
		return "", fmt.Errorf("no merge commit found with message 'git-stitch merge'")
	}
	return commitHash, nil
}

func getCommitsSince(baseCommit string) ([]CommitInfo, error) {
	cmd := exec.Command("git", "rev-list", "--reverse", fmt.Sprintf("%s..HEAD", baseCommit))
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	if len(output) == 0 {
		return []CommitInfo{}, nil
	}

	hashes := strings.Fields(string(output))
	commits := make([]CommitInfo, 0, len(hashes))

	for _, hash := range hashes {
		commit, err := getCommitInfo(hash)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to get info for commit %s: %v\n", hash, err)
			continue
		}
		commits = append(commits, commit)
	}

	return commits, nil
}

func getCommitInfo(hash string) (CommitInfo, error) {
	cmd := exec.Command("git", "show", "-s", "--format=%H%x00%B%x00%an%x00%ae%x00%at%x00%cn%x00%ce%x00%ct", hash)
	output, err := cmd.Output()
	if err != nil {
		return CommitInfo{}, err
	}

	parts := strings.Split(strings.TrimSpace(string(output)), "\x00")
	if len(parts) < 8 {
		return CommitInfo{}, fmt.Errorf("unexpected git show output")
	}

	authorTimestamp, err := strconv.ParseInt(parts[4], 10, 64)
	if err != nil {
		return CommitInfo{}, err
	}

	committerTimestamp, err := strconv.ParseInt(parts[7], 10, 64)
	if err != nil {
		return CommitInfo{}, err
	}

	return CommitInfo{
		Hash:               parts[0],
		Message:            strings.TrimSpace(parts[1]),
		AuthorName:         parts[2],
		AuthorEmail:        parts[3],
		AuthorTimestamp:    authorTimestamp,
		CommitterName:      parts[5],
		CommitterEmail:     parts[6],
		CommitterTimestamp: committerTimestamp,
	}, nil
}

func getRemotesFromBaseCommit(baseCommit string) ([]string, error) {
	cmd := exec.Command("git", "ls-tree", baseCommit)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var remotes []string
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) >= 4 && parts[1] == "tree" {
			// Extract directory name from the tree entry
			dirName := strings.Join(parts[3:], " ")
			remotes = append(remotes, dirName)
		}
	}

	sort.Strings(remotes)
	return remotes, nil
}

func getOriginalCommitForRemote(baseCommit, remote string) (string, error) {
	// Get the parents of the base merge commit
	cmd := exec.Command("git", "show", "-s", "--format=%P", baseCommit)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	parents := strings.Fields(string(output))
	if len(parents) == 0 {
		return "", fmt.Errorf("no parents found for base commit")
	}

	// Try to match the remote with the correct parent by checking tree content
	for _, parent := range parents {
		// Get the tree from this parent
		cmd = exec.Command("git", "rev-parse", parent+"^{tree}")
		output, err = cmd.Output()
		if err != nil {
			continue
		}
		parentTree := strings.TrimSpace(string(output))

		// Get the tree for this remote in the base commit
		cmd = exec.Command("git", "rev-parse", fmt.Sprintf("%s:%s^{tree}", baseCommit, remote))
		output, err = cmd.Output()
		if err != nil {
			continue
		}
		remoteTree := strings.TrimSpace(string(output))

		if parentTree == remoteTree {
			return parent, nil
		}
	}

	// Fallback: return the first parent (this assumes order is preserved)
	return parents[0], nil
}

func getChangedFiles(commitHash string) ([]string, error) {
	cmd := exec.Command("git", "diff-tree", "--no-commit-id", "--name-only", "-r", commitHash)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	files := strings.Fields(string(output))
	return files, nil
}

func createCommitForRemote(commit CommitInfo, remote string, files []string, parentCommit string) (string, error) {
	// Get the parent tree
	cmd := exec.Command("git", "rev-parse", parentCommit+"^{tree}")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	parentTree := strings.TrimSpace(string(output))

	// Read the parent tree
	cmd = exec.Command("git", "ls-tree", "-r", parentTree)
	output, err = cmd.Output()
	if err != nil {
		return "", err
	}

	// Parse existing tree entries
	treeEntries := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) >= 4 {
			mode := parts[0]
			type_ := parts[1]
			hash := parts[2]
			path := strings.Join(parts[3:], " ")
			treeEntries[path] = fmt.Sprintf("%s %s %s\t%s", mode, type_, hash, path)
		}
	}

	// Update tree entries with changes from the monorepo commit
	for _, file := range files {
		// Get the blob hash for this file from the monorepo commit
		monorepoPath := fmt.Sprintf("%s/%s", remote, file)
		cmd = exec.Command("git", "rev-parse", fmt.Sprintf("%s:%s", commit.Hash, monorepoPath))
		output, err = cmd.Output()
		if err != nil {
			// File might have been deleted
			delete(treeEntries, file)
			continue
		}
		blobHash := strings.TrimSpace(string(output))

		// Get the file mode
		cmd = exec.Command("git", "ls-tree", commit.Hash, monorepoPath)
		output, err = cmd.Output()
		if err != nil {
			continue
		}
		treeLine := strings.TrimSpace(string(output))
		parts := strings.Fields(treeLine)
		if len(parts) >= 3 {
			mode := parts[0]
			treeEntries[file] = fmt.Sprintf("%s blob %s\t%s", mode, blobHash, file)
		}
	}

	// Create new tree
	var treeInput strings.Builder
	for _, entry := range treeEntries {
		treeInput.WriteString(entry + "\n")
	}

	cmd = exec.Command("git", "mktree")
	cmd.Stdin = strings.NewReader(treeInput.String())
	output, err = cmd.Output()
	if err != nil {
		return "", err
	}
	newTree := strings.TrimSpace(string(output))

	// Create the commit
	cmd = exec.Command("git", "commit-tree", newTree, "-p", parentCommit, "-m", commit.Message)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("GIT_AUTHOR_NAME=%s", commit.AuthorName),
		fmt.Sprintf("GIT_AUTHOR_EMAIL=%s", commit.AuthorEmail),
		fmt.Sprintf("GIT_COMMITTER_NAME=%s", commit.CommitterName),
		fmt.Sprintf("GIT_COMMITTER_EMAIL=%s", commit.CommitterEmail),
		fmt.Sprintf("GIT_AUTHOR_DATE=%d", commit.AuthorTimestamp),
		fmt.Sprintf("GIT_COMMITTER_DATE=%d", commit.CommitterTimestamp),
	)

	output, err = cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}
