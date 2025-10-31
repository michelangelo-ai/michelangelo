load("@plugin", "storage")

# Test storage.read with empty storage provider parameter
def test():
    return storage.read("s3://bucket/file", storage_provider="")