import importlib
import re


class ServicesGen(object):
    CachedOutputService = None
    DeploymentService = None
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
        services = filter(lambda x: not x.startswith('__') and x.endswith('Service'), cls.__dict__.keys())

        pattern = re.compile(r'(?<!^)(?=[A-Z])')
        for service in services:
            crd = pattern.sub('_', service).lower().rpartition('_service')[0]
            m = importlib.import_module('michelangelo.api.v2.services.gen.{}'.format(crd))
            setattr(cls, service, getattr(m, service)(context))
