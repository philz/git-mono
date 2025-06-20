package main

import (
	"bufio"
	"debug/buildinfo"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

type FileChange struct {
	Path   string
	Status string // "A" for added, "M" for modified, "D" for deleted
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
	if os.Getenv("GIT_STITCH_VERBOSE") != "" {
		fmt.Printf("Found base commit: %s\n", baseCommit)
	}

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
		if os.Getenv("GIT_STITCH_VERBOSE") != "" {
			fmt.Printf("Remote %s starts from commit %s\n", remote, originalCommit)
		}
	}

	// Process each commit
	for _, commit := range commits {
		if os.Getenv("GIT_STITCH_VERBOSE") != "" {
			fmt.Printf("Processing commit: %s\n", commit.Hash)
		}

		// Get the files changed in this commit
		changedFiles, err := getChangedFilesWithStatus(commit.Hash)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting changed files for %s: %v\n", commit.Hash, err)
			os.Exit(1)
		}

		// Group files by remote (directory)
		filesByRemote := make(map[string][]FileChange)
		for _, fileChange := range changedFiles {
			parts := strings.SplitN(fileChange.Path, "/", 2)
			if len(parts) == 2 {
				remote := parts[0]
				filePath := parts[1]
				if slices.Contains(remotes, remote) {
					filesByRemote[remote] = append(filesByRemote[remote], FileChange{
						Path:   filePath,
						Status: fileChange.Status,
					})
				}
			}
		}

		// Create a commit for each remote that has changed files
		for _, remote := range remotes {
			fileChanges, hasChanges := filesByRemote[remote]
			if !hasChanges {
				continue
			}

			if os.Getenv("GIT_STITCH_VERBOSE") != "" {
				fmt.Printf("Creating commit for %s with file changes: %v\n", remote, fileChanges)
			}
			// Create a tree with changes for this remote
			newCommit, err := createCommitForRemoteWithChanges(commit, remote, fileChanges, branchHeads[remote])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error creating commit for %s: %v\n", remote, err)
				fmt.Fprintf(os.Stderr, "Commit details: %+v\n", commit)
				fmt.Fprintf(os.Stderr, "Parent commit: %s\n", branchHeads[remote])
				os.Exit(1)
			}

			branchHeads[remote] = newCommit
			if os.Getenv("GIT_STITCH_VERBOSE") != "" {
				fmt.Printf("Created commit %s for %s\n", newCommit, remote)
			}
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
		return "", fmt.Errorf("failed to get parents of base commit %s: %v", baseCommit, err)
	}

	parents := strings.Fields(string(output))
	if len(parents) == 0 {
		return "", fmt.Errorf("no parents found for base commit %s", baseCommit)
	}

	if os.Getenv("GIT_STITCH_VERBOSE") != "" {
		fmt.Printf("Base commit %s has parents: %v\n", baseCommit, parents)
	}

	// Try to match the remote with the correct parent by checking tree content
	for i, parent := range parents {
		// Get the tree from this parent
		cmd = exec.Command("git", "rev-parse", parent+"^{tree}")
		output, err = cmd.Output()
		if err != nil {
			if os.Getenv("GIT_STITCH_VERBOSE") != "" {
				fmt.Printf("Warning: couldn't get tree for parent %s: %v\n", parent, err)
			}
			continue
		}
		parentTree := strings.TrimSpace(string(output))

		// Get the tree hash for this remote directory in the base commit
		if os.Getenv("GIT_STITCH_VERBOSE") != "" {
			wd, _ := os.Getwd()
			fmt.Printf("Running 'git rev-parse %s:%s' in directory %s\n", baseCommit, remote, wd)
		}
		cmd = exec.Command("git", "rev-parse", fmt.Sprintf("%s:%s", baseCommit, remote))
		output, err = cmd.Output()
		if err != nil {
			if os.Getenv("GIT_STITCH_VERBOSE") != "" {
				fmt.Printf("Warning: couldn't get tree for remote %s in base commit: %v\n", remote, err)
			}
			continue
		}
		remoteTree := strings.TrimSpace(string(output))
		if os.Getenv("GIT_STITCH_VERBOSE") != "" {
			fmt.Printf("Got tree hash for remote %s: %s\n", remote, remoteTree)
		}

		if os.Getenv("GIT_STITCH_VERBOSE") != "" {
			fmt.Printf("Comparing parent %d (%s) tree %s with remote %s tree %s - match: %t\n", i, parent, parentTree, remote, remoteTree, parentTree == remoteTree)
		}
		if parentTree == remoteTree {
			if os.Getenv("GIT_STITCH_VERBOSE") != "" {
				fmt.Printf("Found matching parent %s for remote %s (trees match: %s)\n", parent, remote, parentTree)
			}
			return parent, nil
		}
	}

	// Fallback: return the first parent (this assumes order is preserved)
	if os.Getenv("GIT_STITCH_VERBOSE") != "" {
		fmt.Printf("No exact match found for remote %s, using first parent %s\n", remote, parents[0])
	}
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

func getChangedFilesWithStatus(commitHash string) ([]FileChange, error) {
	cmd := exec.Command("git", "diff-tree", "--no-commit-id", "--name-status", "-r", commitHash)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var changes []FileChange
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 {
			changes = append(changes, FileChange{
				Status: parts[0],
				Path:   parts[1],
			})
		}
	}
	return changes, nil
}

func createCommitForRemote(commit CommitInfo, remote string, files []string, parentCommit string) (string, error) {
	// Much simpler approach: just apply the single file change to the parent tree
	if len(files) != 1 {
		return "", fmt.Errorf("expected exactly 1 file, got %d: %v", len(files), files)
	}
	file := files[0]
	monorepoPath := fmt.Sprintf("%s/%s", remote, file)

	// Get the file content from the monorepo commit
	cmd := exec.Command("git", "show", fmt.Sprintf("%s:%s", commit.Hash, monorepoPath))
	fileContent, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get file content for %s: %v", file, err)
	}

	// Create a blob for this file content
	cmd = exec.Command("git", "hash-object", "-w", "--stdin")
	cmd.Stdin = strings.NewReader(string(fileContent))
	blobOutput, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to create blob for %s: %v", file, err)
	}
	blobHash := strings.TrimSpace(string(blobOutput))

	// Get the file mode from the monorepo
	cmd = exec.Command("git", "ls-tree", commit.Hash, monorepoPath)
	modeOutput, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get mode for %s: %v", file, err)
	}
	parts := strings.Fields(strings.TrimSpace(string(modeOutput)))
	if len(parts) < 1 {
		return "", fmt.Errorf("invalid ls-tree output for %s", file)
	}
	mode := parts[0]

	// Get the parent tree
	cmd = exec.Command("git", "rev-parse", parentCommit+"^{tree}")
	parentTreeOutput, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get parent tree: %v", err)
	}
	parentTree := strings.TrimSpace(string(parentTreeOutput))

	// Read the parent tree and add our file
	cmd = exec.Command("git", "ls-tree", parentTree)
	treeOutput, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to read parent tree: %v", err)
	}

	// Build new tree entries: parent tree + our new file
	var treeEntries []string
	scanner := bufio.NewScanner(strings.NewReader(string(treeOutput)))
	fileExists := false
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 && parts[1] == file {
			// Replace existing file
			treeEntries = append(treeEntries, fmt.Sprintf("%s blob %s\t%s", mode, blobHash, file))
			fileExists = true
		} else {
			// Keep existing entry
			treeEntries = append(treeEntries, line)
		}
	}

	// Add new file if it didn't exist
	if !fileExists {
		treeEntries = append(treeEntries, fmt.Sprintf("%s blob %s\t%s", mode, blobHash, file))
	}

	// Create the new tree
	treeInput := strings.Join(treeEntries, "\n") + "\n"
	cmd = exec.Command("git", "mktree")
	cmd.Stdin = strings.NewReader(treeInput)
	newTreeOutput, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to create tree: %v", err)
	}
	newTree := strings.TrimSpace(string(newTreeOutput))

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

	commitOutput, err := cmd.CombinedOutput() // Use CombinedOutput to capture stderr
	if err != nil {
		return "", fmt.Errorf("failed to create commit-tree (parent: %s, tree: %s): %v, output: %s", parentCommit, newTree, err, string(commitOutput))
	}

	return strings.TrimSpace(string(commitOutput)), nil
}

func createCommitForRemoteWithChanges(commit CommitInfo, remote string, fileChanges []FileChange, parentCommit string) (string, error) {
	// For now, handle multiple changes by applying them one by one
	// This is simpler and more reliable than trying to build complex trees
	currentParent := parentCommit

	for _, change := range fileChanges {
		// Create a temporary single-file change and apply it
		newCommit, err := createCommitForRemoteSingleChange(commit, remote, change, currentParent)
		if err != nil {
			return "", fmt.Errorf("failed to apply change %s: %v", change.Path, err)
		}
		currentParent = newCommit
	}

	return currentParent, nil
}

func createCommitForRemoteSingleChange(commit CommitInfo, remote string, change FileChange, parentCommit string) (string, error) {
	filePath := change.Path
	monorepoPath := fmt.Sprintf("%s/%s", remote, filePath)

	// Use git's index to properly handle subdirectories
	// This is much more robust than trying to manually build trees

	// Create a temporary index file
	tmpDir := "/tmp"
	indexFile := filepath.Join(tmpDir, fmt.Sprintf("git-rip-index-%d", time.Now().UnixNano()))
	defer os.Remove(indexFile)

	// Read the parent tree into the index
	parentTree, err := exec.Command("git", "rev-parse", parentCommit+"^{tree}").Output()
	if err != nil {
		return "", fmt.Errorf("failed to get parent tree: %v", err)
	}
	parentTreeHash := strings.TrimSpace(string(parentTree))

	cmd := exec.Command("git", "read-tree", parentTreeHash)
	cmd.Env = append(os.Environ(), "GIT_INDEX_FILE="+indexFile)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to read parent tree into index: %v", err)
	}

	// Apply the change to the index
	switch change.Status {
	case "D": // Deletion
		cmd = exec.Command("git", "update-index", "--remove", filePath)
		cmd.Env = append(os.Environ(), "GIT_INDEX_FILE="+indexFile)
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("failed to remove file from index: %v", err)
		}
		if os.Getenv("GIT_STITCH_VERBOSE") != "" {
			fmt.Printf("Removed %s from index\n", filePath)
		}

	case "A", "M": // Addition or Modification
		// Get the blob hash from the monorepo
		blobHash, err := exec.Command("git", "rev-parse", fmt.Sprintf("%s:%s", commit.Hash, monorepoPath)).Output()
		if err != nil {
			return "", fmt.Errorf("failed to get blob hash for %s: %v", monorepoPath, err)
		}
		blobHashStr := strings.TrimSpace(string(blobHash))

		// Get the file mode from the monorepo
		modeOutput, err := exec.Command("git", "ls-tree", commit.Hash, monorepoPath).Output()
		if err != nil {
			return "", fmt.Errorf("failed to get mode for %s: %v", monorepoPath, err)
		}
		parts := strings.Fields(strings.TrimSpace(string(modeOutput)))
		if len(parts) < 1 {
			return "", fmt.Errorf("invalid ls-tree output for %s", monorepoPath)
		}
		mode := parts[0]

		// Add/update the file in the index
		cmd = exec.Command("git", "update-index", "--add", "--cacheinfo", mode, blobHashStr, filePath)
		cmd.Env = append(os.Environ(), "GIT_INDEX_FILE="+indexFile)
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("failed to update index for %s: %v", filePath, err)
		}
		if os.Getenv("GIT_STITCH_VERBOSE") != "" {
			fmt.Printf("Updated %s in index with mode %s and blob %s\n", filePath, mode, blobHashStr)
		}
	}

	// Write the tree from the index
	cmd = exec.Command("git", "write-tree")
	cmd.Env = append(os.Environ(), "GIT_INDEX_FILE="+indexFile)
	newTreeOutput, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to write tree from index: %v", err)
	}
	newTree := strings.TrimSpace(string(newTreeOutput))

	if os.Getenv("GIT_STITCH_VERBOSE") != "" {
		fmt.Printf("Created tree %s for change %s %s\n", newTree, change.Status, filePath)
	}

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

	commitOutput, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to create commit-tree (parent: %s, tree: %s): %v, output: %s", parentCommit, newTree, err, string(commitOutput))
	}

	return strings.TrimSpace(string(commitOutput)), nil
}

func createBlobAndGetMode(commitHash, monorepoPath string) (string, string, error) {
	// Get the file content from the monorepo commit
	cmd := exec.Command("git", "show", fmt.Sprintf("%s:%s", commitHash, monorepoPath))
	fileContent, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("failed to get file content for %s: %v", monorepoPath, err)
	}

	// Create a blob for this file content
	cmd = exec.Command("git", "hash-object", "-w", "--stdin")
	cmd.Stdin = strings.NewReader(string(fileContent))
	blobOutput, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("failed to create blob for %s: %v", monorepoPath, err)
	}
	blobHash := strings.TrimSpace(string(blobOutput))

	// Get the file mode from the monorepo
	cmd = exec.Command("git", "ls-tree", commitHash, monorepoPath)
	modeOutput, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("failed to get mode for %s: %v", monorepoPath, err)
	}
	parts := strings.Fields(strings.TrimSpace(string(modeOutput)))
	if len(parts) < 1 {
		return "", "", fmt.Errorf("invalid ls-tree output for %s", monorepoPath)
	}
	mode := parts[0]

	return blobHash, mode, nil
}
