"""sitecustomize.py - Automatic Uniflow initialization for remote environments.

This module is automatically imported by Python during startup when it's in the
Python path. It runs the uniflow pre-run script to download and apply local
changes in remote containers.

This leverages Python's built-in site initialization mechanism for clean,
automatic execution without explicit container startup script modifications.
"""

import logging
import os
import sys
import traceback

from michelangelo.uniflow.core.file_sync import FsspecDownloader, file_sync_pre_run

# Run the file sync pre-run functionality automatically when this module is imported
if __name__ != "__main__":  # pragma: no cover
    # Initialize module-level logger
    log = logging.getLogger(__name__)
    log.propagate = False

    # Check if debug mode is enabled via environment variable
    debug_mode = os.environ.get("UF_FILE_SYNC_DEBUG", "").lower() in (
        "1",
        "true",
        "yes",
    )

    # Configure logger based on debug mode
    if debug_mode:
        log.setLevel(logging.INFO)
        handler = logging.StreamHandler(sys.stderr)
        handler.setFormatter(logging.Formatter("[sitecustomize] %(message)s"))
        log.addHandler(handler)

        # Print debug info
        log.info(f"Python executable: {sys.executable}")
        log.info(f"Working directory: {os.getcwd()}")
        log.info(
            "UF_FILE_SYNC_TARBALL_URL: "
            f"{os.environ.get('UF_FILE_SYNC_TARBALL_URL', 'NOT SET')}"
        )
    else:
        # Disable logger completely if not in debug mode
        log.disabled = True

    try:
        file_sync_pre_run(downloader=FsspecDownloader())
    except Exception as e:
        log.error(f"Error: {e}")
        log.error(f"Traceback: {traceback.format_exc()}")
        # Continue despite errors to avoid breaking containers
