import logging
from michelangelo._internal.errors.hdfs_error import HDFSError
from michelangelo._internal.utils.cmd_utils import execute_cmd, decode_output

_logger = logging.getLogger(__name__)


def download_from_hdfs(src_path: str, des_path: str):
    """
    Download a file from HDFS

    Args:
        src_path: The source path in HDFS
        des_path: The destination path

    Returns:
        None
    """
    _logger.info(f"Downloading from HDFS {src_path} to {des_path}")

    cmd = ["hdfs", "dfs", "-get", src_path, des_path]

    _logger.info(f"Executing command: {cmd}")
    out, err, exitcode = execute_cmd(cmd)
    if exitcode != 0:
        raise HDFSError(f"Error downloading from HDFS {src_path} to {des_path}. Error: {decode_output(err)}")

    _logger.info(f"Successfully downloaded from HDFS {src_path} to {des_path}")
