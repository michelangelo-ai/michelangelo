load("@plugin", "storage")

# Test storage.read with invalid URL to trigger error
def test():
    return storage.read("invalid://url", storage_provider="")