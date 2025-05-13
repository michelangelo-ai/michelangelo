import gzip
import shutil


def gzip_compress(src_file: str, dest_file: str):
    """
    Use gzip to compress existing files on disk.
    To compress data in memory, please use gzip.compress

    Args:
        src_file: the path of file to be compressed
        dest_file: the compressed file destination
    """
    with open(src_file, "rb") as f_in:
        with gzip.open(dest_file, "wb") as f_out:
            shutil.copyfileobj(f_in, f_out)


def gzip_decompress(src_file: str, dest_file: str):
    """
    Use gzip to decompress existing files on disk.
    To decompress data in memory, please use gzip.decompress

    Args:
        src_file: the path of file to be decompressed
        dest_file: the decompressed file destination
    """
    with gzip.open(src_file, "rb") as f_in:
        with open(dest_file, "wb") as f_out:
            shutil.copyfileobj(f_in, f_out)
