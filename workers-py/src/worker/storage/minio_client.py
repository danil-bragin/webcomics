"""MinIO/S3 upload helper."""
from __future__ import annotations

import io
import logging
from typing import IO

from minio import Minio
from minio.error import S3Error

log = logging.getLogger(__name__)


class ObjectStore:
    def __init__(self, endpoint: str, access_key: str, secret_key: str, bucket: str, secure: bool) -> None:
        self.client = Minio(endpoint, access_key=access_key, secret_key=secret_key, secure=secure)
        self.bucket = bucket
        try:
            if not self.client.bucket_exists(bucket):
                self.client.make_bucket(bucket)
        except S3Error:
            log.exception("bucket setup failed", extra={"bucket": bucket})

    def put_bytes(self, key: str, data: bytes, content_type: str) -> None:
        self.client.put_object(
            self.bucket,
            key,
            io.BytesIO(data),
            length=len(data),
            content_type=content_type,
        )

    def put_stream(self, key: str, stream: IO[bytes], length: int, content_type: str) -> None:
        self.client.put_object(self.bucket, key, stream, length=length, content_type=content_type)
