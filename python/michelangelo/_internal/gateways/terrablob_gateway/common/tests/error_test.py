from unittest import TestCase
from michelangelo._internal.gateways.terrablob_gateway.common import get_terrablob_error
from michelangelo._internal.errors.terrablob_error import (
    TerrablobError,
    TerrablobPermissionError,
    TerrablobFileNotFoundError,
    TerrablobFailedPreconditionError,
    TerrablobBadFileDescriptorError,
    TerrablobConnectionTimeoutError,
    TerrablobRetriableError,
    TerrablobConnectionError,
    TerrablobContextDeadlineExceededError,
)


class ErrorTest(TestCase):
    def test_get_terrablob_error(self):
        self.assertIsInstance(
            get_terrablob_error(
                "error code:permission-denied ...",
                "message",
            ),
            TerrablobPermissionError,
        )

        self.assertIsInstance(
            get_terrablob_error(
                "error code:not-found ...",
                "message",
            ),
            TerrablobFileNotFoundError,
        )

        self.assertIsInstance(
            get_terrablob_error(
                "error code:failed-precondition ...",
                "message",
            ),
            TerrablobFailedPreconditionError,
        )

        self.assertIsInstance(
            get_terrablob_error("error", "message"),
            TerrablobError,
        )

        self.assertIsInstance(
            get_terrablob_error('os_error:"Bad file descriptor"', "message"),
            TerrablobBadFileDescriptorError,
        )

        self.assertIsInstance(
            get_terrablob_error('os_error:"Bad file descriptor"', "message"),
            TerrablobRetriableError,
        )

        self.assertIsInstance(
            get_terrablob_error("reset reason: connection timeout", "message"),
            TerrablobConnectionTimeoutError,
        )

        self.assertIsInstance(
            get_terrablob_error(
                "code:unavailable message:closing transport due to: connection error",
                "message",
            ),
            TerrablobConnectionError,
        )

        self.assertIsInstance(
            get_terrablob_error("- context deadline exceeded", "message"),
            TerrablobContextDeadlineExceededError,
        )

        self.assertIsNone(
            get_terrablob_error("", "message"),
        )
