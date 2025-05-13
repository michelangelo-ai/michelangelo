from unittest import TestCase
from uber.ai.michelangelo.shared.errors.terrablob_error import (
    TerrablobError,
    TerrablobPermissionError,
    TerrablobFileNotFoundError,
    TerrablobFailedPreconditionError,
    TerrablobRetriableError,
    TerrablobBadFileDescriptorError,
    TerrablobConnectionTimeoutError,
    TerrablobConnectionError,
    TerrablobContextDeadlineExceededError,
)


class TerrablobErrorTest(TestCase):
    def test_terrablob_error(self):
        with self.assertRaises(TerrablobError):
            raise TerrablobError("test")

    def test_terrablob_permission_error(self):
        with self.assertRaises(TerrablobPermissionError):
            raise TerrablobPermissionError("test")

    def test_terrablob_file_not_found_error(self):
        with self.assertRaises(TerrablobFileNotFoundError):
            raise TerrablobFileNotFoundError("test")

    def test_terrablob_failed_precondition_error(self):
        with self.assertRaises(TerrablobFailedPreconditionError):
            raise TerrablobFailedPreconditionError("test")

    def test_terrablob_bad_file_descriptor_error(self):
        with self.assertRaises(TerrablobBadFileDescriptorError):
            raise TerrablobBadFileDescriptorError("test")

    def test_terrablob_connection_time_out_error(self):
        with self.assertRaises(TerrablobConnectionTimeoutError):
            raise TerrablobConnectionTimeoutError("test")

    def test_terrablob_retriable_error(self):
        with self.assertRaises(TerrablobRetriableError):
            raise TerrablobRetriableError("test")

    def test_terrablob_retriable_error_inheritance(self):
        with self.assertRaises(TerrablobRetriableError):
            raise TerrablobBadFileDescriptorError("test")

    def test_terrablob_connection_error(self):
        with self.assertRaises(TerrablobConnectionError):
            raise TerrablobConnectionError("test")

    def test_terrablob_context_deadline_exceeded_error(self):
        with self.assertRaises(TerrablobContextDeadlineExceededError):
            raise TerrablobContextDeadlineExceededError("test")
