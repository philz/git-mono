package main

import (
	"fmt"
	"os"
)

// Build information (set via ldflags)
var (
	Version = "dev"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	switch command {
	case "init":
		if err := handleInit(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "rebase":
		if err := handleRebase(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "reset":
		if err := handleReset(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "explode":
		if err := handleExplode(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "version", "--version", "-v":
		printVersion()
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("git-stitch - A tool for managing monorepos from multiple repositories")
	fmt.Printf("Version: %s\n", Version)
	fmt.Println("")
	fmt.Println("Getting started:")
	fmt.Println("  git init")
	fmt.Println("  git remote add remote1 <url1>")
	fmt.Println("  git remote add remote2 <url2>")
	fmt.Println("  git-stitch init remote1 remote2")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  git-stitch init <remote1> <remote2> [<remote3> ...]")
	fmt.Println("    Creates a monorepo by pulling in trees from existing remotes (minimum 2)")
	fmt.Println("    Uses remote names as directory names and detects default branches")
	fmt.Println("")
	fmt.Println("  git-stitch rebase [<subdir1> <ref1> [<subdir2> <ref2> ...]]")
	fmt.Println("    Fetches remotes and creates new base commit, then rebases")
	fmt.Println("    If subdir/ref pairs specified, uses those instead of defaults")
	fmt.Println("")
	fmt.Println("  git-stitch reset <subdir> <ref>")
	fmt.Println("    Creates a monorepo commit with subdir pointed to specific ref")
	fmt.Println("")
	fmt.Println("  git-stitch explode")
	fmt.Println("    Replays monorepo commits back to individual remotes")
	fmt.Println("")
	fmt.Println("  git-stitch version")
	fmt.Println("    Show version information")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  git-stitch init remote1 remote2")
	fmt.Println("  git-stitch rebase")
	fmt.Println("  git-stitch rebase subdir1 feature-branch")
	fmt.Println("  git-stitch reset subdir1 v2.0.0")
	fmt.Println("  git-stitch explode")
}

func printVersion() {
	fmt.Printf("git-stitch version %s\n", Version)
}
