import uuid
from datetime import datetime, timezone


def generate_random_name(prefix):
    """Generate object name following k8s and Michelangelo api conventions
    """
    if len(prefix) == 0:
        raise RuntimeError("Prefix cannot be empty.")
    if len(prefix) > 128:
        raise RuntimeError("Prefix cannot have more than 128 characters.")

    prefix = prefix.lower().replace("_", "-")
    t = datetime.now(timezone.utc)
    return "{prefix}-{ts}-{rand_char}".format(
        prefix=prefix,
        ts=datetime.strftime(t, "%Y%m%d-%H%M%S"),
        rand_char=str(uuid.uuid4()).split("-")[0],
    )
