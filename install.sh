#!/usr/bin/env bash
# deadline (gotodo) installer — one script for all Linux distros.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/19Naveen/deadline/main/install.sh | bash
#   curl -fsSL https://raw.githubusercontent.com/19Naveen/deadline/main/install.sh | bash -s -- --dir ~/.local/bin
#
# What it does:
#   1. Detects your distro / package manager (apt, pacman, dnf, yum, zypper, apk)
#   2. Installs git + Go (>= 1.22) if missing
#   3. Clones the repo to a temp dir and builds the `gotodo` binary
#   4. Installs it to ~/.local/bin (or --dir) and adds that dir to PATH
#      in ~/.bashrc and ~/.zshrc (idempotent — never appends twice)
#   5. Installs the `deadline` agent skill for Claude Code, Codex, OpenCode
#      and other Agent-Skills-compatible tools
set -euo pipefail

: "${HOME:?HOME must be set}"

newline='
'

REPO="${DEADLINE_REPO:-19Naveen/deadline}"
BRANCH="${DEADLINE_BRANCH:-main}"
BINARY="gotodo"
INSTALL_DIR="${HOME}/.local/bin"
GO_MIN_MAJOR=1
GO_MIN_MINOR=22
UNINSTALL=0

# --- pretty output ------------------------------------------------------
if [ -t 1 ]; then
  GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; RESET='\033[0m'
else
  GREEN=''; YELLOW=''; RED=''; RESET=''
fi
info() { printf "${GREEN}==> %s${RESET}\n" "$*"; }
warn() { printf "${YELLOW}==> %s${RESET}\n" "$*"; }
die()  { printf "${RED}error: %s${RESET}\n" "$*" >&2; exit 1; }

# Pick up a previous rootless Go install so re-runs don't re-download it.
if [ -x "${HOME}/.local/go/bin/go" ]; then
  case ":${PATH}:" in
    *":${HOME}/.local/go/bin:"*) ;;
    *) export PATH="${HOME}/.local/go/bin:${PATH}" ;;
  esac
fi

# --- args ---------------------------------------------------------------
while [ $# -gt 0 ]; do
  case "$1" in
    --dir)
      INSTALL_DIR="${2:?--dir needs a path}"; shift 2
      # Expand a leading ~/ and require an absolute path: a relative dir
      # would write a PATH export that breaks in every future shell.
      case "$INSTALL_DIR" in
        "~"/*) INSTALL_DIR="$HOME/${INSTALL_DIR#"~/"}" ;;
        "~") INSTALL_DIR="$HOME" ;;
      esac
      case "$INSTALL_DIR" in
        /*) ;;
        *) die "--dir must be an absolute path, got: $INSTALL_DIR" ;;
      esac
      case "$INSTALL_DIR" in
        *'`'* | *'"'* | *"'"* | *'$'* | *'!'* | *'\'* | *"$newline"*)
          die "--dir contains characters unsafe for a shell export" ;;
      esac ;;
    --repo)
      REPO="${2:?--repo needs owner/name}"; shift 2 ;;
    --branch)
      BRANCH="${2:?--branch needs a branch}"; shift 2 ;;
    --uninstall)
      UNINSTALL=1; shift ;;
    -h|--help)
      cat <<'EOF'
deadline (gotodo) installer — one script for all Linux distros.
Usage:
  curl -fsSL https://raw.githubusercontent.com/19Naveen/deadline/main/install.sh | bash
  curl -fsSL https://raw.githubusercontent.com/19Naveen/deadline/main/install.sh | bash -s -- --dir ~/.local/bin
  curl -fsSL https://raw.githubusercontent.com/19Naveen/deadline/main/install.sh | bash -s -- --uninstall
Options:
  --dir PATH      install directory (default ~/.local/bin)
  --repo O/N      GitHub repo (default 19Naveen/deadline)
  --branch NAME   git branch (default main)
  --uninstall     remove the binary, skills and PATH lines (boards kept)
  -h, --help      show this help
Env overrides: DEADLINE_REPO, DEADLINE_BRANCH
EOF
      exit 0 ;; 
    *)
      echo "Unknown option: $1 (see --help)" >&2; exit 1 ;;
  esac
done

printf '%s' "$REPO" | grep -Eq '^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$' \
  || die "--repo must look like owner/name, got: $REPO"

# --- helpers ------------------------------------------------------------
have() { command -v "$1" >/dev/null 2>&1; }

run_privileged() {
  # Run "$@" as root: directly if root, via sudo if available.
  if [ "$(id -u)" -eq 0 ]; then
    "$@"
  elif have sudo; then
    sudo "$@"
  else
    return 1
  fi
}

detect_pm() {
  # Echo one of: apt pacman dnf yum zypper apk, or empty if unknown.
  if have apt-get;   then echo apt;
  elif have pacman;  then echo pacman;
  elif have dnf;     then echo dnf;
  elif have yum;     then echo yum;
  elif have zypper;  then echo zypper;
  elif have apk;     then echo apk;
  else echo "";
  fi
}

go_version_ok() {
  # True if `go version` reports >= GO_MIN_MAJOR.GO_MIN_MINOR.
  have go || return 1
  local v
  v="$(go version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+(\.[0-9]+)?' | head -n1)" || return 1
  [ -n "$v" ] || return 1
  local major minor
  major="${v%%.*}"
  minor="$(echo "$v" | cut -d. -f2)"
  [ "$major" -gt "$GO_MIN_MAJOR" ] && return 0
  [ "$major" -eq "$GO_MIN_MAJOR" ] && [ "$minor" -ge "$GO_MIN_MINOR" ] && return 0
  return 1
}

install_via_pm() {
  # $1 = pm, rest = packages. Returns 0 on success.
  local pm="$1"; shift
  case "$pm" in
    apt)     run_privileged apt-get update && run_privileged apt-get install -y "$@" ;;
    pacman)  run_privileged pacman -Sy --noconfirm "$@" ;;
    dnf|yum) run_privileged "$pm" install -y "$@" ;;
    zypper)  run_privileged zypper --non-interactive install "$@" ;;
    apk)     run_privileged apk add --no-cache "$@" ;;
    *) return 1 ;;
  esac
}

install_go_official() {
  # Fallback when the distro's Go is too old or missing: install the
  # latest stable Go toolchain into ~/.local/go and use it for this run.
  local gotag os arch url tmp
  have curl || die "curl is required to download Go — please install curl first."
  info "Distro Go is missing or < ${GO_MIN_MAJOR}.${GO_MIN_MINOR}; fetching official Go toolchain…"
  gotag="$(curl -fsSL https://go.dev/VERSION?m=text | head -n1)"
  [ -n "$gotag" ] || die "Could not determine the latest Go version from go.dev"
  os="linux"
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64) arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    armv6l|armv7l) arch="armv6l" ;;
    i386|i686) arch="386" ;;
    *) die "Unsupported CPU architecture for official Go download: $arch" ;;
  esac
  url="https://go.dev/dl/${gotag}.${os}-${arch}.tar.gz"
  info "Downloading ${gotag} for ${os}/${arch}…"
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' RETURN
  curl -fsSL "$url" -o "$tmp/go.tar.gz" || die "Go download failed: $url"
  # Verify the checksum: a toolchain download is a trust boundary.
  have sha256sum || die "sha256sum is required to verify the Go download — please install coreutils first."
  curl -fsSL "$url.sha256" -o "$tmp/go.tar.gz.sha256" || die "Go checksum download failed: $url.sha256"
  (cd "$tmp" && sha256sum -c go.tar.gz.sha256) || die "Go checksum mismatch — refusing to install."
  if [ -e "${HOME}/.local/go" ]; then
    bak="${HOME}/.local/go.bak.$(date +%s)"
    warn "Moving existing ${HOME}/.local/go aside to $bak"
    mv "${HOME}/.local/go" "$bak" || die "Could not move aside ${HOME}/.local/go"
  fi
  mkdir -p "${HOME}/.local"
  tar -C "${HOME}/.local" -xzf "$tmp/go.tar.gz"
  export PATH="${HOME}/.local/go/bin:${PATH}"
  export GOROOT="${HOME}/.local/go"
  go_version_ok || die "Official Go install failed unexpectedly."
  info "Installed ${gotag} to ~/.local/go"
  # Persist the rootless Go on PATH so future shells (and re-runs) find it.
  go_line='export PATH="$HOME/.local/go/bin:$PATH"'
  for rc in "$HOME/.bashrc" "$HOME/.zshrc"; do
    [ -f "$rc" ] || continue
    grep -qF ".local/go/bin" "$rc" || printf '\n# added by deadline installer (go)\n%s\n' "$go_line" >> "$rc"
  done
}

ensure_deps() {
  local pm missing_git missing_go pm_ok pm_hint pkgs
  missing_git=0; missing_go=0
  have git || missing_git=1
  go_version_ok || missing_go=1
  if [ "$missing_git" -eq 0 ] && [ "$missing_go" -eq 0 ]; then
    info "Dependencies already satisfied (git + $(go version | awk '{print $3}'))."
    return 0
  fi

  pm="$(detect_pm)"
  [ -n "$pm" ] || {
    warn "No supported package manager found (looked for apt, pacman, dnf, yum, zypper, apk)."
    [ "$missing_git" -eq 1 ] && warn "Please install git manually, then re-run this script."
    [ "$missing_go" -eq 1 ] && install_go_official && return 0
    return 0
  }

  info "Installing missing tools with ${pm} (git=$([ "$missing_git" -eq 1 ] && echo yes || echo no), go=$([ "$missing_go" -eq 1 ] && echo yes || echo no))…"
  pm_ok=1
  case "$pm" in
    apt)
      pkgs=""
      [ "$missing_git" -eq 1 ] && pkgs="$pkgs git"
      [ "$missing_go" -eq 1 ] && pkgs="$pkgs golang-go"
      # shellcheck disable=SC2086
      install_via_pm apt $pkgs || pm_ok=0
      pm_hint="sudo apt-get update && sudo apt-get install -y git golang-go"
      ;;
    pacman)
      pkgs=""
      [ "$missing_git" -eq 1 ] && pkgs="$pkgs git"
      [ "$missing_go" -eq 1 ] && pkgs="$pkgs go"
      # shellcheck disable=SC2086
      install_via_pm pacman $pkgs || pm_ok=0
      pm_hint="sudo pacman -Sy --noconfirm git go"
      ;;
    dnf|yum)
      pkgs=""
      [ "$missing_git" -eq 1 ] && pkgs="$pkgs git"
      [ "$missing_go" -eq 1 ] && pkgs="$pkgs golang"
      # shellcheck disable=SC2086
      install_via_pm "$pm" $pkgs || pm_ok=0
      pm_hint="sudo $pm install -y git golang"
      ;;
    zypper)
      pkgs=""
      [ "$missing_git" -eq 1 ] && pkgs="$pkgs git"
      [ "$missing_go" -eq 1 ] && pkgs="$pkgs go"
      # shellcheck disable=SC2086
      install_via_pm zypper $pkgs || pm_ok=0
      pm_hint="sudo zypper install -y git go"
      ;;
    apk)
      pkgs=""
      [ "$missing_git" -eq 1 ] && pkgs="$pkgs git"
      [ "$missing_go" -eq 1 ] && pkgs="$pkgs go"
      # shellcheck disable=SC2086
      install_via_pm apk $pkgs || pm_ok=0
      pm_hint="sudo apk add git go"
      ;;
  esac

  if [ "$pm_ok" -eq 0 ]; then
    # Package-manager install failed — most commonly "sudo: a password is
    # required" when piped via curl. If git is already here we can still
    # finish rootless: fetch the official Go toolchain into ~/.local/go.
    if [ "$missing_git" -eq 0 ] && [ "$missing_go" -eq 1 ] && ! go_version_ok; then
      warn "Could not install Go with ${pm} (no root?). Falling back to a rootless install."
      install_go_official
    else
      die "Package install failed. Please run manually and re-run this script: ${pm_hint}"
    fi
  fi

  # Distro Go (notably on Debian/Ubuntu LTS) is often older than 1.22 —
  # fall back to the official toolchain so the build never fails there.
  if [ "$missing_go" -eq 1 ] && ! go_version_ok; then
    warn "Distro Go is still < ${GO_MIN_MAJOR}.${GO_MIN_MINOR}; switching to official Go."
    install_go_official
  fi
  have git || die "git is still missing after install — please install it manually."
  go_version_ok || die "Go >= ${GO_MIN_MAJOR}.${GO_MIN_MINOR} is still missing — please install it manually."
}

ensure_path_in_rc() {
  # Append export line to ~/.bashrc and ~/.zshrc if not already present.
  local line="export PATH=\"\$HOME/.local/bin:\$PATH\""
  local rc
  # Only touch the target dir's PATH line when installing to the default.
  case "$INSTALL_DIR" in
    "$HOME/.local/bin") line="export PATH=\"\$HOME/.local/bin:\$PATH\"" ;;
    *) line="export PATH=\"$INSTALL_DIR:\$PATH\"" ;;
  esac
  for rc in "$HOME/.bashrc" "$HOME/.zshrc"; do
    if [ ! -f "$rc" ]; then
      # Create the rc if its shell is actually installed — otherwise skip.
      case "$rc" in
        *.bashrc) have bash || continue ;;
        *.zshrc)  have zsh  || continue ;;
      esac
      touch "$rc"
    fi
    # Match the directory rather than the exact line: equivalent exports
    # (e.g. an expanded $HOME vs a literal one) already do the job, and a
    # duplicate PATH entry is pure noise.
    if grep -qF -- "$INSTALL_DIR" "$rc"; then
      info "$INSTALL_DIR already on PATH in $(basename "$rc") — skipping."
    else
      printf '\n# added by deadline installer\n%s\n' "$line" >> "$rc"
      info "Added $INSTALL_DIR to PATH in $(basename "$rc")."
    fi
  done
  # Effect for the rest of this script / session hint.
  case ":${PATH}:" in
    *":${INSTALL_DIR}:"*) ;;
    *) export PATH="${INSTALL_DIR}:${PATH}" ;;
  esac
}

# uninstall_gotodo removes the binary, the agent skills and the PATH lines
# this script added. Boards are data, not installation, so they stay:
# ~/.config/gotodo and ./.deadline in projects survive for reinstalls.
uninstall_gotodo() {
  rm -f "$INSTALL_DIR/$BINARY"
  info "Removed $INSTALL_DIR/$BINARY (if present)."
  local dest rc tmp
  for dest in \
    "$HOME/.claude/skills/deadline" \
    "$HOME/.codex/skills/deadline" \
    "$HOME/.agents/skills/deadline" \
    "$HOME/.config/opencode/skills/deadline"; do
    if [ -e "$dest" ]; then
      rm -rf "$dest"
      info "Removed $dest"
    fi
  done
  for rc in "$HOME/.bashrc" "$HOME/.zshrc"; do
    [ -f "$rc" ] || continue
    tmp="$(mktemp)"
    awk '/^# added by deadline installer/ { pending=1; next } pending { pending=0; if (/^export PATH=/) next } { print }' "$rc" > "$tmp" && cat "$tmp" > "$rc"
    rm -f "$tmp"
  done
  info "Uninstall complete. Boards kept — delete ~/.config/gotodo and ./.deadline too for a full wipe."
}

# install_skill copies the agent skill into every well-known skills dir so
# Claude Code, Codex, OpenCode and other Agent-Skills-compatible tools pick
# it up without further setup. $1 is the cloned repo root.
install_skill() {
  local src="$1/skills/deadline/SKILL.md"
  [ -f "$src" ] || { warn "Skill source missing, skipping agent setup."; return 0; }
  local dest
  for dest in \
    "$HOME/.claude/skills/deadline/SKILL.md" \
    "$HOME/.codex/skills/deadline/SKILL.md" \
    "$HOME/.agents/skills/deadline/SKILL.md" \
    "$HOME/.config/opencode/skills/deadline/SKILL.md"; do
    mkdir -p "$(dirname "$dest")"
    cp "$src" "$dest"
    info "Agent skill installed to $dest"
  done
}

# --- main ---------------------------------------------------------------
if [ "$UNINSTALL" -eq 1 ]; then
  uninstall_gotodo
  exit 0
fi
info "Installing deadline (gotodo) from ${REPO}@${BRANCH}…"
# Whether the install dir was already usable before this script touched
# PATH, so the closing hint is honest even though ensure_path_in_rc exports
# it into this shell's environment below.
case ":${PATH}:" in
  *":${INSTALL_DIR}:"*) path_was_ready=1 ;;
  *) path_was_ready=0 ;;
esac
ensure_deps

mkdir -p "$INSTALL_DIR"
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
info "Cloning https://github.com/${REPO} (branch ${BRANCH})…"
git clone --quiet --depth 1 --branch "$BRANCH" -- "https://github.com/${REPO}.git" "$workdir/src" \
  || die "git clone failed — check the repo name/branch and your network."

head_sha="$(git -C "$workdir/src" rev-parse HEAD)"
installed_version=""
if [ -x "$INSTALL_DIR/$BINARY" ]; then
  installed_version="$("$INSTALL_DIR/$BINARY" version 2>/dev/null || true)"
fi
if [ -n "$head_sha" ] && [ "$installed_version" = "$head_sha" ]; then
  info "$BINARY is already up to date ($head_sha) — skipping rebuild."
else
  info "Building ${BINARY} (needs Go >= ${GO_MIN_MAJOR}.${GO_MIN_MINOR})…"
  (
    cd "$workdir/src"
    go build -trimpath -ldflags "-s -w -X main.version=$head_sha" -o "$INSTALL_DIR/$BINARY" .
  ) || die "go build failed."
  chmod +x "$INSTALL_DIR/$BINARY"
fi
install_skill "$workdir/src"
trap - EXIT
rm -rf "$workdir"

ensure_path_in_rc

if "$INSTALL_DIR/$BINARY" -h >/dev/null 2>&1; then
  info "Verified: $INSTALL_DIR/$BINARY runs."
else
  warn "Binary built but '$BINARY -h' returned non-zero — it may still work; try running it."
fi

printf '\n%s\n' "Done! Run it with:"
printf '  %s\n' "  $BINARY"
if [ "$path_was_ready" -eq 0 ]; then
  printf '%s\n' "  (Restart your terminal or run: export PATH=\"$INSTALL_DIR:\$PATH\")"
fi
printf '%s\n' "Data lives in ~/.config/gotodo/tasks.json (override with: gotodo -file ./work.json)"
printf '%s\n' "Project board: cd into a project and run 'gotodo init' (kept in ./.deadline/, gitignored)"
