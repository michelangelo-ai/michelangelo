load("@plugin", "storage")

# Test storage.read with explicit storage provider parameter
def test():
    return storage.read("s3://bucket/file", storage_provider="aws-prod")