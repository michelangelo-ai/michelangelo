#!/usr/bin/env python3
import argparse
import sys
import ray
import fsspec
def main():

    ray.init(logging_level="DEBUG")
    # fs = fsspec.filesystem(
    #     "s3",
    #     key="1inaX9pX7vIeaJf3",
    #     secret="2OuOk0lGBLM1W9lrRzEBPiOoxZZBq6o0",
    #     client_kwargs={"endpoint_url": "http://localhost:9001"}
    # )

    url = 's3://default/test'
    credentials = {
        "key": "1inaX9pX7vIeaJf3",
        "secret": "2OuOk0lGBLM1W9lrRzEBPiOoxZZBq6o0",
        "client_kwargs": {"endpoint_url": f'http://localhost:9001'},
        "skip_instance_cache": True,
    }

    fs, path = fsspec.core.url_to_fs(url, **credentials)
    #
    # try:
    #     files = fs.ls("s3://default/test")
    #     print("Uploaded files:")
    #     for file in files:
    #         print(file)
    # except Exception as e:
    #     print(f"Error verifying upload: {e}")
    #     exit()
    #
    # import pyarrow.parquet as pq
    # for file in files:
    #     if file.endswith(".parquet"):
    #         try:
    #             with fs.open(file, "rb") as f:
    #                 table = pq.read_table(f)
    #                 print(f"Contents of {file}:")
    #                 print(table.to_pandas())
    #         except Exception as e:
    #             print(f"Error reading Parquet file {file}: {e}")

    import pyarrow.fs
    # Configure PyArrow's S3FileSystem for MinIO
    s3fs = pyarrow.fs.S3FileSystem(
        access_key="1inaX9pX7vIeaJf3",
        secret_key="2OuOk0lGBLM1W9lrRzEBPiOoxZZBq6o0",
        endpoint_override="http://localhost:9001"
    )
    data = ray.data.from_items([{"col1": 1}, {"col1": 2}, {"col1": 3}])
    data.write_parquet(url, filesystem=s3fs)

    # fs, path = fsspec.core.url_to_fs(url, **credentials)
    data = ray.data.read_parquet(path, filesystem=s3fs)
    print(data)
    return


if __name__ == "__main__":
    sys.exit(main())
