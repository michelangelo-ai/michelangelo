"""
Utility functions for MaCTL subprocess management and environment detection.
"""

import os
import shutil
import subprocess
import sys
from pathlib import Path
from typing import Optional, Tuple
from logging import getLogger

_logger = getLogger(__name__)


def detect_user_python_interpreter() -> str:
    """
    Detect the user's Python interpreter from environment metadata.

    This function attempts to find the appropriate Python interpreter
    for the user's project environment, following common patterns:

    1. Virtual environment (venv/virtualenv)
    2. Conda environment
    3. Poetry environment
    4. UV environment
    5. System Python (fallback)

    Returns:
        str: Path to the user's Python interpreter

    Raises:
        RuntimeError: If no suitable Python interpreter can be found
    """
    _logger.debug("Detecting user Python interpreter")

    # 1. Check for virtual environment
    if "VIRTUAL_ENV" in os.environ:
        venv_path = Path(os.environ["VIRTUAL_ENV"])
        python_exe = "python.exe" if sys.platform == "win32" else "python"
        python_path = venv_path / "bin" / python_exe

        if python_path.exists():
            _logger.info("Found virtual environment Python: %s", python_path)
            return str(python_path)

    # 2. Check for Conda environment
    if "CONDA_DEFAULT_ENV" in os.environ or "CONDA_PREFIX" in os.environ:
        conda_prefix = os.environ.get("CONDA_PREFIX")
        if conda_prefix:
            conda_path = Path(conda_prefix)
            python_exe = "python.exe" if sys.platform == "win32" else "python"
            python_path = conda_path / "bin" / python_exe

            if python_path.exists():
                _logger.info("Found Conda environment Python: %s", python_path)
                return str(python_path)

    # 3. Check for Poetry environment
    poetry_python = detect_poetry_python()
    if poetry_python:
        _logger.info("Found Poetry environment Python: %s", poetry_python)
        return poetry_python

    # 4. Check for UV environment
    uv_python = detect_uv_python()
    if uv_python:
        _logger.info("Found UV environment Python: %s", uv_python)
        return uv_python

    # 5. Fall back to system Python
    system_python = shutil.which("python3") or shutil.which("python")
    if system_python:
        _logger.info("Using system Python: %s", system_python)
        return system_python

    raise RuntimeError(
        "Could not detect a suitable Python interpreter. "
        "Please ensure Python is installed and available in PATH."
    )


def detect_poetry_python() -> Optional[str]:
    """
    Detect Python interpreter from Poetry environment.

    Returns:
        Optional[str]: Path to Poetry's Python interpreter if found
    """
    try:
        # Check if poetry is available
        if not shutil.which("poetry"):
            return None

        # Run poetry env info to get interpreter path
        result = subprocess.run(
            ["poetry", "env", "info", "--path"],
            capture_output=True,
            text=True,
            timeout=10,
        )

        if result.returncode == 0:
            venv_path = Path(result.stdout.strip())
            python_exe = "python.exe" if sys.platform == "win32" else "python"
            python_path = venv_path / "bin" / python_exe

            if python_path.exists():
                return str(python_path)

    except (subprocess.TimeoutExpired, subprocess.SubprocessError, OSError):
        _logger.debug("Failed to detect Poetry environment")

    return None


def detect_uv_python() -> Optional[str]:
    """
    Detect Python interpreter from UV environment.

    Returns:
        Optional[str]: Path to UV's Python interpreter if found
    """
    try:
        # Check if uv is available
        if not shutil.which("uv"):
            return None

        # Check for UV project
        if not Path("pyproject.toml").exists():
            return None

        # Run uv python find to get interpreter path
        result = subprocess.run(
            ["uv", "python", "find"], capture_output=True, text=True, timeout=10
        )

        if result.returncode == 0:
            python_path = result.stdout.strip()
            if python_path and Path(python_path).exists():
                return python_path

    except (subprocess.TimeoutExpired, subprocess.SubprocessError, OSError):
        _logger.debug("Failed to detect UV environment")

    return None


def run_subprocess_registration(
    project: str,
    pipeline: str,
    config_file_path: str,
    output_dir: str,
    storage_url: Optional[str] = None,
    output_filename: Optional[str] = None,
    environ: Optional[dict] = None,
    args: Optional[list] = None,
    kwargs: Optional[dict] = None,
) -> subprocess.CompletedProcess:
    """
    Execute registration in a subprocess using the user's Python environment.

    Args:
        project: Project name
        pipeline: Pipeline name
        config_file_path: Path to pipeline configuration file
        output_dir: Directory for output files
        storage_url: Optional storage URL
        output_filename: Optional output filename
        environ: Optional environment variables
        args: Optional positional arguments
        kwargs: Optional keyword arguments

    Returns:
        subprocess.CompletedProcess: Result of subprocess execution

    Raises:
        RuntimeError: If subprocess execution fails
    """
    # Detect user's Python interpreter
    user_python = detect_user_python_interpreter()

    # Build subprocess command
    cmd = [
        user_python,
        "-m",
        "michelangelo.uniflow.registration.subprocess",
        "--project",
        project,
        "--pipeline",
        pipeline,
        "--config-file",
        config_file_path,
        "--output-dir",
        output_dir,
    ]

    # Add optional arguments
    if storage_url:
        cmd.extend(["--storage-url", storage_url])
    if output_filename:
        cmd.extend(["--output-filename", output_filename])
    if environ:
        import json

        cmd.extend(["--environ", json.dumps(environ)])
    if args:
        import json

        cmd.extend(["--args", json.dumps(args)])
    if kwargs:
        import json

        cmd.extend(["--kwargs", json.dumps(kwargs)])

    _logger.info("Executing registration subprocess: %s", " ".join(cmd))

    try:
        # Execute subprocess with timeout
        result = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            timeout=300,  # 5 minute timeout
            cwd=os.getcwd(),
        )

        _logger.debug("Subprocess stdout: %s", result.stdout)
        if result.stderr:
            _logger.debug("Subprocess stderr: %s", result.stderr)

        return result

    except subprocess.TimeoutExpired as e:
        raise RuntimeError(f"Registration subprocess timed out after 5 minutes: {e}")
    except subprocess.SubprocessError as e:
        raise RuntimeError(f"Failed to execute registration subprocess: {e}")


def read_subprocess_outputs(output_dir: str) -> Tuple[bool, str, Optional[str]]:
    """
    Read outputs from subprocess registration.

    Args:
        output_dir: Directory containing output files

    Returns:
        Tuple of (success, message, remote_path)
        - success: Whether registration succeeded
        - message: Success/error message
        - remote_path: Remote tarball path if successful
    """
    output_path = Path(output_dir)

    # Check for success indicator
    success_file = output_path / "registration_success.txt"
    if success_file.exists():
        content = success_file.read_text().strip()
        if content.startswith("SUCCESS: "):
            remote_path = content[9:]  # Remove "SUCCESS: " prefix
            return True, "Registration completed successfully", remote_path

    # Check for error indicator
    error_file = output_path / "registration_error.txt"
    if error_file.exists():
        content = error_file.read_text().strip()
        if content.startswith("ERROR: "):
            error_msg = content[7:]  # Remove "ERROR: " prefix
            return False, f"Registration failed: {error_msg}", None

    # No clear indicator found
    return False, "Registration status unknown - no status files found", None
