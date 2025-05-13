from michelangelo.lib.model_manager._private.utils.env_utils import is_local


def get_download_multipart_options() -> dict:
    """
    Get multipart options for the download_from_terrablob function

    Returns:
        dict: The multipart options
    """
    return {"multipart": True, "keepalive": True} if is_local() else {}


def get_upload_multipart_options() -> dict:
    """
    Get multipart options for the upload_to_terrablob function

    Returns:
        dict: The multipart options
    """
    return {"multipart": True, "concurrency": 10, "keepalive": True} if is_local() else {}
