import michelangelo as ma


@ma.task(config="test")
def load_data():
    return [1, 2, 3]


if __name__ == "__main__":
    data = load_data()
    print("data:", data)
    print("ok.")
