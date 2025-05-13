def decode_output(out: bytes) -> str:
    """
    Decode the output of a command to a string.
    Suppresses the error when the decode method is not successful.

    Args:
        out: The output to decode.
    Returns:
        The decoded output as a string.
    """
    try:
        out = out.decode("utf-8")
    except (UnicodeDecodeError, AttributeError):
        pass
    return out
