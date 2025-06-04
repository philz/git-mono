
## Design

`git-stitch` uses Git's plumbing commands to create synthetic commits that preserve the complete history of multiple repositories while presenting them as a unified workspace.

Key design principles:

- **History Preservation**: All commits maintain proper parent relationships, ensuring no git history is lost
- **Tree-based Operations**: Uses Git tree objects to efficiently combine subdirectories from different repos
- **Merge Semantics**: Creates proper merge commits with multiple parents when combining or updating repos
- **Configuration Storage**: Stores monorepo configuration directly in Git config within existing remote sections

## Intended Workflow

`git-stitch` is designed to support a specific workflow for coordinating changes across multiple repositories:

1. **Initialize** with your constituent remotes, creating a commit that combines the remotes, and check it out.
2. **Do work** that spans repos, creating commits as you go along. Commits can be mixed across the subdirs as necessary. Commits could be coming from coding agents.
3. **If necessary, rebase** onto a new combination, since the remotes have had their upstream move.
4. **Explode the commits** into commits onto the upstreams.
5. **Individually push** the exploded commits.

This workflow enables:
- Cross-repository refactoring and coordinated changes
- Automated tooling that works across repository boundaries  
- Preservation of logical commit boundaries when pushing back to individual repos
- Clean integration with existing Git workflows and tooling

## Implementation

The tool implements four main operations:

### Init
Creates the initial monorepo commit by:
1. Fetching specified remotes and detecting their default branches
2. Building a synthetic tree with each remote's content in its own subdirectory
3. Creating a merge commit with all remote HEAD commits as parents
4. Storing configuration in `remote.<name>.mono-*` git config keys

### Rebase
Updates the monorepo base by:
1. Fetching latest changes from all or specified remotes
2. Creating a new synthetic tree with updated content
3. Creating a new base commit with the previous base as parent
4. Rebasing any commits on top of the new base

### Reset
Points a subdirectory to a specific ref by:
1. Fetching the target ref from the appropriate remote
2. Creating a new tree with the subdirectory pointing to the target ref
3. Creating a merge commit with both current HEAD and target ref as parents
4. Preserving complete git history from both branches

### Explode
Replays monorepo commits back to individual remotes by:
1. Identifying commits since the last base commit
2. Extracting changes for each subdirectory
3. Creating corresponding commits on remote tracking branches
4. Preserving original commit messages

## Usage

### Getting Started

```bash
# Set up a new monorepo
git init
git remote add repo1 https://github.com/user/repo1.git
git remote add repo2 https://github.com/user/repo2.git
git-stitch init repo1 repo2

# Check out the monorepo
git checkout -b main <commit-hash>
```

### Basic Workflow

```bash
# Update all repos to latest
git-stitch rebase

# Work on your changes across both repos
echo "shared config" > repo1/config.json
echo "import '../repo1/config.json'" >> repo2/main.js
git add .
git commit -m "Add shared configuration"

# Push changes back to individual repos
git-stitch explode
# Then manually push the updated remote branches
```

### Advanced Operations

```bash
# Update only specific subdirectories
git-stitch rebase repo1 feature-branch repo2 v2.0.0

# Reset a subdirectory to a specific version
git-stitch reset repo1 v1.5.0

# Check tool version
git-stitch version
```

### Commands Reference

#### `git-stitch init <remote1> <remote2> [<remote3> ...]`
Creates initial monorepo from existing remotes (minimum 2 required).
- Uses remote names as subdirectory names
- Auto-detects default branches
- Preserves complete history from all repos

#### `git-stitch rebase [<subdir1> <ref1> [<subdir2> <ref2> ...]]`
Updates monorepo base with latest changes.
- Without arguments: updates all subdirs to their default branches
- With subdir/ref pairs: updates only specified subdirs to specified refs
- Automatically rebases any commits on top of new base

#### `git-stitch reset <subdir> <ref>`
Resets a subdirectory to point to a specific ref.
- Creates merge commit preserving history from both current state and target ref
- Useful for upgrading/downgrading individual components

#### `git-stitch explode`
Replays monorepo commits back to individual remote tracking branches.
- Processes all commits since last base commit
- Preserves original commit messages
- Updates remote tracking branches (manual push required)

#### `git-stitch version`
Shows version information including build details.

### Configuration

Monorepo configuration is stored in git config:

```bash
# Global settings
mono.remotes=repo1 repo2
mono.init-commit=<hash>

# Per-remote settings  
remote.repo1.mono-branch=main
remote.repo1.mono-subdir=.
remote.repo1.mono-dir=repo1
```

### Tips

- **Branch Management**: Work on a dedicated monorepo branch to keep it separate from individual repo work
- **Commit Hygiene**: Make logical commits that span the repos meaningfully
- **Expansion**: Run `git-stitch explode` before major updates to sync changes back
- **Conflicts**: Handle merge conflicts manually when rebasing across repo updates
- **History**: Use `git log --graph` to visualize the preserved multi-repo history

### Example: Cross-Repo Refactoring

```bash
# Set up monorepo with frontend and backend
git-stitch init frontend backend
git checkout -b refactor <monorepo-commit>

# Make coordinated changes
mv frontend/api/types.ts shared/types.ts
echo "export * from '../shared/types.js'" > backend/src/types.ts
echo "export * from '../shared/types.js'" > frontend/src/types.ts
git add .
git commit -m "Extract shared types to common location"

# Update backend to handle new API
sed -i 's/old_endpoint/new_endpoint/g' backend/src/api.py
sed -i 's/old_endpoint/new_endpoint/g' frontend/src/api.js
git add .
git commit -m "Update API endpoints consistently"

# Push changes back to individual repos
git-stitch explode
git push frontend refs/remotes/frontend/main:main
git push backend refs/remotes/backend/main:main
```

This workflow enables coordinated changes across multiple repositories while maintaining proper git history and the ability to work on repos individually.