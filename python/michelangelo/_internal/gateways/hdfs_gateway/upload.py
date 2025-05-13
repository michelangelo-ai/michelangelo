import logging
from michelangelo._internal.errors.hdfs_error import HDFSError
from michelangelo._internal.utils.cmd import execute_cmd, decode_output

_logger = logging.getLogger(__name__)


def upload_to_hdfs(src_path: str, des_path: str):
    """
    Upload a file to HDFS

    Args:
        src_path: The source path
        des_path: The destination path in HDFS

    Returns:
        None
    """
    _logger.info(f"Uploading {src_path} to HDFS {des_path}")

    cmd = ["hdfs", "dfs", "-put", "-f", src_path, des_path]

    _logger.info(f"Executing command: {cmd}")
    out, err, exitcode = execute_cmd(cmd)
    if exitcode != 0:
        raise HDFSError(f"Error uploading {src_path} to HDFS {des_path}. Error: {decode_output(err)}")

    _logger.info(f"Successfully uploaded {src_path} to HDFS {des_path}")
