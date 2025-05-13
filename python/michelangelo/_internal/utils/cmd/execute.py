import subprocess


def execute_cmd(cmd: "list[str]") -> "tuple[bytes, bytes, int]":
    """
    Execute a command in a subprocess and return the output.

    Args:
        cmd: The command to execute.
             This should be a list of strings, where the first element is the command to execute.
    Returns:
        A tuple of bytes containing the standard output, standard error and exitcode of the command.
    """
    sp = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    out, err = sp.communicate()
    exitcode = sp.returncode
    return out, err, exitcode
