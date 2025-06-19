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
