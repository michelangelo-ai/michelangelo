load("@plugin", "storage")

def test_read():
    result, err = storage.read("s3", "default/d47efe2f682f4965bcf119f9d9a06eb1.json")
    if err == None:
        return result
    return None
