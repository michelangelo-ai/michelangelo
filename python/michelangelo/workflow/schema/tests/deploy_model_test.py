"""Tests for michelangelo.workflow.schema.deploy_model config dataclasses."""

from __future__ import annotations

import dataclasses
import json
from unittest import TestCase

from michelangelo.uniflow.core.codec import DataclassCodec, EnumCodec
from michelangelo.workflow.schema import deploy_model as dm
from michelangelo.workflow.schema.deploy_model import (
    BlastUpdateStrategy,
    DeploymentDefinition,
    DeployModelConfig,
    DeviceIntegrityLevel,
    DisaggregatedUpdateStep,
    DisaggregatedUpdateStrategy,
    Duration,
    InferenceServerTarget,
    LabelSelector,
    LabelSelectorRequirement,
    MobileTarget,
    RedBlackUpdateStrategy,
    ResourceIdentifier,
    RollingUpdateStrategy,
    ShadowSpec,
    TargetType,
    UserInfo,
    ZonalUpdateStrategy,
)


def _full_config() -> DeployModelConfig:
    return DeployModelConfig(
        deployment_name="ranker",
        model_name="ranker-v3",
        namespace="proj",
        inference_server=InferenceServerTarget(namespace="proj", name="srv"),
        mobile=MobileTarget(level=DeviceIntegrityLevel.STRONG),
        zonal_strategy=ZonalUpdateStrategy(rollout_period_in_seconds=30.5),
        blast_strategy=BlastUpdateStrategy(
            with_rollback_trigger=True, issue_link="https://example.com/issues/1"
        ),
        rolling_strategy=RollingUpdateStrategy(increment_percentage=25),
        red_black_strategy=RedBlackUpdateStrategy(zonal_parallel=True),
        disaggregated_strategy=DisaggregatedUpdateStrategy(
            steps=[DisaggregatedUpdateStep(environment="prod", strategy="rolling")]
        ),
        definition=DeploymentDefinition(
            type=TargetType.TARGET_TYPE_INFERENCE_SERVER, sub_type="realtime"
        ),
        owner=UserInfo(name="ml-platform", proxy_user="svc"),
        selector=LabelSelector(
            match_labels={"zone": "a"},
            match_expressions=[
                LabelSelectorRequirement(key="tier", operator="In", values=["x"])
            ],
        ),
        model_family=ResourceIdentifier(namespace="proj", name="family"),
        shadow_spec=ShadowSpec(
            traffic_percentage=10.0,
            duration=Duration(seconds=60, nanos=5),
            enabled=True,
        ),
        resource_links={"dashboard": "https://example.com/d"},
        stage="staging",
        additional_labels={"team": "ml"},
        wait=True,
        timeout_seconds=600,
        deployment_template="template-deployment",
    )


def _minimal_config() -> DeployModelConfig:
    return DeployModelConfig(deployment_name="d", model_name="m", namespace="ns")


class TestEnums(TestCase):
    """Tests for the schema enums."""

    def test_device_integrity_level_values(self):
        """It exposes STRONG and STANDARD as string values."""
        self.assertEqual(DeviceIntegrityLevel.STRONG, "STRONG")
        self.assertEqual(DeviceIntegrityLevel.STANDARD.value, "STANDARD")
        self.assertIsInstance(DeviceIntegrityLevel.STRONG, str)

    def test_target_type_values(self):
        """It exposes the inference server and mobile target types."""
        self.assertEqual(
            TargetType.TARGET_TYPE_INFERENCE_SERVER, "TARGET_TYPE_INFERENCE_SERVER"
        )
        self.assertEqual(TargetType.TARGET_TYPE_MOBILE.value, "TARGET_TYPE_MOBILE")

    def test_enums_construct_from_value(self):
        """It looks members up by value and rejects unknown values."""
        self.assertIs(DeviceIntegrityLevel("STANDARD"), DeviceIntegrityLevel.STANDARD)
        with self.assertRaises(ValueError):
            TargetType("nope")


class TestDefaults(TestCase):
    """Tests for field defaults."""

    def test_minimal_config_defaults(self):
        """Only the three required fields are needed; the rest take defaults."""
        cfg = _minimal_config()
        self.assertEqual(cfg.stage, "production")
        self.assertFalse(cfg.wait)
        self.assertEqual(cfg.timeout_seconds, 72000)
        self.assertIsNone(cfg.deployment_template)
        for name in (
            "inference_server",
            "mobile",
            "zonal_strategy",
            "blast_strategy",
            "rolling_strategy",
            "red_black_strategy",
            "disaggregated_strategy",
            "definition",
            "owner",
            "selector",
            "model_family",
            "shadow_spec",
            "resource_links",
            "additional_labels",
        ):
            with self.subTest(field=name):
                self.assertIsNone(getattr(cfg, name))

    def test_required_fields_enforced(self):
        """It raises TypeError when a required field is missing."""
        with self.assertRaises(TypeError):
            DeployModelConfig(deployment_name="d", model_name="m")  # type: ignore[call-arg]
        with self.assertRaises(TypeError):
            RollingUpdateStrategy()  # type: ignore[call-arg]

    def test_optional_leaf_defaults(self):
        """Leaf dataclasses default their optional fields to None."""
        self.assertIsNone(ZonalUpdateStrategy().rollout_period_in_seconds)
        self.assertIsNone(BlastUpdateStrategy().with_rollback_trigger)
        self.assertIsNone(BlastUpdateStrategy().issue_link)
        self.assertIsNone(RedBlackUpdateStrategy().zonal_parallel)
        self.assertIsNone(Duration().seconds)
        self.assertIsNone(Duration().nanos)
        self.assertIsNone(ShadowSpec().duration)
        self.assertIsNone(DeploymentDefinition().type)
        self.assertIsNone(UserInfo(name="u").proxy_user)
        self.assertIsNone(LabelSelectorRequirement(key="k", operator="Exists").values)
        self.assertIsNone(LabelSelector().match_labels)


class TestSchemaShape(TestCase):
    """Tests for conventions required by the UniFlow codec."""

    def test_all_exports_resolve(self):
        """Every name in __all__ exists on the module."""
        for name in dm.__all__:
            with self.subTest(name=name):
                self.assertTrue(hasattr(dm, name))

    def test_no_init_false_fields_or_mutable_defaults(self):
        """Dataclass fields are all init fields with no mutable defaults."""
        for name in dm.__all__:
            obj = getattr(dm, name)
            if not dataclasses.is_dataclass(obj):
                continue
            for f in dataclasses.fields(obj):
                with self.subTest(cls=name, field=f.name):
                    self.assertTrue(f.init)
                    self.assertIs(f.default_factory, dataclasses.MISSING)
                    self.assertNotIsInstance(f.default, (list, dict, set))

    def test_excludes_unsupported_fields(self):
        """The config does not define api_version, deployment_kind or is_async."""
        names = {f.name for f in dataclasses.fields(DeployModelConfig)}
        self.assertTrue(
            {"api_version", "deployment_kind", "is_async"}.isdisjoint(names)
        )

    def test_blast_strategy_has_issue_link(self):
        """BlastUpdateStrategy exposes issue_link."""
        blast = {f.name for f in dataclasses.fields(BlastUpdateStrategy)}
        self.assertIn("issue_link", blast)


class TestCodecRoundTrip(TestCase):
    """Tests that every schema class round-trips through the UniFlow codec."""

    def setUp(self):
        """Create codecs."""
        self.codec = DataclassCodec()
        self.enum_codec = EnumCodec()

    def test_minimal_config(self):
        """A minimal config survives encode/decode."""
        cfg = _minimal_config()
        self.assertEqual(self.codec.decode(self.codec.encode(cfg)), cfg)

    def test_full_config(self):
        """A fully populated config, including nested types, survives."""
        cfg = _full_config()
        decoded = self.codec.decode(self.codec.encode(cfg))
        self.assertEqual(decoded, cfg)
        self.assertIsInstance(decoded, DeployModelConfig)

    def test_encoded_form_is_json_serializable_leaf(self):
        """The top-level encoded dict is JSON-serializable for a flat config."""
        encoded = self.codec.encode(_minimal_config())
        self.assertIsInstance(json.dumps(encoded, default=str), str)

    def test_each_leaf_class(self):
        """Each dataclass round-trips on its own."""
        leaves = (
            InferenceServerTarget(namespace="a", name="b"),
            MobileTarget(level=DeviceIntegrityLevel.STANDARD),
            ZonalUpdateStrategy(rollout_period_in_seconds=1.5),
            BlastUpdateStrategy(with_rollback_trigger=False, issue_link="x"),
            RollingUpdateStrategy(increment_percentage=50),
            RedBlackUpdateStrategy(zonal_parallel=False),
            DisaggregatedUpdateStep(environment="e", strategy="s"),
            DisaggregatedUpdateStrategy(
                steps=[DisaggregatedUpdateStep(environment="e", strategy="s")]
            ),
            LabelSelectorRequirement(key="k", operator="NotIn", values=["a", "b"]),
            LabelSelector(match_labels={"a": "b"}),
            ResourceIdentifier(namespace="a", name="b"),
            Duration(seconds=3, nanos=4),
            ShadowSpec(traffic_percentage=5.0, duration=Duration(seconds=1)),
            DeploymentDefinition(type=TargetType.TARGET_TYPE_MOBILE, sub_type="s"),
            UserInfo(name="u", proxy_user="p"),
        )
        for obj in leaves:
            with self.subTest(cls=type(obj).__name__):
                self.assertEqual(self.codec.decode(self.codec.encode(obj)), obj)

    def test_enum_round_trip(self):
        """Enums round-trip through the enum codec."""
        for member in (*DeviceIntegrityLevel, *TargetType):
            with self.subTest(member=member):
                self.assertTrue(self.enum_codec.can_encode(member))
                self.assertIs(
                    self.enum_codec.decode(self.enum_codec.encode(member)), member
                )

    def test_asdict_round_trip_minimal(self):
        """dataclasses.asdict()/cls(**dct) round-trips scalar-only configs."""
        cfg = _minimal_config()
        self.assertEqual(DeployModelConfig(**dataclasses.asdict(cfg)), cfg)
