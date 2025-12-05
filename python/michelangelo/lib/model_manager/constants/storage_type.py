"""Storage type constants."""


class StorageType:
    """Storage type constants for model storage locations.

    This class defines constants for different storage backends that can be
    used to store and retrieve model files.

    Attributes:
        LOCAL: Local filesystem storage. Model files are stored on the local
            disk accessible to the current process.
    """

    LOCAL = "local"
