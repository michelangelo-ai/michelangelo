"""Tests for pipeline metadata validation and type normalization."""

import unittest

from michelangelo.uniflow.core.pipeline_metadata import (
    PipelineMetadata,
    PipelineMetadataError,
    get_pipeline_metadata,
    normalize_pipeline_type,
)


class FakeCatalog:
    """Pipeline-type catalog with a custom, non-proto set of names."""

    def names(self) -> frozenset[str]:
        """Return the fake enum names."""
        return frozenset({"PIPELINE_TYPE_INVALID", "PIPELINE_TYPE_CUSTOM"})


class NormalizePipelineTypeTest(unittest.TestCase):
    """Tests for normalize_pipeline_type."""

    def test_accepts_short_full_and_any_case(self):
        """Short, full and mixed-case names normalize to the full enum name."""
        for value in ("train", "TRAIN", " Train ", "PIPELINE_TYPE_TRAIN"):
            with self.subTest(value=value):
                self.assertEqual("PIPELINE_TYPE_TRAIN", normalize_pipeline_type(value))

    def test_rejects_invalid_member(self):
        """PIPELINE_TYPE_INVALID is declared but never accepted."""
        with self.assertRaises(PipelineMetadataError) as ctx:
            normalize_pipeline_type("INVALID")
        self.assertIn("Expected one of:", str(ctx.exception))
        self.assertNotIn("INVALID,", str(ctx.exception).split("Expected")[1])

    def test_suggests_close_match(self):
        """A near-miss suggests the closest valid type."""
        with self.assertRaisesRegex(
            PipelineMetadataError,
            'type "TRAINING" is not a valid pipeline type; did you mean "TRAIN"\\?',
        ):
            normalize_pipeline_type("TRAINING")

    def test_uses_injected_catalog(self):
        """Valid names come from the catalog, not a hard-coded list."""
        self.assertEqual(
            "PIPELINE_TYPE_CUSTOM", normalize_pipeline_type("custom", FakeCatalog())
        )
        with self.assertRaisesRegex(PipelineMetadataError, "Expected one of: CUSTOM"):
            normalize_pipeline_type("TRAIN", FakeCatalog())


class PipelineMetadataTest(unittest.TestCase):
    """Tests for PipelineMetadata.validate and normalized."""

    def _valid(self, **overrides):
        values = {
            "name": "demo-pipeline",
            "namespace": "ma-examples",
            "owner": "jane.doe",
            "description": "A description with spaces.",
            "type": "TRAIN",
            "image": "ghcr.io/michelangelo-ai/examples:main",
            "git_ref": "master",
            "branch": "feature/x",
        }
        values.update(overrides)
        return PipelineMetadata(**values)

    def test_valid_metadata_has_no_errors(self):
        """Realistic metadata validates cleanly."""
        self.assertEqual([], self._valid().validate())

    def test_all_fields_optional(self):
        """Empty metadata is valid at declaration time."""
        self.assertEqual([], PipelineMetadata().validate())

    def test_rejects_invalid_names(self):
        """Names must be DNS-1123 subdomains."""
        for name in ("Upper", "under_score", "-leading", "trailing-", "a" * 254):
            with self.subTest(name=name):
                errors = self._valid(name=name).validate()
                self.assertEqual(1, len(errors))
                self.assertIn("DNS-1123 subdomain", errors[0])
        self.assertEqual([], self._valid(name="a.b-c").validate())

    def test_rejects_invalid_namespaces(self):
        """Namespaces must be DNS-1123 labels."""
        for namespace in ("with.dot", "Upper", "a" * 64):
            with self.subTest(namespace=namespace):
                errors = self._valid(namespace=namespace).validate()
                self.assertEqual(1, len(errors))
                self.assertIn("DNS-1123 label", errors[0])

    def test_rejects_whitespace_in_identifiers(self):
        """Owner, git_ref and branch must not contain whitespace."""
        for field in ("owner", "git_ref", "branch"):
            with self.subTest(field=field):
                errors = self._valid(**{field: "has space"}).validate()
                self.assertEqual(
                    [f'{field} "has space" must not contain whitespace'], errors
                )

    def test_rejects_empty_and_non_string_values(self):
        """Declared values must be non-empty strings."""
        self.assertEqual(
            ["description must not be empty"], self._valid(description=" ").validate()
        )
        self.assertEqual(
            ["owner must be a string, got int"], self._valid(owner=42).validate()
        )

    def test_reports_every_problem(self):
        """All problems are reported at once."""
        errors = self._valid(name="Bad", namespace="bad.ns", type="NOPE").validate()
        self.assertEqual(3, len(errors))

    def test_normalized_expands_type(self):
        """normalized() expands the type to the full enum name."""
        self.assertEqual(
            "PIPELINE_TYPE_TRAIN", self._valid(type="train").normalized().type
        )
        meta = PipelineMetadata(name="x")
        self.assertIs(meta, meta.normalized())


class GetPipelineMetadataTest(unittest.TestCase):
    """Tests for get_pipeline_metadata."""

    def test_returns_none_without_metadata(self):
        """Plain functions carry no metadata."""
        self.assertIsNone(get_pipeline_metadata(lambda: None))


_TRIGGER = {
    "cronSchedule": {"cron": "0 8 * * *"},
    "batchPolicy": {"batchSize": 1, "wait": "60s"},
    "parametersMap": {"cola": {"kwArgs": {"tokenizer_max_length": 128}}},
    "maxConcurrency": 1,
}
_NOTIFICATION = {
    "notificationType": "NOTIFICATION_TYPE_SLACK",
    "eventTypes": ["EVENT_TYPE_PIPELINE_RUN_STATE_FAILED"],
    "resourceType": "RESOURCE_TYPE_PIPELINE_RUN",
    "slackDestinations": ["#ml-alerts"],
}


class RecordingSchema:
    """MessageSchema that records checks and rejects one message type."""

    def __init__(self, reject=None):
        """Initialize with the message name to reject, if any."""
        self.reject = reject
        self.checked = []

    def check(self, message, value):
        """Record the check; reject ``self.reject`` messages."""
        self.checked.append(message)
        return "nope" if message == self.reject else None


class StructuredFieldsTest(unittest.TestCase):
    """Tests for labels, annotations, triggers and notifications."""

    def _errors(self, **fields):
        return PipelineMetadata(**fields).validate()

    def test_valid_structured_fields(self):
        """Realistic structured values validate against the real protos."""
        self.assertEqual(
            [],
            self._errors(
                image="img",
                labels={"team": "ml", "example.com/tier": "gold", "empty": ""},
                annotations={"example.com/note": "free text, any value"},
                triggers={"daily": _TRIGGER},
                notifications=[_NOTIFICATION],
            ),
        )

    def test_labels(self):
        """Label keys and values follow Kubernetes rules."""
        self.assertIn('labels key "Bad Key"', self._errors(labels={"Bad Key": "x"})[0])
        self.assertIn(
            'labels["team"] value "has space"',
            self._errors(labels={"team": "has space"})[0],
        )
        self.assertEqual(
            ["labels keys and values must be strings"], self._errors(labels={"n": 1})
        )
        self.assertEqual(
            ["labels must be a mapping, got list"], self._errors(labels=["team"])
        )

    def test_annotations(self):
        """Annotation keys are checked, values are free text, image must agree."""
        self.assertIn('annotations key "-x"', self._errors(annotations={"-x": "v"})[0])
        image_key = "michelangelo/uniflow-image"
        self.assertEqual([], self._errors(image="a", annotations={image_key: "a"}))
        self.assertEqual(
            [f'image "a" conflicts with annotations["{image_key}"] "b"; set only one'],
            self._errors(image="a", annotations={image_key: "b"}),
        )

    def test_triggers(self):
        """Triggers are checked against the Trigger proto, in camelCase."""
        errors = self._errors(triggers={"daily": {"cron_schedule": {"cron": "x"}}})
        self.assertEqual(
            [
                'triggers["daily"]: use "cronSchedule" instead of "cron_schedule" '
                "at cron_schedule"
            ],
            errors,
        )
        nested = {"parametersMap": {"cola": {"kw_args": {}}}}
        self.assertIn(
            'use "kwArgs" instead of "kw_args" at parametersMap.cola.kw_args',
            self._errors(triggers={"daily": nested})[0],
        )
        self.assertIn(
            'no field named "cronSchedul"',
            self._errors(triggers={"daily": {"cronSchedul": {}}})[0],
        )
        self.assertIn(
            "maxConcurrency",
            self._errors(triggers={"daily": {"maxConcurrency": "many"}})[0],
        )
        self.assertEqual(
            ['triggers["daily"] must be a mapping, got str'],
            self._errors(triggers={"daily": "0 8 * * *"}),
        )
        self.assertEqual(
            ["trigger names must be non-empty strings"],
            self._errors(triggers={"": _TRIGGER}),
        )

    def test_notifications(self):
        """Notifications are a list of Notification protos."""
        bad_enum = dict(_NOTIFICATION, notificationType="SLACK")
        self.assertIn(
            "notifications[0]: Failed to parse notificationType",
            self._errors(notifications=[bad_enum])[0],
        )
        self.assertEqual(
            ["notifications must be a list, got dict"],
            self._errors(notifications=_NOTIFICATION),
        )
        self.assertEqual(
            ["notifications[1] must be a mapping, got str"],
            self._errors(notifications=[_NOTIFICATION, "#ml-alerts"]),
        )

    def test_uses_injected_schema(self):
        """Message checks go through the injected schema."""
        schema = RecordingSchema(reject="Notification")
        meta = PipelineMetadata(triggers={"a": {}}, notifications=[{}, {}])
        self.assertEqual(
            ["notifications[0]: nope", "notifications[1]: nope"],
            meta.validate(schema=schema),
        )
        self.assertEqual(["Trigger", "Notification", "Notification"], schema.checked)

    def test_normalized_copies_containers(self):
        """normalized() detaches values from the caller's containers."""
        labels = {"team": "ml"}
        notifications = ({"emails": ("a@example.com",)},)
        meta = PipelineMetadata(labels=labels, notifications=notifications).normalized()
        labels["team"] = "changed"
        self.assertEqual({"team": "ml"}, meta.labels)
        self.assertEqual([{"emails": ["a@example.com"]}], meta.notifications)


class EdgeCaseTest(unittest.TestCase):
    """Tests for less common shapes and helpers."""

    def test_as_dict(self):
        """as_dict returns every field, None when unset."""
        data = PipelineMetadata(name="demo", labels={"team": "ml"}).as_dict()
        self.assertEqual("demo", data["name"])
        self.assertEqual({"team": "ml"}, data["labels"])
        self.assertIsNone(data["notifications"])

    def test_triggers_must_be_a_mapping(self):
        """A list of triggers is rejected before any proto check."""
        self.assertEqual(
            ["triggers must be a mapping, got list"],
            PipelineMetadata(triggers=["daily"]).validate(),
        )

    def test_label_key_prefix_must_be_a_dns_subdomain(self):
        """The prefix before '/' in a label key must be a DNS-1123 subdomain."""
        errors = PipelineMetadata(labels={"Bad_Prefix/team": "ml"}).validate()
        self.assertEqual(1, len(errors))
        self.assertIn('labels key "Bad_Prefix/team"', errors[0])

    def test_nested_non_mapping_is_left_to_the_proto_parser(self):
        """A scalar where a message is expected fails in the proto parser."""
        errors = PipelineMetadata(
            triggers={"daily": {"cronSchedule": "0 8 * * *"}}
        ).validate()
        self.assertEqual(1, len(errors))
        self.assertTrue(errors[0].startswith('triggers["daily"]: '))
        self.assertNotIn("instead of", errors[0])

    def test_repeated_messages_are_checked(self):
        """Repeated message fields are walked item by item."""
        rerun = {"pipelineRuns": [{"namespace": "ns", "name": "run-1"}]}
        self.assertEqual(
            [], PipelineMetadata(triggers={"rerun": {"batchRerun": rerun}}).validate()
        )
        rerun_snake = dict(rerun, resume_from="step")
        self.assertIn(
            'use "resumeFrom" instead of "resume_from" at batchRerun.resume_from',
            PipelineMetadata(
                triggers={"rerun": {"batchRerun": rerun_snake}}
            ).validate()[0],
        )

    def test_is_repeated_falls_back_to_label(self):
        """Before protobuf 7, repeated fields are detected through their label."""
        from types import SimpleNamespace

        from michelangelo.uniflow.core.pipeline_metadata import _is_repeated

        self.assertTrue(_is_repeated(SimpleNamespace(label=3, LABEL_REPEATED=3)))
        self.assertFalse(_is_repeated(SimpleNamespace(label=1, LABEL_REPEATED=3)))
