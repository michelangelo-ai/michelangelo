class TerrablobError(Exception):
    """
    Exception raised when an error occurs while in Terrablob.
    """


class TerrablobPermissionError(TerrablobError):
    """
    Exception raised when a permission error occurs when interacting with Terrablob.
    """


class TerrablobFileNotFoundError(TerrablobError):
    """
    Exception raised when a file or directory is not found in Terrablob.
    """


class TerrablobFailedPreconditionError(TerrablobError):
    """
    Exception raised when a precondition fails in Terrablob.
    """


class TerrablobRetriableError(TerrablobError):
    """
    Exception raised when a retriable error occurs in Terrablob.
    """


class TerrablobBadFileDescriptorError(TerrablobRetriableError):
    """
    Exception raised when a bad file descriptor error occurs in Terrablob.

    This error is often raised when sending lots of tb requests consecutively.
    Such error is likely to be transient and retry helps.
    """


class TerrablobConnectionTimeoutError(TerrablobRetriableError):
    """
    Exception raised when a connection timeout error occurs in Terrablob.
    """


class TerrablobConnectionError(TerrablobRetriableError):
    """
    Exception raised when a connection error occurs in Terrablob.
    """


class TerrablobContextDeadlineExceededError(TerrablobRetriableError):
    """
    Exception raised when a context deadline exceeded error occurs in Terrablob.
    Context deadline exceeded error is thrown when the server does not respond before the deadline is exceeded.
    Such error can sometime be transient and retry helps.
    Timeout in tb-cli is typically thrown as context cancelled error.
    See https://sg.uberinternal.com/code.uber.internal/uber-code/go-code/-/blob/src/code.uber.internal/storage/terrablob/client/cmd/tb-cli/internal/put.go?L110
    """
