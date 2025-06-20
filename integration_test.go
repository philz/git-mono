package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestIntegration runs comprehensive end-to-end tests
func TestIntegration(t *testing.T) {
	testDir := t.TempDir()

	// Build the tools first
	buildTools(t)

	t.Run("BasicMergeAndSplit", func(t *testing.T) {
		testBasicMergeAndSplit(t, testDir)
	})

	t.Run("FileOperations", func(t *testing.T) {
		testFileOperations(t, testDir)
	})

	t.Run("READMEFlow", func(t *testing.T) {
		testREADMEFlow(t, testDir)
	})

	t.Run("DeterministicBehavior", func(t *testing.T) {
		testDeterministicBehavior(t, testDir)
	})

	t.Run("MultipleCommitsStacking", func(t *testing.T) {
		testMultipleCommitsStacking(t, testDir)
	})

	t.Run("SubdirectoryOperations", func(t *testing.T) {
		testSubdirectoryOperations(t, testDir)
	})
}

func buildTools(t *testing.T) {
	cmd := exec.Command("go", "build", "-o", "git-stitch", "./cmd/git-stitch")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to build git-stitch: %v", err)
	}

	cmd = exec.Command("go", "build", "-o", "git-rip", "./cmd/git-rip")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to build git-rip: %v", err)
	}
}

func testBasicMergeAndSplit(t *testing.T, baseDir string) {
	testDir := filepath.Join(baseDir, "basic")
	os.MkdirAll(testDir, 0755)

	// Create two test repositories
	repo1Dir := filepath.Join(testDir, "repo1")
	repo2Dir := filepath.Join(testDir, "repo2")
	monoDir := filepath.Join(testDir, "mono")

	createTestRepo(t, repo1Dir, "repo1", []TestCommit{
		{Message: "Initial commit", Files: map[string]string{"README.md": "# Repo 1"}},
		{Message: "Add feature", Files: map[string]string{"feature.txt": "feature1"}},
	})

	createTestRepo(t, repo2Dir, "repo2", []TestCommit{
		{Message: "Initial commit", Files: map[string]string{"README.md": "# Repo 2"}},
		{Message: "Add config", Files: map[string]string{"config.json": `{"name": "repo2"}`}},
	})

	// Create mono repo and add remotes
	setupMonoRepo(t, monoDir, map[string]string{
		"repo1": repo1Dir,
		"repo2": repo2Dir,
	})

	// Test git-stitch
	stitchOutput := runGitStitch(t, monoDir, "repo1/master", "repo2/master")
	if !strings.Contains(stitchOutput, "Stitched") {
		t.Errorf("Expected stitch output to contain 'Stitched', got: %s", stitchOutput)
	}

	// Extract commit hash and checkout
	lines := strings.Split(stitchOutput, "\n")
	var commitHash string
	for _, line := range lines {
		if strings.Contains(line, "Stitched") {
			parts := strings.Fields(line)
			commitHash = parts[len(parts)-1]
			break
		}
	}

	checkoutCommit(t, monoDir, "mono", commitHash)

	// Verify structure
	verifyFileExists(t, filepath.Join(monoDir, "repo1", "README.md"))
	verifyFileExists(t, filepath.Join(monoDir, "repo1", "feature.txt"))
	verifyFileExists(t, filepath.Join(monoDir, "repo2", "README.md"))
	verifyFileExists(t, filepath.Join(monoDir, "repo2", "config.json"))

	// Make changes in monorepo
	writeFile(t, filepath.Join(monoDir, "repo1", "new_feature.txt"), "new feature")
	writeFile(t, filepath.Join(monoDir, "repo2", "settings.txt"), "settings")
	commitChanges(t, monoDir, "Add new features")

	// Test git-rip
	ripOutput := runGitRip(t, monoDir, "test")
	if !strings.Contains(ripOutput, "Branches created:") {
		t.Errorf("Expected rip output to contain 'Branches created:', got: %s", ripOutput)
	}

	// Verify branches exist
	verifyBranchExists(t, monoDir, "test-repo1")
	verifyBranchExists(t, monoDir, "test-repo2")

	// Check that the split branches have the correct files
	checkoutBranch(t, monoDir, "test-repo1")
	verifyFileExists(t, filepath.Join(monoDir, "new_feature.txt"))
	verifyFileNotExists(t, filepath.Join(monoDir, "settings.txt"))

	checkoutBranch(t, monoDir, "test-repo2")
	verifyFileExists(t, filepath.Join(monoDir, "settings.txt"))
	verifyFileNotExists(t, filepath.Join(monoDir, "new_feature.txt"))
}

func testFileOperations(t *testing.T, baseDir string) {
	testDir := filepath.Join(baseDir, "fileops")
	os.MkdirAll(testDir, 0755)

	repo1Dir := filepath.Join(testDir, "repo1")
	repo2Dir := filepath.Join(testDir, "repo2")
	monoDir := filepath.Join(testDir, "mono")

	createTestRepo(t, repo1Dir, "repo1", []TestCommit{
		{Message: "Initial commit", Files: map[string]string{"file1.txt": "content1", "file2.txt": "content2"}},
	})

	createTestRepo(t, repo2Dir, "repo2", []TestCommit{
		{Message: "Initial commit", Files: map[string]string{"fileA.txt": "contentA", "fileB.txt": "contentB"}},
	})

	setupMonoRepo(t, monoDir, map[string]string{
		"repo1": repo1Dir,
		"repo2": repo2Dir,
	})

	// Stitch repos
	stitchOutput := runGitStitch(t, monoDir, "repo1/master", "repo2/master")
	lines := strings.Split(stitchOutput, "\n")
	var commitHash string
	for _, line := range lines {
		if strings.Contains(line, "Stitched") {
			parts := strings.Fields(line)
			commitHash = parts[len(parts)-1]
			break
		}
	}
	checkoutCommit(t, monoDir, "mono", commitHash)

	// Test file deletion
	deleteFile(t, monoDir, "repo1/file1.txt")
	commitChanges(t, monoDir, "Delete file1.txt")

	// Test file rename/move
	moveFile(t, monoDir, "repo2/fileA.txt", "repo2/renamedA.txt")
	commitChanges(t, monoDir, "Rename fileA.txt")

	// Test modification
	writeFile(t, filepath.Join(monoDir, "repo1", "file2.txt"), "modified content2")
	commitChanges(t, monoDir, "Modify file2.txt")

	// Test adding new file
	writeFile(t, filepath.Join(monoDir, "repo2", "newfile.txt"), "new content")
	commitChanges(t, monoDir, "Add newfile.txt")

	// Rip the changes
	ripOutput := runGitRip(t, monoDir, "filetest")
	if !strings.Contains(ripOutput, "Branches created:") {
		t.Errorf("Expected rip output to contain 'Branches created:', got: %s", ripOutput)
	}

	// Verify repo1 branch
	checkoutBranch(t, monoDir, "filetest-repo1")
	verifyFileNotExists(t, filepath.Join(monoDir, "file1.txt"))                    // deleted
	verifyFileContent(t, filepath.Join(monoDir, "file2.txt"), "modified content2") // modified

	// Verify repo2 branch
	checkoutBranch(t, monoDir, "filetest-repo2")
	verifyFileExists(t, filepath.Join(monoDir, "renamedA.txt")) // renamed
	verifyFileNotExists(t, filepath.Join(monoDir, "fileA.txt")) // old name should not exist
	verifyFileExists(t, filepath.Join(monoDir, "newfile.txt"))  // added
	verifyFileContent(t, filepath.Join(monoDir, "newfile.txt"), "new content")
}

func testREADMEFlow(t *testing.T, baseDir string) {
	testDir := filepath.Join(baseDir, "readme")
	os.MkdirAll(testDir, 0755)

	monoDir := filepath.Join(testDir, "mono")
	setupMonoRepo(t, monoDir, map[string]string{
		"romeo":  "https://github.com/philz/romeo.git",
		"juliet": "https://github.com/philz/juliet.git",
	})

	// Follow the exact README flow
	stitchOutput := runGitStitch(t, monoDir, "romeo/main", "juliet/main")
	lines := strings.Split(stitchOutput, "\n")
	var commitHash string
	for _, line := range lines {
		if strings.Contains(line, "Stitched") {
			parts := strings.Fields(line)
			commitHash = parts[len(parts)-1]
			break
		}
	}

	// Store the actual commit hash for README update
	fmt.Printf("README Flow: git-stitch created commit %s\n", commitHash)

	checkoutCommit(t, monoDir, "mono", commitHash)

	// Add house metadata as per README
	writeFile(t, filepath.Join(monoDir, "juliet", "house.txt"), "Caplet")
	writeFile(t, filepath.Join(monoDir, "romeo", "house.txt"), "Romeo")
	commitChanges(t, monoDir, "Adding house metadata.")

	// Fix typo as per README
	writeFile(t, filepath.Join(monoDir, "juliet", "house.txt"), "Capulet")
	commitChanges(t, monoDir, "Fixing typo")

	// Rip as per README
	ripOutput := runGitRip(t, monoDir, "verona")
	if !strings.Contains(ripOutput, "verona-juliet") {
		t.Errorf("Expected rip output to contain 'verona-juliet', got: %s", ripOutput)
	}
	if !strings.Contains(ripOutput, "verona-romeo") {
		t.Errorf("Expected rip output to contain 'verona-romeo', got: %s", ripOutput)
	}

	// Verify the branches have the correct content
	checkoutBranch(t, monoDir, "verona-juliet")
	verifyFileContent(t, filepath.Join(monoDir, "house.txt"), "Capulet")

	checkoutBranch(t, monoDir, "verona-romeo")
	verifyFileContent(t, filepath.Join(monoDir, "house.txt"), "Romeo")

	fmt.Printf("README Flow completed successfully with base commit: %s\n", commitHash)
}

func testDeterministicBehavior(t *testing.T, baseDir string) {
	testDir := filepath.Join(baseDir, "deterministic")
	os.MkdirAll(testDir, 0755)

	repo1Dir := filepath.Join(testDir, "repo1")
	repo2Dir := filepath.Join(testDir, "repo2")
	monoDir1 := filepath.Join(testDir, "mono1")
	monoDir2 := filepath.Join(testDir, "mono2")

	createTestRepo(t, repo1Dir, "repo1", []TestCommit{
		{Message: "Initial commit", Files: map[string]string{"README.md": "# Repo 1"}},
	})

	createTestRepo(t, repo2Dir, "repo2", []TestCommit{
		{Message: "Initial commit", Files: map[string]string{"README.md": "# Repo 2"}},
	})

	// Setup two identical mono repos
	setupMonoRepo(t, monoDir1, map[string]string{
		"repo1": repo1Dir,
		"repo2": repo2Dir,
	})
	setupMonoRepo(t, monoDir2, map[string]string{
		"repo1": repo1Dir,
		"repo2": repo2Dir,
	})

	// Stitch both with same parameters
	output1 := runGitStitch(t, monoDir1, "-no-fetch", "repo1/master", "repo2/master")
	output2 := runGitStitch(t, monoDir2, "-no-fetch", "repo1/master", "repo2/master")

	// Extract commit hashes
	hash1 := extractCommitHash(output1)
	hash2 := extractCommitHash(output2)

	if hash1 != hash2 {
		t.Errorf("git-stitch is not deterministic: got different hashes %s vs %s", hash1, hash2)
	}

	fmt.Printf("Deterministic test passed: both runs produced commit %s\n", hash1)
}

type TestCommit struct {
	Message string
	Files   map[string]string
}

func createTestRepo(t *testing.T, dir, name string, commits []TestCommit) {
	os.MkdirAll(dir, 0755)

	// Initialize repo
	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "config", "user.name", "Test User")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")

	// Create commits
	for _, commit := range commits {
		for path, content := range commit.Files {
			fullPath := filepath.Join(dir, path)
			os.MkdirAll(filepath.Dir(fullPath), 0755)
			writeFile(t, fullPath, content)
		}
		runGitCmd(t, dir, "add", ".")
		runGitCmd(t, dir, "commit", "-m", commit.Message)
	}
}

func setupMonoRepo(t *testing.T, dir string, remotes map[string]string) {
	os.MkdirAll(dir, 0755)
	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "config", "user.name", "Test User")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")

	for name, url := range remotes {
		runGitCmd(t, dir, "remote", "add", name, url)
		if !strings.HasPrefix(url, "http") {
			// Local repo, fetch it
			runGitCmd(t, dir, "fetch", name)
		}
	}
}

func runGitStitch(t *testing.T, dir string, args ...string) string {
	// Get absolute path to git-stitch binary
	wd, _ := os.Getwd()
	binaryPath := filepath.Join(wd, "git-stitch")
	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git-stitch failed: %v, output: %s", err, output)
	}
	return string(output)
}

func runGitRip(t *testing.T, dir string, args ...string) string {
	// Get absolute path to git-rip binary
	wd, _ := os.Getwd()
	binaryPath := filepath.Join(wd, "git-rip")
	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git-rip failed: %v, output: %s", err, output)
	}
	return string(output)
}

func runGitCmd(t *testing.T, dir string, args ...string) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v failed: %v", args, err)
	}
}

func checkoutCommit(t *testing.T, dir, branch, commit string) {
	runGitCmd(t, dir, "checkout", "-b", branch, commit)
}

func checkoutBranch(t *testing.T, dir, branch string) {
	runGitCmd(t, dir, "checkout", branch)
}

func commitChanges(t *testing.T, dir, message string) {
	runGitCmd(t, dir, "add", ".")
	runGitCmd(t, dir, "commit", "-m", message)
}

func writeFile(t *testing.T, path, content string) {
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write file %s: %v", path, err)
	}
}

func deleteFile(t *testing.T, dir, relPath string) {
	runGitCmd(t, dir, "rm", relPath)
}

func moveFile(t *testing.T, dir, oldPath, newPath string) {
	runGitCmd(t, dir, "mv", oldPath, newPath)
}

func verifyFileExists(t *testing.T, path string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("Expected file %s to exist", path)
	}
}

func verifyFileNotExists(t *testing.T, path string) {
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("Expected file %s to not exist", path)
	}
}

func verifyFileContent(t *testing.T, path, expected string) {
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", path, err)
	}
	if strings.TrimSpace(string(content)) != expected {
		t.Errorf("File %s content mismatch. Expected: %s, Got: %s", path, expected, strings.TrimSpace(string(content)))
	}
}

func verifyBranchExists(t *testing.T, dir, branch string) {
	cmd := exec.Command("git", "branch", "--list", branch)
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil || !strings.Contains(string(output), branch) {
		t.Errorf("Expected branch %s to exist", branch)
	}
}

func extractCommitHash(output string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Stitched") {
			parts := strings.Fields(line)
			return parts[len(parts)-1]
		}
	}
	return ""
}

func testMultipleCommitsStacking(t *testing.T, baseDir string) {
	testDir := filepath.Join(baseDir, "stacking")
	os.MkdirAll(testDir, 0755)

	repo1Dir := filepath.Join(testDir, "repo1")
	repo2Dir := filepath.Join(testDir, "repo2")
	monoDir := filepath.Join(testDir, "mono")

	// Create two test repositories with initial commits
	createTestRepo(t, repo1Dir, "repo1", []TestCommit{
		{Message: "Initial commit", Files: map[string]string{"README.md": "# Repo 1", "file1.txt": "initial content"}},
	})

	createTestRepo(t, repo2Dir, "repo2", []TestCommit{
		{Message: "Initial commit", Files: map[string]string{"README.md": "# Repo 2", "file2.txt": "initial content"}},
	})

	// Create mono repo and add remotes
	setupMonoRepo(t, monoDir, map[string]string{
		"repo1": repo1Dir,
		"repo2": repo2Dir,
	})

	// Test git-stitch
	stitchOutput := runGitStitch(t, monoDir, "repo1/master", "repo2/master")
	commitHash := extractCommitHash(stitchOutput)
	checkoutCommit(t, monoDir, "mono", commitHash)

	// Make multiple commits in the monorepo
	// Commit 1: Both repos get changes
	writeFile(t, filepath.Join(monoDir, "repo1", "change1.txt"), "change 1 for repo1")
	writeFile(t, filepath.Join(monoDir, "repo2", "change1.txt"), "change 1 for repo2")
	commitChanges(t, monoDir, "First change to both repos")

	// Commit 2: Only repo1 gets changes
	writeFile(t, filepath.Join(monoDir, "repo1", "change2.txt"), "change 2 for repo1")
	commitChanges(t, monoDir, "Second change to repo1 only")

	// Commit 3: Only repo2 gets changes
	writeFile(t, filepath.Join(monoDir, "repo2", "change2.txt"), "change 2 for repo2")
	commitChanges(t, monoDir, "Second change to repo2 only")

	// Commit 4: Both repos get changes again
	writeFile(t, filepath.Join(monoDir, "repo1", "change3.txt"), "change 3 for repo1")
	writeFile(t, filepath.Join(monoDir, "repo2", "change3.txt"), "change 3 for repo2")
	commitChanges(t, monoDir, "Third change to both repos")

	// Check that we have the expected commits in the monorepo
	monolog := getGitLog(t, monoDir, "--oneline")
	monologLines := strings.Split(strings.TrimSpace(monolog), "\n")
	if len(monologLines) < 5 { // base commit + 4 new commits
		t.Errorf("Expected at least 5 commits in monorepo, got %d", len(monologLines))
	}

	// Run git-rip
	ripOutput := runGitRip(t, monoDir, "stacking")
	if !strings.Contains(ripOutput, "stacking-repo1") {
		t.Errorf("Expected rip output to contain 'stacking-repo1', got: %s", ripOutput)
	}
	if !strings.Contains(ripOutput, "stacking-repo2") {
		t.Errorf("Expected rip output to contain 'stacking-repo2', got: %s", ripOutput)
	}

	// Verify repo1 branch has the correct commits and files
	checkoutBranch(t, monoDir, "stacking-repo1")
	repo1log := getGitLog(t, monoDir, "--oneline")
	repo1logLines := strings.Split(strings.TrimSpace(repo1log), "\n")

	// Should have: initial + commit1 + commit2 + commit4 (repo1 was changed in these commits)
	expectedRepo1Commits := 4
	if len(repo1logLines) != expectedRepo1Commits {
		t.Errorf("Expected %d commits in repo1 branch, got %d: %v", expectedRepo1Commits, len(repo1logLines), repo1logLines)
	}

	// Verify files exist
	verifyFileExists(t, filepath.Join(monoDir, "README.md"))
	verifyFileExists(t, filepath.Join(monoDir, "file1.txt"))
	verifyFileExists(t, filepath.Join(monoDir, "change1.txt"))
	verifyFileExists(t, filepath.Join(monoDir, "change2.txt"))
	verifyFileExists(t, filepath.Join(monoDir, "change3.txt"))
	// Should NOT have repo2 files
	verifyFileNotExists(t, filepath.Join(monoDir, "file2.txt"))

	// Verify repo2 branch has the correct commits and files
	checkoutBranch(t, monoDir, "stacking-repo2")
	repo2log := getGitLog(t, monoDir, "--oneline")
	repo2logLines := strings.Split(strings.TrimSpace(repo2log), "\n")

	// Should have: initial + commit1 + commit3 + commit4 (repo2 was changed in these commits)
	expectedRepo2Commits := 4
	if len(repo2logLines) != expectedRepo2Commits {
		t.Errorf("Expected %d commits in repo2 branch, got %d: %v", expectedRepo2Commits, len(repo2logLines), repo2logLines)
	}

	// Verify files exist
	verifyFileExists(t, filepath.Join(monoDir, "README.md"))
	verifyFileExists(t, filepath.Join(monoDir, "file2.txt"))
	verifyFileExists(t, filepath.Join(monoDir, "change1.txt"))
	verifyFileExists(t, filepath.Join(monoDir, "change2.txt"))
	verifyFileExists(t, filepath.Join(monoDir, "change3.txt"))
	// Should NOT have repo1 files
	verifyFileNotExists(t, filepath.Join(monoDir, "file1.txt"))

	// Verify commit messages are preserved
	if !strings.Contains(repo1log, "First change to both repos") {
		t.Errorf("Expected repo1 log to contain 'First change to both repos'")
	}
	if !strings.Contains(repo1log, "Second change to repo1 only") {
		t.Errorf("Expected repo1 log to contain 'Second change to repo1 only'")
	}
	if !strings.Contains(repo1log, "Third change to both repos") {
		t.Errorf("Expected repo1 log to contain 'Third change to both repos'")
	}
	// Should NOT contain repo2-only commit
	if strings.Contains(repo1log, "Second change to repo2 only") {
		t.Errorf("repo1 log should not contain 'Second change to repo2 only'")
	}

	if !strings.Contains(repo2log, "First change to both repos") {
		t.Errorf("Expected repo2 log to contain 'First change to both repos'")
	}
	if !strings.Contains(repo2log, "Second change to repo2 only") {
		t.Errorf("Expected repo2 log to contain 'Second change to repo2 only'")
	}
	if !strings.Contains(repo2log, "Third change to both repos") {
		t.Errorf("Expected repo2 log to contain 'Third change to both repos'")
	}
	// Should NOT contain repo1-only commit
	if strings.Contains(repo2log, "Second change to repo1 only") {
		t.Errorf("repo2 log should not contain 'Second change to repo1 only'")
	}

	fmt.Printf("Multiple commits stacking test passed!\n")
	fmt.Printf("Repo1 commits: %d\n", len(repo1logLines))
	fmt.Printf("Repo2 commits: %d\n", len(repo2logLines))
}

func getGitLog(t *testing.T, dir string, args ...string) string {
	cmdArgs := append([]string{"log"}, args...)
	cmd := exec.Command("git", cmdArgs...)
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log failed: %v", err)
	}
	return string(output)
}

func testSubdirectoryOperations(t *testing.T, baseDir string) {
	testDir := filepath.Join(baseDir, "subdirs")
	os.MkdirAll(testDir, 0755)

	// Create repo1 with subdirectory structure
	repo1Dir := filepath.Join(testDir, "repo1")
	createTestRepo(t, repo1Dir, "repo1", []TestCommit{
		{Message: "Initial structure with subdirectories", Files: map[string]string{
			"README.md":       "# Repo1",
			"src/main/app.go": "package main\nfunc main() {}",
			"src/utils.go":    "package src\nfunc Helper() {}",
			"docs/api.md":     "# API Documentation",
		}},
	})

	// Create repo2 with different structure
	repo2Dir := filepath.Join(testDir, "repo2")
	createTestRepo(t, repo2Dir, "repo2", []TestCommit{
		{Message: "Initial JS structure", Files: map[string]string{
			"index.js":      "console.log('hello');",
			"lib/helper.js": "module.exports = {};",
		}},
	})

	// Create monorepo directory
	monoDir := filepath.Join(testDir, "mono")
	setupMonoRepo(t, monoDir, map[string]string{
		"repo1": repo1Dir,
		"repo2": repo2Dir,
	})

	// Stitch repos together
	stitchOutput := runGitStitch(t, monoDir, "repo1/master", "repo2/master")
	commitHash := extractCommitHash(stitchOutput)
	if commitHash == "" {
		t.Fatalf("Failed to extract commit hash from stitch output: %s", stitchOutput)
	}
	checkoutCommit(t, monoDir, "mono", commitHash)

	// Verify subdirectory structure is preserved
	verifyFileContent(t, filepath.Join(monoDir, "repo1", "README.md"), "# Repo1")
	verifyFileContent(t, filepath.Join(monoDir, "repo1", "src", "main", "app.go"), "package main\nfunc main() {}")
	verifyFileContent(t, filepath.Join(monoDir, "repo1", "src", "utils.go"), "package src\nfunc Helper() {}")
	verifyFileContent(t, filepath.Join(monoDir, "repo1", "docs", "api.md"), "# API Documentation")
	verifyFileContent(t, filepath.Join(monoDir, "repo2", "index.js"), "console.log('hello');")
	verifyFileContent(t, filepath.Join(monoDir, "repo2", "lib", "helper.js"), "module.exports = {};")

	// Make changes to files in subdirectories
	writeFile(t, filepath.Join(monoDir, "repo1", "src", "main", "app.go"), "package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"Hello\") }")
	commitChanges(t, monoDir, "Update app.go with imports")

	writeFile(t, filepath.Join(monoDir, "repo1", "docs", "api.md"), "# API Documentation\n\n## Overview\nThis is the API.")
	commitChanges(t, monoDir, "Expand API documentation")

	// Add new file in subdirectory
	writeFile(t, filepath.Join(monoDir, "repo2", "lib", "config.js"), "module.exports = { debug: true };")
	commitChanges(t, monoDir, "Add config file")

	// Delete a file in subdirectory
	deleteFile(t, monoDir, "repo1/src/utils.go")
	commitChanges(t, monoDir, "Remove utils.go")

	// Move/rename file in subdirectory
	moveFile(t, monoDir, "repo2/index.js", "repo2/main.js")
	commitChanges(t, monoDir, "Rename index.js to main.js")

	// Rip the changes back
	ripOutput := runGitRip(t, monoDir, "subdir-test")
	if !strings.Contains(ripOutput, "Branches created:") {
		t.Errorf("Expected rip output to contain 'Branches created:', got: %s", ripOutput)
	}

	// Verify repo1 branch has correct subdirectory changes
	checkoutBranch(t, monoDir, "subdir-test-repo1")
	verifyFileContent(t, filepath.Join(monoDir, "README.md"), "# Repo1")
	verifyFileContent(t, filepath.Join(monoDir, "src", "main", "app.go"), "package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"Hello\") }")
	verifyFileContent(t, filepath.Join(monoDir, "docs", "api.md"), "# API Documentation\n\n## Overview\nThis is the API.")
	verifyFileNotExists(t, filepath.Join(monoDir, "src", "utils.go")) // deleted

	// Verify repo2 branch has correct subdirectory changes
	checkoutBranch(t, monoDir, "subdir-test-repo2")
	verifyFileContent(t, filepath.Join(monoDir, "main.js"), "console.log('hello');") // renamed
	verifyFileNotExists(t, filepath.Join(monoDir, "index.js"))                       // old name
	verifyFileContent(t, filepath.Join(monoDir, "lib", "helper.js"), "module.exports = {};")
	verifyFileContent(t, filepath.Join(monoDir, "lib", "config.js"), "module.exports = { debug: true };") // added

	t.Logf("Subdirectory operations test passed!")
}
