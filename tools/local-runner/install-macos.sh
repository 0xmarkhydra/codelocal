#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RUNNER_DIR="$HOME/.codelocal/github-actions/codelocal-runner"
LAUNCH_AGENTS_DIR="$HOME/Library/LaunchAgents"
PLIST_PATH="$LAUNCH_AGENTS_DIR/com.codelocal.github-jit-runner.plist"
LABEL="com.codelocal.github-jit-runner"
RUNNER_VERSION="2.337.0"
RUNNER_ARCHIVE="actions-runner-osx-arm64-${RUNNER_VERSION}.tar.gz"
RUNNER_URL="https://github.com/actions/runner/releases/download/v${RUNNER_VERSION}/${RUNNER_ARCHIVE}"
PYTHON_BIN="/opt/homebrew/bin/python3"
GH_BIN="/opt/homebrew/bin/gh"

need() {
  if [[ ! -x "$1" ]]; then
    echo "Missing required executable: $1" >&2
    exit 1
  fi
}

need "$PYTHON_BIN"
need "$GH_BIN"

if ! "$GH_BIN" auth status >/dev/null 2>&1; then
  echo "GitHub CLI is not authenticated. Run: gh auth login" >&2
  exit 1
fi

mkdir -p "$RUNNER_DIR" "$LAUNCH_AGENTS_DIR"

if [[ ! -x "$RUNNER_DIR/run.sh" ]]; then
  echo "Installing GitHub Actions runner v$RUNNER_VERSION..."
  tmp_archive="$RUNNER_DIR/$RUNNER_ARCHIVE"
  curl -fL --retry 5 --retry-delay 2 --connect-timeout 30 -o "$tmp_archive" "$RUNNER_URL"
  tar xzf "$tmp_archive" -C "$RUNNER_DIR"
  rm -f "$tmp_archive"
fi

chmod 700 "$ROOT_DIR/tools/local-runner/jit-runner.py"

cat > "$PLIST_PATH" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>$LABEL</string>
  <key>ProgramArguments</key>
  <array>
    <string>$PYTHON_BIN</string>
    <string>$ROOT_DIR/tools/local-runner/jit-runner.py</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>ThrottleInterval</key>
  <integer>10</integer>
  <key>WorkingDirectory</key>
  <string>$ROOT_DIR</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key>
    <string>/opt/homebrew/bin:$HOME/.nvm/versions/node/v24.20.0/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
    <key>HOME</key>
    <string>$HOME</string>
  </dict>
  <key>StandardOutPath</key>
  <string>/tmp/codelocal-github-jit-runner.log</string>
  <key>StandardErrorPath</key>
  <string>/tmp/codelocal-github-jit-runner.log</string>
</dict>
</plist>
PLIST

plutil -lint "$PLIST_PATH"

launchctl bootout "gui/$(id -u)/$LABEL" >/dev/null 2>&1 || true
launchctl bootstrap "gui/$(id -u)" "$PLIST_PATH"
launchctl kickstart -k "gui/$(id -u)/$LABEL"

sleep 2
if launchctl print "gui/$(id -u)/$LABEL" >/dev/null 2>&1; then
  echo "CodeLocal JIT runner service installed and active."
  echo "Log: /tmp/codelocal-github-jit-runner.log"
else
  echo "Failed to start $LABEL" >&2
  exit 1
fi
