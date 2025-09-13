Aider Launcher

Go launcher that installs and runs Aider entirely under your home folder, without requiring a system Python or pyenv.

Where things go:
- ~/.gh_aider/src     - Git clone of the repository (default https://github.com/drajnic/aider)
- ~/.gh_aider/.venv   - Python 3.12 virtual environment (created by uv)
- ~/.gh_aider/bin     - uv binary (downloaded automatically if missing)
- ~/.gh_aider/python  - uv-managed Python runtime
- ~/.gh_aider/pip-cache, ~/.gh_aider/uv-cache - local caches for installs

What the launcher does:
1) Ensures `uv` is available (installs to ~/.gh_aider/bin if missing)
2) Creates a Python venv (3.12) with `uv venv` under ~/.gh_aider/.venv
3) Clones/updates the repo into ~/.gh_aider/src
4) Bootstraps pip in the venv if needed (via `python -m ensurepip --upgrade`), then:
   - Upgrades base tooling: `python -m pip install -U pip setuptools wheel`
   - Installs Aider (editable) with extras. `playwright` is always included and required.
   - Runs Playwright Chromium install automatically (uses `--with-deps` on Linux/macOS)
5) Runs Aider: `python -m aider.main [args...]`

Constants (in main.go):
- InstallDirName: ".gh_aider" (install root under HOME)
- DefaultRepoURL: "https://github.com/drajnic/aider"
- RequiredPythonVersion: "3.12"
- RepoRef: "" (optional branch/tag/SHA to checkout; empty = repo default)

Environment variables:
- AIDER_LAUNCHER_REPO   - Override repo URL (default is DefaultRepoURL)
- AIDER_LAUNCHER_EXTRAS - Comma-separated extras to install in addition to `playwright`, e.g. `browser,help,dev`

Requirements:
- git must be installed and on PATH
- No system Python required; uv downloads/uses Python inside ~/.gh_aider

Build:
- Ensure Go >= 1.21 is installed (e.g., go1.22+)
- From repo root:
  - cd launcher
  - go build -o aider-launcher

Run unit tests:
- cd launcher && go test -v ./...

Smoke test (isolated HOME):
- macOS/Linux:
  - export HOME="$(mktemp -d)"
  - ./aider-launcher --help
- Windows (PowerShell):
  - $tmp = Join-Path $env:TEMP ("home_" + [guid]::NewGuid().Guid)
  - New-Item -ItemType Directory -Force -Path $tmp | Out-Null
  - $env:USERPROFILE = $tmp
  - .\aider-launcher.exe --help

CI (GitHub Actions):
- Workflow: .github/workflows/launcher-ci.yml
  - Builds the launcher on Linux/macOS/Windows
  - Runs `go test -v ./...`
  - Creates an isolated HOME and runs the launcher one time to validate uv, venv, repo clone, install with `playwright`, and Chromium install
- Manual trigger supported (workflow_dispatch). Run from the "Actions" tab.

Notes & troubleshooting:
- Playwright is installed automatically and Chromium is fetched during setup. On macOS the `--with-deps` flag is ignored (harmless).
- Voice features may require OS-level packages: macOS `brew install portaudio`, Linux `sudo apt-get install libportaudio2`.
- If the venv Python initially lacks pip, the launcher bootstraps it using `python -m ensurepip --upgrade` before calling `python -m pip`.
- Everything is installed strictly under `~/.gh_aider` — nothing is written elsewhere.
