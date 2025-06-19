# git-stitch & git-rip: tools for mono-repo'ing your multi-repo.

`git-stitch` combines two repos into one repo, so you (or a coding
agent or a code search tool) can operate on them as one. `git-rip`
takes any commits done on this combined repo, and splits
them up into commits that can be applied to the base repos.

## Usage Example

In this example, Romeo and Juliet are our two repos, alike in dignity.

```
$ git remote add romeo git@github.com:philz/romeo.git
$ git remote add juliet git@github.com:philz/juliet.git
$ git-stitch romeo/main juliet/main
Fetching romeo... romeo/main is aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
Fetching juliet... juliet/main is bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
Stitched romeo & juliet into cccccccccccccccccccccccccccccccccccccccc
To check out the new commit, run:
  git checkout -b mono cccccccccccccccccccccccccccccccccccccccc
Or to update your current branch:
  git reset cccccccccccccccccccccccccccccccccccccccc
$ echo "Caplet" >> juliet/house.txt
$ echo "Romeo" >> romeo/house.txt
$ git add juliet/house.txt romeo/house.txt
$ git commit -a -m'Adding house metadata.'
[main 63c67d03] Adding house metadata.
$ echo "Capulet" > juliet/house.txt
$ git commit -a -m'Fixing typo'
[main 77777777] Adding house metadata.
$ git-rip verona
Found base commit: cccccccccccccccccccccccccccccccccccccccc
Processing commit: 63c67d03
Created commit abcdef for romeo
Created commit abc123 for juliet
Processing commit: 7777777
Created commit bcde12 for romeo
Branches created:
  romeo-verona
  romeo-capulet
```

## Installation

```
go install github.com/philz/git-stitch/cmd/git-stitch github.com/philz/git-stitch/cmd/git-rip
```

## Usage

```
git-stitch [-no-fetch] ref1 [ref2...]

Creates a new commit which includes the tree of ref1 in a directory named
as the first component of ref1 when split by /, and the same for any additional
refs. Typically, refs might look like "remote/branch".

To help with determinism, the merge commit uses the same timestamps when
given the same refs (and they point to the same commits). The git author is
"git-stitch"
```

```
git-rip [prefix]
```

Splits any commits since the original merge into branches prefixed with prefix
and suffixed by the directory name. If no prefix is specified, "rip-<timestamp>" is used.

## Use cases

Tell me about yours. Mine are:

1. "git grep" across several repos.

2. Giving a single repo to a coding agent, which then needs to work across those repos to make some changes.

## How does this work?

See https://blog.philz.dev/blog/git-monorepo/

## What does this look like visually?

TODO: Insert mermaid diagrams here of the merged romeo and juliet from above
as well as the generated split commits.
