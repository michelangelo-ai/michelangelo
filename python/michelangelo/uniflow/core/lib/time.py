import time as py_time

from michelangelo.uniflow.core import star_plugin, workflow


@star_plugin("time.sleep")
def sleep(seconds: float):
    """
    Suspends execution of the calling thread for the given number of seconds.
    The argument may be a floating point number to indicate a more precise sleep time.

    Parameters:
        seconds (float): The number of seconds to sleep.
    Returns:
        None
    """
    py_time.sleep(seconds)


@star_plugin("time.time")
def time() -> float:
    """
    Returns the current unix time in seconds as floating point number.
    Fractions of a second may be present if the system clock provides them.
    Returns:
        float
    """
    return py_time.time()


@star_plugin("time.utc_format_seconds")
def utc_format_seconds(pattern: str, seconds: float) -> str:
    """
    Converts the given unix time in seconds to a string as specified by the format argument.
    The formatted result string represents the UTC time.

    Parameters:
        pattern (string): The format string containing the date and time directives such as %Y, %m, %d, %H, %M, %S.
        seconds (float): The unix time in seconds. Fractions of a second may be present.

    Returns:
        str - Formatted UTC time
    """
    t = py_time.gmtime(seconds)
    return py_time.strftime(pattern, t)


@workflow()
def datestr(days_offset: int = 0):
    """
    Returns the current UTC date in the format 'YYYY-MM-DD' with an optional offset in days.

    Parameters:
        days_offset (int): The number of days to offset from the current date. Default is 0.

    Returns:
        str - UTC date in the format 'YYYY-MM-DD'
    """
    epoch_seconds = time() + 86400 * days_offset
    return utc_format_seconds("%Y-%m-%d", epoch_seconds)
