import os


def is_local() -> bool:
    """
    Guess if the current environment is local or not.

    Returns:
        bool: True if the current environment is local, False otherwise
    """
    _LOCAL_RUN = os.getenv("_LOCAL_RUN")

    return _LOCAL_RUN == "1"
