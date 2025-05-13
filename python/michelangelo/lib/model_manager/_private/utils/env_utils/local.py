import os


def is_local() -> bool:
    """
    Guess if the current environment is local or not.

    Returns:
        bool: True if the current environment is local, False otherwise
    """

    UBER_LDAP_UID = os.getenv("UBER_LDAP_UID")
    UBER_OWNER = os.getenv("UBER_OWNER")
    UF_TASK_IMAGE = os.getenv("UF_TASK_IMAGE")

    return UBER_LDAP_UID and UBER_OWNER and UBER_LDAP_UID == UBER_OWNER.split("@")[0] and not UF_TASK_IMAGE
