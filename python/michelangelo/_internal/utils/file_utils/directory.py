import contextlib
import os


@contextlib.contextmanager
def cd(newdir: str):
    """
    Changes the working directory.
    The original working directory is restored when the context manager exits.

    Args:
        newdir: the target working directory
    """
    prevdir = os.getcwd()
    os.chdir(os.path.expanduser(newdir))
    try:
        yield
    finally:
        os.chdir(prevdir)
