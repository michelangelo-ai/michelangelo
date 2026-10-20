"""Tests for michelangelo.uniflow.plugins.ray.data_context."""

from __future__ import annotations

from typing import ClassVar
from unittest import TestCase
from unittest.mock import patch

import ray

from michelangelo.uniflow.plugins.ray.data_context import (
    RETRIED_IO_ERRORS,
    set_ray_data_context,
)


class SetRayDataContextTest(TestCase):
    """Tests for set_ray_data_context."""

    def setUp(self):
        """Reset Ray's DataContext to defaults before each test."""
        ray.data.DataContext._set_current(ray.data.DataContext())

    def tearDown(self):
        """Reset Ray's DataContext to defaults after each test."""
        ray.data.DataContext._set_current(ray.data.DataContext())

    def test_no_args_leaves_block_sizes_untouched(self):
        """Calling with no arguments leaves Ray's default block sizes."""
        ctx = ray.data.DataContext.get_current()
        default_min = ctx.target_min_block_size
        default_max = ctx.target_max_block_size
        set_ray_data_context()
        self.assertEqual(ctx.target_min_block_size, default_min)
        self.assertEqual(ctx.target_max_block_size, default_max)

    def test_block_sizes_are_applied_when_given(self):
        """Explicit min/max block sizes override Ray's defaults."""
        set_ray_data_context(min_block_size=1024, max_block_size=2048)
        ctx = ray.data.DataContext.get_current()
        self.assertEqual(ctx.target_min_block_size, 1024)
        self.assertEqual(ctx.target_max_block_size, 2048)

    def test_default_retried_io_errors_are_appended(self):
        """With no override, the module's default retry patterns are appended."""
        ctx = ray.data.DataContext.get_current()
        before = list(ctx.retried_io_errors)
        set_ray_data_context()
        for pattern in RETRIED_IO_ERRORS:
            self.assertIn(pattern, ctx.retried_io_errors)
        self.assertEqual(
            len(ctx.retried_io_errors), len(before) + len(RETRIED_IO_ERRORS)
        )

    def test_custom_retried_io_errors_are_appended_instead(self):
        """A custom retried_io_errors list replaces the module defaults."""
        ctx = ray.data.DataContext.get_current()
        before = list(ctx.retried_io_errors)
        set_ray_data_context(retried_io_errors=["my custom error"])
        self.assertIn("my custom error", ctx.retried_io_errors)
        for pattern in RETRIED_IO_ERRORS:
            self.assertNotIn(pattern, ctx.retried_io_errors)
        self.assertEqual(len(ctx.retried_io_errors), len(before) + 1)

    def test_empty_retried_io_errors_list_appends_nothing(self):
        """An explicit empty list skips appending any extra retry patterns."""
        ctx = ray.data.DataContext.get_current()
        before = list(ctx.retried_io_errors)
        set_ray_data_context(retried_io_errors=[])
        self.assertEqual(ctx.retried_io_errors, before)

    def test_object_store_memory_limit_sets_resource_limits(self):
        """A memory limit is applied to the execution options' resource limits."""
        set_ray_data_context(object_store_memory_limit=512)
        ctx = ray.data.DataContext.get_current()
        self.assertEqual(ctx.execution_options.resource_limits.object_store_memory, 512)

    def test_wait_for_min_actors_s_is_applied(self):
        """An explicit actor-warmup wait is applied to the context."""
        set_ray_data_context(wait_for_min_actors_s=30)
        ctx = ray.data.DataContext.get_current()
        self.assertEqual(ctx.wait_for_min_actors_s, 30)

    def test_max_blocks_in_streaming_gen_buffer_is_applied(self):
        """An explicit buffer size is applied to the context."""
        set_ray_data_context(max_blocks_in_streaming_gen_buffer=1)
        ctx = ray.data.DataContext.get_current()
        self.assertEqual(ctx._max_num_blocks_in_streaming_gen_buffer, 1)

    def test_max_blocks_in_streaming_gen_buffer_default_untouched(self):
        """Leaving the argument unset keeps Ray's default buffer size."""
        ctx = ray.data.DataContext.get_current()
        default = ctx._max_num_blocks_in_streaming_gen_buffer
        set_ray_data_context()
        self.assertEqual(ctx._max_num_blocks_in_streaming_gen_buffer, default)

    def test_max_blocks_in_streaming_gen_buffer_warns_when_unsupported(self):
        """A Ray without the private attribute logs a warning and skips it."""

        class _ContextWithoutAttr:
            """Stand-in DataContext lacking the private buffer attribute."""

            target_min_block_size = 0
            target_max_block_size = 0
            retried_io_errors: ClassVar[list] = []

        target = (
            "michelangelo.uniflow.plugins.ray.data_context.ray.data.DataContext"
            ".get_current"
        )
        logger_name = "michelangelo.uniflow.plugins.ray.data_context"
        stub = _ContextWithoutAttr()
        with (
            patch(target, return_value=stub),
            self.assertLogs(logger_name, level="WARNING") as logs,
        ):
            set_ray_data_context(
                retried_io_errors=[], max_blocks_in_streaming_gen_buffer=1
            )
        self.assertIn("max_blocks_in_streaming_gen_buffer", logs.output[0])
        self.assertFalse(
            hasattr(stub, "_max_num_blocks_in_streaming_gen_buffer"),
            "the setting must be skipped, not set on a context that lacks it",
        )

    def test_repeated_calls_do_not_duplicate_retried_io_errors(self):
        """Calling twice with overlapping patterns does not grow the list unboundedly.

        DataContext is a process-global singleton, so a driver process that
        calls this more than once (retries, multiple pipeline steps) must not
        accumulate duplicate patterns indefinitely.
        """
        ctx = ray.data.DataContext.get_current()
        before = list(ctx.retried_io_errors)
        set_ray_data_context(retried_io_errors=["shared error", "first-only error"])
        set_ray_data_context(retried_io_errors=["shared error", "second-only error"])
        self.assertEqual(
            ctx.retried_io_errors,
            [*before, "shared error", "first-only error", "second-only error"],
        )
