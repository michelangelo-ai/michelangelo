import os


def get_terrablob_auth_mode() -> str:
    """
    Get the Terrablob authentication mode.

    Returns:
        The Terrablob authentication mode.
    """

    UBER_LDAP_UID = os.getenv("UBER_LDAP_UID")
    UBER_OWNER = os.getenv("UBER_OWNER")

    # tb-cli uses this pattern to determine if the user is on developer laptop
    # and use usso auth mode when it's true.
    # https://sg.uberinternal.com/code.uber.internal/uber-code/go-code/-/blob/src/code.uber.internal/storage/terrablob/client/cmd/tb-cli/internal/root.go?L56
    # However, this is not accurate, need to add more checks for uniflow env vars
    if UBER_LDAP_UID and UBER_OWNER and UBER_LDAP_UID == UBER_OWNER.split("@")[0] and os.getenv("UF_TASK_IMAGE"):
        return "legacy"

    # use the default auth mode for all the other cases
    return None
