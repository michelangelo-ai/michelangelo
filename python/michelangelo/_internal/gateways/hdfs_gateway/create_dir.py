import logging
from michelangelo._internal.errors.hdfs_error import HDFSError
from michelangelo._internal.utils.cmd import execute_cmd, decode_output

_logger = logging.getLogger(__name__)


def create_dir_in_hdfs(dir_path: str):
    """
    Create a directory in HDFS

    Args:
        dir_path: The directory path in HDFS

    Returns:
        None
    """
    _logger.info(f"Creating directory in HDFS {dir_path}")

    cmd = ["hdfs", "dfs", "-mkdir", "-p", dir_path]

    _logger.info(f"Executing command: {cmd}")
    out, err, exitcode = execute_cmd(cmd)
    if exitcode != 0:
        raise HDFSError(f"Error creating directory in HDFS {dir_path}. Error: {decode_output(err)}")

    _logger.info(f"Successfully created directory in HDFS {dir_path}")
