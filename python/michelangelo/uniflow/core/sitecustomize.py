"""sitecustomize.py - Automatic Uniflow initialization for remote environments.

This module is automatically imported by Python during startup when it's in the
Python path. It runs the uniflow file sync to download and apply local code
changes in remote containers.

This leverages Python's built-in site initialization mechanism for clean,
automatic execution without explicit container startup script modifications.
"""

from michelangelo.uniflow.core import file_sync

file_sync.run(downloader=file_sync.FsspecDownloader())
