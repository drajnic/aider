package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	InstallDirName         = ".gh_aider"                          // install root under user's home
	DefaultRepoURL         = "https://github.com/drajnic/aider"   // repository to clone
	RequiredPythonVersion  = "3.12"                               // Python version for uv venv
	RepoRef                = "feature/custom-litellm"			  // branch, tag, or commit SHA; empty = repo default
)

func cmdExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	return cmd.Run()
}

func runInDir(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	return cmd.Run()
}

func runWithEnv(env []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), env...)
	return cmd.Run()
}

func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return h
}

func launcherHome() string {
	// Install everything strictly under ~/.gh_aider
	return filepath.Join(homeDir(), InstallDirName)
}

func venvPythonPath(venv string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(venv, "Scripts", "python.exe")
	}
	return filepath.Join(venv, "bin", "python")
}

func aiderEntryExists(venv string) bool {
	p := venvPythonPath(venv)
	if _, err := os.Stat(p); err != nil {
		return false
	}
	// Quick sanity: try `-c "import aider"` (no output)
	cmd := exec.Command(p, "-c", "import aider; print('ok')")
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	return err == nil && string(out) == "ok\n"
}

func cloneOrUpdateRepo(repoURL, dest string) error {
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		if !cmdExists("git") {
			return errors.New("git not found; required to clone repository")
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		// Clone into ~/.gh_aider/src
		if err := run("git", "clone", repoURL, dest); err != nil {
			return err
		}
		if RepoRef != "" {
			if err := runInDir(dest, "git", "checkout", RepoRef); err != nil {
				return err
			}
		}
		return nil
	}
	// Update existing checkout (best-effort); keep within ~/.gh_aider/src
	if !cmdExists("git") {
		return nil
	}
	if err := runInDir(dest, "git", "fetch", "--all", "--tags"); err != nil {
		return nil
	}
	if RepoRef != "" {
		_ = runInDir(dest, "git", "checkout", RepoRef)
	} else {
		_ = runInDir(dest, "git", "pull", "--ff-only")
	}
	return nil
}

// Retained for future fallback, currently unused since we rely on uv to provision Python.
 type pySpec struct {
	cmd   string
	pre   []string
 }

 func isPython312(spec pySpec) bool { return false }
 func findSystemPython312() (pySpec, bool) { return pySpec{}, false }

 func uvBinName() string {
 	if runtime.GOOS == "windows" {
 		return "uv.exe"
 	}
 	return "uv"
 }

 func ensureUV(root string) (string, error) {
 	binDir := filepath.Join(root, "bin")
 	uvBin := filepath.Join(binDir, uvBinName())
 	if _, err := os.Stat(uvBin); err == nil {
 		return uvBin, nil
 	}
 	if cmdExists("uv") {
 		return "uv", nil
 	}
 	// Attempt to install uv into ~/.gh_aider/bin
 	if err := os.MkdirAll(binDir, 0o755); err != nil {
 		return "", err
 	}
 	installEnv := append(os.Environ(), "UV_INSTALL_DIR="+binDir)
 	var err error
 	if runtime.GOOS == "windows" {
 		err = runWithEnv(installEnv, "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", "irm https://astral.sh/uv/install.ps1 | iex")
 	} else {
 		err = runWithEnv(installEnv, "sh", "-c", "curl -LsSf https://astral.sh/uv/install.sh | sh -s -- --version latest")
 	}
 	if err != nil {
 		return "", fmt.Errorf("failed to install uv: %w", err)
 	}
 	if _, err := os.Stat(uvBin); err == nil {
 		return uvBin, nil
 	}
 	if cmdExists("uv") {
 		return "uv", nil
 	}
 	return "", errors.New("uv installation did not produce a usable binary")
 }

 func uvEnv(root string) []string {
 	return append(os.Environ(),
 		"UV_PYTHON_INSTALL_DIR="+filepath.Join(root, "python"),
 		"UV_TOOL_DIR="+filepath.Join(root, "tools"),
 		"UV_CACHE_DIR="+filepath.Join(root, "uv-cache"),
 		"XDG_CACHE_HOME="+filepath.Join(root, "xdg-cache"),
 		"XDG_DATA_HOME="+filepath.Join(root, "xdg-data"),
 	)
 }

func createVenvWithSystemPython(spec pySpec, venvPath string) error {
	if !isPython312(spec) {
		return errors.New("specified Python is not 3.12.x")
	}
	args := append(append([]string{}, spec.pre...), "-m", "venv", venvPath)
	return run(spec.cmd, args...)
}

// ensurePip ensures pip is available in the given Python by attempting to bootstrap via ensurepip if needed.
func ensurePip(python string) error {
	// Check if pip exists
	cmd := exec.Command(python, "-m", "pip", "--version")
	if err := cmd.Run(); err == nil {
		return nil
	}
	// Try to bootstrap pip
	cmd = exec.Command(python, "-m", "ensurepip", "--upgrade")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ensurepip failed: %w", err)
	}
	return nil
}

// pipInstall installs packages into the given venv using python -m pip, bootstrapping pip if missing.
func pipInstall(python string, dir string, installRoot string, pkgs ...string) error {
	if err := ensurePip(python); err != nil {
		return err
	}
	args := append([]string{"-m", "pip", "install"}, pkgs...)
	cmd := exec.Command(python, args...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cache := filepath.Join(installRoot, "pip-cache")
	cmd.Env = append(os.Environ(),
		"PIP_CACHE_DIR="+cache,
		"PIP_DISABLE_PIP_VERSION_CHECK=1",
	)
	return cmd.Run()
}

func runAiderFromVenv(venv string, args []string) error {
	python := venvPythonPath(venv)
	if _, err := os.Stat(python); err != nil {
		return err
	}
	runArgs := append([]string{"-m", "aider.main"}, args...)
	cmd := exec.Command(python, runArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"PIP_CACHE_DIR="+filepath.Join(launcherHome(), "pip-cache"),
	)
	return cmd.Run()
}

func sanitizeExtras(s string) string {
	// Keep only [a-zA-Z0-9_,-], then remove empty segments around commas
	b := make([]rune, 0, len(s))
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == ',' || r == '-' || r == '_' {
			b = append(b, r)
		}
	}
	parts := strings.Split(string(b), ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return strings.Join(out, ",")
}

func ensurePlaywrightIncluded(extras string) string {
	extras = strings.Trim(extras, ",")
	if extras == "" {
		return "playwright"
	}
	list := strings.Split(extras, ",")
	found := false
	for _, e := range list {
		if strings.TrimSpace(e) == "playwright" {
			found = true
			break
		}
	}
	if !found {
		list = append(list, "playwright")
	}
	// dedupe
	seen := map[string]struct{}{}
	res := make([]string, 0, len(list))
	for _, e := range list {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if _, ok := seen[e]; ok {
			continue
		}
		seen[e] = struct{}{}
		res = append(res, e)
	}
	return strings.Join(res, ",")
}

func setupVenvFromRepo(repoURL, installRoot string) (string, error) {
	repoDir := filepath.Join(installRoot, "src")
	venvDir := filepath.Join(installRoot, ".venv")
	binDir := filepath.Join(installRoot, "bin")
	_ = os.MkdirAll(binDir, 0o755)

	if err := os.MkdirAll(installRoot, 0o755); err != nil {
		return "", err
	}
	if err := cloneOrUpdateRepo(repoURL, repoDir); err != nil {
		return "", err
	}

	// Ensure uv is available for venv creation and pip operations
	uvPath, err := ensureUV(installRoot)
	if err != nil {
		return "", fmt.Errorf("uv not available: %w", err)
	}

	// Create venv if missing using uv (downloads Python inside ~/.gh_aider)
	python := venvPythonPath(venvDir)
	if _, err := os.Stat(python); os.IsNotExist(err) {
		if err := runWithEnv(uvEnv(installRoot), uvPath, "venv", "--python", RequiredPythonVersion, venvDir); err != nil {
			return "", fmt.Errorf("failed to create venv with uv: %w", err)
		}
	}

	// Upgrade base tooling in venv (bootstrap pip if needed)
	if err := pipInstall(python, repoDir, installRoot, "-U", "pip", "setuptools", "wheel"); err != nil {
		return "", fmt.Errorf("pip base upgrade failed: %w", err)
	}

	// Always install with playwright extra (and any user extras), and install Chromium
	extrasRaw := os.Getenv("AIDER_LAUNCHER_EXTRAS")
	extras := ensurePlaywrightIncluded(sanitizeExtras(extrasRaw))
	spec := fmt.Sprintf(".[%s]", extras)
	if err := pipInstall(python, repoDir, installRoot, "-e", spec); err != nil {
		return "", fmt.Errorf("aider install with extras '%s' failed: %w", extras, err)
	}
	// Install Chromium for Playwright
	if runtime.GOOS == "windows" {
		if err := runInDir(repoDir, python, "-m", "playwright", "install", "chromium"); err != nil {
			fmt.Fprintln(os.Stderr, "Warning: Playwright Chromium install failed:", err)
		}
	} else {
		if err := runInDir(repoDir, python, "-m", "playwright", "install", "--with-deps", "chromium"); err != nil {
			fmt.Fprintln(os.Stderr, "Warning: Playwright Chromium install failed:", err)
		}
	}

	return venvDir, nil
}

func main() {
	args := os.Args[1:]

	installRoot := launcherHome()
	venvDir := filepath.Join(installRoot, ".venv")
	repoURL := os.Getenv("AIDER_LAUNCHER_REPO")
	if repoURL == "" {
		repoURL = DefaultRepoURL
	}

	if aiderEntryExists(venvDir) {
		if err := runAiderFromVenv(venvDir, args); err == nil {
			return
		}
	}

	fmt.Fprintln(os.Stderr, "Setting up local virtual environment from repository using uv...")
	vdir, err := setupVenvFromRepo(repoURL, installRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Setup failed:", err)
		fmt.Fprintln(os.Stderr)
		printHelp()
		os.Exit(1)
	}

	if err := runAiderFromVenv(vdir, args); err != nil {
		fmt.Fprintln(os.Stderr, "Aider failed to start:", err)
		fmt.Fprintln(os.Stderr)
		printHelp()
		os.Exit(1)
	}
}

func printHelp() {
	rootDisplay := "~/" + InstallDirName
	fmt.Fprintln(os.Stderr, "This launcher installs and runs Aider entirely under "+rootDisplay+":")
	fmt.Fprintf(os.Stderr, "  repo:   %s/src (%s)\n", rootDisplay, DefaultRepoURL)
	fmt.Fprintf(os.Stderr, "  venv:   %s/.venv (managed by uv, Python %s)\n", rootDisplay, RequiredPythonVersion)
	fmt.Fprintf(os.Stderr, "  uv/bin: %s/bin\n", rootDisplay)
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Requirements:")
	fmt.Fprintln(os.Stderr, "  - git must be installed to clone/update the repository.")
	fmt.Fprintln(os.Stderr, "  - No system Python needed; uv will download Python inside "+rootDisplay+".")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Notes:")
	fmt.Fprintln(os.Stderr, "  - Playwright is installed automatically; Chromium is fetched during setup.")
	fmt.Fprintln(os.Stderr, "  - Voice (PortAudio) may require OS-level packages: macOS 'brew install portaudio', Linux 'sudo apt-get install libportaudio2'.")
}
