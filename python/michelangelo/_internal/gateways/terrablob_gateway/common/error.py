from michelangelo._internal.errors.terrablob_error import (
    TerrablobError,
    TerrablobPermissionError,
    TerrablobFileNotFoundError,
    TerrablobFailedPreconditionError,
    TerrablobBadFileDescriptorError,
    TerrablobConnectionTimeoutError,
    TerrablobConnectionError,
    TerrablobContextDeadlineExceededError,
)


def get_terrablob_error(
    cmd_error: str,
    error_message: str,
) -> TerrablobError:
    """
    Get the Terrablob error object based on the error message
    from the Terrablob command.

    Args:
        cmd_error: The error message from the Terrablob command.
        error_message: The error message to display.

    Returns:
        The Terrablob error object.
        If cmd_error is not truethy, return None.
    """
    if cmd_error and "code:permission-denied" in cmd_error:
        return TerrablobPermissionError(f"{error_message}. Error: {cmd_error}")

    if cmd_error and "code:not-found" in cmd_error:
        return TerrablobFileNotFoundError(f"{error_message}. Error: {cmd_error}")

    if cmd_error and "code:failed-precondition" in cmd_error:
        return TerrablobFailedPreconditionError(f"{error_message}. Error: {cmd_error}")

    if cmd_error and 'os_error:"Bad file descriptor"' in cmd_error:
        return TerrablobBadFileDescriptorError(f"{error_message}. Error: {cmd_error}")

    if cmd_error and "reset reason: connection timeout" in cmd_error:
        return TerrablobConnectionTimeoutError(f"{error_message}. Error: {cmd_error}")

    if (
        cmd_error
        and "code:unavailable message:closing transport due to: connection error"
        in cmd_error
    ):
        return TerrablobConnectionError(f"{error_message}. Error: {cmd_error}")

    if cmd_error and "context deadline exceeded" in cmd_error:
        return TerrablobContextDeadlineExceededError(
            f"{error_message}. Error: {cmd_error}"
        )

    if cmd_error:
        return TerrablobError(f"{error_message}. Error: {cmd_error}")

    return None
