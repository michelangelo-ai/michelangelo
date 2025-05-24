load("@plugin", "storage")

def test_read():
    result = storage.read("s3", "default/d47efe2f682f4965bcf119f9d9a06eb1.json")
    if len(result) == 2 and result[1]:
        return None
    return result[0]
