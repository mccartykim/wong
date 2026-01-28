package vcs

import (
	"os"
	"path/filepath"
)

// detectVCSType detects the VCS type for the given path.
// It prefers jj over git in colocated repositories.
func detectVCSType(path string) (VCSType, error) {
	// Start from the given path and walk up to find VCS directories
	absPath, err := filepath.Abs(path)
	if err != nil {
		return VCSTypeUnknown, err
	}

	current := absPath
	for {
		jjPath := filepath.Join(current, ".jj")
		gitPath := filepath.Join(current, ".git")

		hasJJ := isDirectory(jjPath)
		hasGit := isDirectoryOrFile(gitPath) // .git can be a file for worktrees

		// Prefer jj in colocated repos (user chose jj-first workflow)
		if hasJJ {
			return VCSTypeJujutsu, nil
		}

		if hasGit {
			return VCSTypeGit, nil
		}

		// Move to parent directory
		parent := filepath.Dir(current)
		if parent == current {
			// Reached root without finding VCS
			break
		}
		current = parent
	}

	return VCSTypeUnknown, ErrNoVCSFound
}

// FindRepoRoot finds the repository root for the given path.
func FindRepoRoot(path string) (string, VCSType, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", VCSTypeUnknown, err
	}

	current := absPath
	for {
		jjPath := filepath.Join(current, ".jj")
		gitPath := filepath.Join(current, ".git")

		hasJJ := isDirectory(jjPath)
		hasGit := isDirectoryOrFile(gitPath)

		// Prefer jj
		if hasJJ {
			return current, VCSTypeJujutsu, nil
		}

		if hasGit {
			return current, VCSTypeGit, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	return "", VCSTypeUnknown, ErrNoVCSFound
}

// IsColocatedRepo checks if a path has both .jj and .git directories.
func IsColocatedRepo(path string) (bool, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false, err
	}

	current := absPath
	for {
		jjPath := filepath.Join(current, ".jj")
		gitPath := filepath.Join(current, ".git")

		hasJJ := isDirectory(jjPath)
		hasGit := isDirectoryOrFile(gitPath)

		if hasJJ && hasGit {
			return true, nil
		}

		// If we found one but not the other, not colocated
		if hasJJ || hasGit {
			return false, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	return false, ErrNoVCSFound
}

// isDirectory checks if path exists and is a directory.
func isDirectory(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// isDirectoryOrFile checks if path exists (as file or directory).
// This is needed for .git which can be a file pointing to the real git dir (worktrees).
func isDirectoryOrFile(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// GetJJRoot finds the .jj directory root.
func GetJJRoot(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	current := absPath
	for {
		jjPath := filepath.Join(current, ".jj")
		if isDirectory(jjPath) {
			return current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	return "", ErrNoVCSFound
}

// GetGitRoot finds the .git directory root.
func GetGitRoot(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	current := absPath
	for {
		gitPath := filepath.Join(current, ".git")
		if isDirectoryOrFile(gitPath) {
			return current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	return "", ErrNoVCSFound
}
