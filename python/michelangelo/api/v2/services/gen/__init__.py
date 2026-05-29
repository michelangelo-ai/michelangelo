import importlib
import re

_SERVICE_PATTERN = re.compile(r"(?<!^)(?=[A-Z])")


def _service_names():
    return [
        k
        for k in ServicesGen.__dict__
        if not k.startswith("__") and k.endswith("Service")
    ]


def _wire(target, context):
    """Instantiate each service stub and attach it to *target*.

    *target* may be a class (singleton path) or an instance (per-instance
    path). Instance attributes shadow class attributes, so per-instance stubs
    take precedence on the instance while the class-level singleton stubs
    remain accessible via the class.
    """
    for service in _service_names():
        crd = _SERVICE_PATTERN.sub("_", service).lower().rpartition("_service")[0]
        m = importlib.import_module("michelangelo.api.v2.services.gen.{}".format(crd))
        setattr(target, service, getattr(m, service)(context))


class ServicesGen(object):
    CachedOutputService = None
    ModelService = None
    ModelFamilyService = None
    PipelineService = None
    PipelineRunService = None
    ProjectService = None
    RayClusterService = None
    RayJobService = None
    SparkJobService = None
    TriggerRunService = None

    @classmethod
    def init(cls, context):
        """Wire all service stubs onto *cls* as class-level attributes (singleton path)."""
        _wire(cls, context)

    @staticmethod
    def init_instance(obj, context):
        """Wire all service stubs onto *obj* as instance-level attributes.

        Instance attributes shadow the class-level ones, so ``obj.ModelService``
        returns the per-instance stub while ``APIClient.ModelService`` still
        returns the class-level singleton stub — backward compat is preserved.
        """
        _wire(obj, context)
