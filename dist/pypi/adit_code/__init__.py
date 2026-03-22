"""adit-code: AI-navigability code structure analysis."""

import os
import platform
import sys
import zipfile
from pathlib import Path

__version__ = "0.1.0"

# Binary is placed alongside this package during wheel build.
# Platform-tagged wheels contain the correct binary for each OS/arch.
_BIN_DIR = Path(__file__).parent / "bin"


def _find_binary() -> str:
    """Find the adit binary for the current platform."""
    system = platform.system().lower()
    machine = platform.machine().lower()

    # Normalize architecture names
    arch_map = {
        "x86_64": "amd64",
        "amd64": "amd64",
        "aarch64": "arm64",
        "arm64": "arm64",
    }
    arch = arch_map.get(machine, machine)

    # Normalize OS names
    os_map = {
        "linux": "linux",
        "darwin": "darwin",
        "windows": "windows",
    }
    os_name = os_map.get(system, system)

    ext = ".exe" if os_name == "windows" else ""
    binary_name = f"adit{ext}"

    # Check in bin directory (wheel layout)
    bin_path = _BIN_DIR / f"{os_name}_{arch}" / binary_name
    if bin_path.exists():
        return str(bin_path)

    # Check if adit is on PATH (installed via go install)
    for path_dir in os.environ.get("PATH", "").split(os.pathsep):
        candidate = os.path.join(path_dir, binary_name)
        if os.path.isfile(candidate) and os.access(candidate, os.X_OK):
            return candidate

    return ""


def main() -> None:
    """Entry point: exec the Go binary with the same arguments."""
    binary = _find_binary()
    if not binary:
        print(
            "adit-code: binary not found. Install via:\n"
            "  go install github.com/justinstimatze/adit-code/cmd/adit@latest\n"
            "or download from GitHub releases.",
            file=sys.stderr,
        )
        sys.exit(2)

    os.execvp(binary, [binary] + sys.argv[1:])
