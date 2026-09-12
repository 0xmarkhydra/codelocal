#!/usr/bin/env python3
"""Run one ephemeral GitHub Actions JIT runner for CodeLocal.

This process is designed to be supervised by launchd. It creates a repo-scoped
JIT runner, waits for exactly one trusted workflow job, cleans up the runner,
and exits. launchd then starts a fresh JIT runner for the next job.
"""

from __future__ import annotations

import json
import os
import shutil
import signal
import subprocess
import sys
from pathlib import Path

REPOSITORY = "codelocal-cloud/codelocal"
RUNNER_LABEL = "codelocal-release"
RUNNER_ROOT = Path.home() / ".codelocal" / "github-actions" / "codelocal-runner"


def command_path(name: str, preferred: str) -> str:
    if Path(preferred).is_file():
        return preferred
    resolved = shutil.which(name)
    if resolved:
        return resolved
    raise RuntimeError(f"Required command not found: {name}")


def remove_local_credentials() -> None:
    for filename in (".runner", ".credentials", ".credentials_rsaparams"):
        path = RUNNER_ROOT / filename
        try:
            path.unlink()
        except FileNotFoundError:
            pass


def delete_remote_runner(gh: str, runner_id: int | None) -> None:
    if runner_id is None:
        return
    subprocess.run(
        [
            gh,
            "api",
            "--method",
            "DELETE",
            f"repos/{REPOSITORY}/actions/runners/{runner_id}",
        ],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        check=False,
        env=os.environ,
    )


def main() -> int:
    os.environ["PATH"] = ":".join(
        [
            "/opt/homebrew/bin",
            str(Path.home() / ".nvm" / "versions" / "node" / "v24.20.0" / "bin"),
            "/usr/local/bin",
            "/usr/bin",
            "/bin",
            "/usr/sbin",
            "/sbin",
        ]
    )

    gh = command_path("gh", "/opt/homebrew/bin/gh")
    run_script = RUNNER_ROOT / "run.sh"
    if not run_script.is_file():
        raise RuntimeError(
            f"GitHub Actions runner is not installed at {RUNNER_ROOT}. "
            "Run tools/local-runner/install-macos.sh first."
        )

    runner_id: int | None = None
    child: subprocess.Popen[str] | None = None

    def stop_child(signum: int, _frame: object) -> None:
        if child is not None and child.poll() is None:
            child.send_signal(signum)

    signal.signal(signal.SIGTERM, stop_child)
    signal.signal(signal.SIGINT, stop_child)

    try:
        remove_local_credentials()
        hostname = os.uname().nodename.split(".", 1)[0]
        response = subprocess.run(
            [
                gh,
                "api",
                "--method",
                "POST",
                f"repos/{REPOSITORY}/actions/runners/generate-jitconfig",
                "-F",
                "runner_group_id=1",
                "-f",
                f"name=codelocal-jit-{hostname}",
                "-f",
                "labels[]=self-hosted",
                "-f",
                f"labels[]={RUNNER_LABEL}",
                "-f",
                "work_folder=_work",
            ],
            text=True,
            capture_output=True,
            check=True,
            env=os.environ,
        )
        payload = json.loads(response.stdout)
        runner_id = int(payload["runner"]["id"])
        jit_config = str(payload["encoded_jit_config"])

        print(f"CodeLocal JIT runner {runner_id} ready for one release job.", flush=True)
        child = subprocess.Popen(
            [str(run_script), "--jitconfig", jit_config],
            cwd=RUNNER_ROOT,
            text=True,
            env=os.environ,
        )
        return child.wait()
    finally:
        delete_remote_runner(gh, runner_id)
        remove_local_credentials()


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print(f"CodeLocal JIT runner failed: {exc}", file=sys.stderr, flush=True)
        raise SystemExit(1)
