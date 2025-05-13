from retrying import retry
import logging
from uber.ai.michelangelo.shared.utils.cmd_utils import (
    execute_cmd,
    decode_output,
)
from uber.ai.michelangelo.shared.gateways.terrablob_gateway.common.error import (
    get_terrablob_error,
)
from uber.ai.michelangelo.shared.gateways.terrablob_gateway.common.option import TerrablobOptions
from uber.ai.michelangelo.shared.errors.terrablob_error import (
    TerrablobRetriableError,
    TerrablobError,
)

_logger = logging.getLogger(__name__)


@retry(stop_max_attempt_number=3, wait_random_min=1000, wait_random_max=2000)
def execute_terrablob_cmd(cmd: list[str]) -> tuple[bytes, bytes, int]:
    """
    Execute a tb-cli command in a subprocess and return the output.

    Args:
        cmd: The command to execute.
             This should be a list of strings, where the first element is the command to execute.
    Returns:
        A tuple of bytes containing the standard output and standard error of the command.
    """
    _logger.info(f"TerraBlob cmd to execute: {cmd}")
    out, err, exitcode = execute_cmd(cmd)
    if out:
        _logger.info(f"Standard output of the terrablob cmd:{cmd}, output:{out}")
    if err:
        _logger.info(f"Error output of the terrablob cmd:{cmd}, output:{err}")
    return out, err, exitcode


@retry(retry_on_exception=lambda e: isinstance(e, TerrablobRetriableError), stop_max_attempt_number=3, wait_exponential_multiplier=4000)
def execute_terrablob_cmd_with_exception(cmd: list[str], error_message: str) -> str:
    """
    Execute a tb-cli command in a subprocess and return the output. Throw exception if stderr is not empty.

    Args:
        cmd: The command to execute.
             This should be a list of strings, where the first element is the command to execute.
        error_message: The error message to log if an exception is thrown.

    Returns:
        A str of tb-cli standard output if no exception is thrown.

    Raises:
        TerrablobPermissionError: If the user does not have permission.
        TerrablobFileNotFoundError: If the file is not found.
        TerrablobFailedPreconditionError: If the precondition failed.
        TerrablobBadFileDescriptorError: If the tb-cli claims os_error:"Bad file descriptor". This is Retriable.
        TerrablobConnectionTimeoutError: If the tb-cli connection times out. This is Retriable.
        TerrablobError: As long as the exitcode is not 0 and none of the above errors are raised.
    """
    out, err, exitcode = execute_terrablob_cmd(cmd)

    message = decode_output(out)
    error = decode_output(err)

    if exitcode != 0:
        terrablob_error = get_terrablob_error(
            error,
            error_message,
        )
        if terrablob_error:
            raise terrablob_error
        raise TerrablobError(f"{error_message} Error: Unknown")

    return message


def construct_terrablob_cmd(cmd: list[str], options: TerrablobOptions) -> list[str]:
    """
    Construct a tb-cli command.

    Args:
        cmd: The base command to execute.
        options: The additional options for the command.
    Returns:
        A list of strings representing the tb-cli command.
    """
    res = list(cmd)

    if options.timeout:
        res.extend(["-t", options.timeout])

    if options.keepalive:
        res.append("-k")

    if options.source_entity:
        res.extend(["-a", options.source_entity])

    if options.is_staging:
        res.append("-s")

    if options.auth_mode:
        res.extend(["--auth-mode", options.auth_mode])

    return res
