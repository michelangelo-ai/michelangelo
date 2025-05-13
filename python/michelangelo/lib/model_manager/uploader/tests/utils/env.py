import os


def mimic_local_env():
    os.environ["UBER_LDAP_UID"] = "test"
    os.environ["UBER_OWNER"] = "test@uber.com"
    if "UF_TASK_IMAGE" in os.environ:
        del os.environ["UF_TASK_IMAGE"]


def mimic_remote_env():
    if "UBER_LDAP_UID" in os.environ:
        del os.environ["UBER_LDAP_UID"]
