from michelangelo.gen.api.v2.pipeline_svc_pb2_grpc import PipelineServiceStub
from michelangelo.gen.api.v2.pipeline_svc_pb2 import (
    CreatePipelineRequest,
    GetPipelineRequest,
    UpdatePipelineRequest,
    DeletePipelineRequest,
    DeletePipelineCollectionRequest,
    ListPipelineRequest,
)
from michelangelo.gen.api.list_pb2 import CriterionOperation, ListOptionsExt
from michelangelo.gen.k8s.io.apimachinery.pkg.apis.meta.v1.generated_pb2 import (
    CreateOptions,
    GetOptions,
    UpdateOptions,
    DeleteOptions,
    ListOptions,
)

from ..base import BaseService, _TIMEOUT_SECONDS


class PipelineService(BaseService):

    def __init__(self, context):
        super(PipelineService, self).__init__(context, PipelineServiceStub)

    def create_pipeline(self, pipeline, create_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Create pipeline

        :param pipeline: pipeline object
        :type pipeline: Pipeline
        :param create_options: create options
        :type create_options: Optional[Union[CreateOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: created pipeline
        :rtype: Pipeline

        :example:

        >>> from michelangelo.gen.api.v2.pipeline_pb2 import Pipeline
        >>> pipeline = Pipeline()
        >>> pipeline.namespace = 'my-project'
        >>> pipeline.name = 'my-pipeline'
        >>> APIClient.PipelineService.create_pipeline(pipeline)
        """
        req = CreatePipelineRequest(pipeline=pipeline)
        create_options = self._process_message_or_dict(create_options, CreateOptions)
        req.create_options.CopyFrom(create_options)
        resp = self._stub.CreatePipeline(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.pipeline

    def get_pipeline(self, namespace, name, get_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Get pipeline

        :param namespace: project name
        :type namespace: str
        :param name: pipeline object name
        :type name: str
        :param get_options: get options
        :type get_options: Optional[Union[GetOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: pipeline
        :rtype: Pipeline

        :example:

        >>> pipeline = APIClient.PipelineService.get_pipeline(namespace='my-project', name='my-object')
        """
        req = GetPipelineRequest(name=name, namespace=namespace)
        get_options = self._process_message_or_dict(get_options, GetOptions)
        req.get_options.CopyFrom(get_options)
        resp = self._stub.GetPipeline(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.pipeline

    def update_pipeline(self, pipeline, update_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Update pipeline

        :param pipeline: pipeline object
        :type pipeline: Pipeline
        :param update_options: update options
        :type update_options: Optional[Union[UpdateOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: updated pipeline
        :rtype: Pipeline

        :example:

        >>> pipeline = APIClient.PipelineService.get_pipeline(namespace='my-project', name='my-object')
        >>> pipeline.spec.some_field = 'some_value'
        >>> APIClient.PipelineService.update_pipeline(pipeline)
        """
        req = UpdatePipelineRequest(pipeline=pipeline)
        update_options = self._process_message_or_dict(update_options, UpdateOptions)
        req.update_options.CopyFrom(update_options)
        resp = self._stub.UpdatePipeline(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.pipeline

    def delete_pipeline(self, namespace, name, delete_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Delete pipeline

        :param namespace: project name
        :type namespace: str
        :param name: pipeline object name
        :type name: str
        :param delete_options: delete options
        :type delete_options: Optional[Union[DeleteOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :example:

        >>> APIClient.PipelineService.delete_pipeline(namespace='my-project', name='my-object')
        """
        req = DeletePipelineRequest(namespace=namespace, name=name)
        delete_options = self._process_message_or_dict(delete_options, DeleteOptions)
        req.delete_options.CopyFrom(delete_options)
        self._stub.DeletePipeline(req, metadata=self._get_metadata(headers), timeout=timeout)

    def delete_pipeline_collection(self, namespace, delete_options=None, list_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Delete pipeline collection

        :param namespace: project name
        :type namespace: str
        :param delete_options: delete options
        :type delete_options: Optional[Union[DeleteOptions, Dict[str, Any]]]
        :param list_options: list options
        :type list_options: Optional[Union[ListOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :example:

        >>> APIClient.PipelineService.delete_pipeline_collection(namespace='my-project')
        """
        req = DeletePipelineCollectionRequest(namespace=namespace)
        delete_options = self._process_message_or_dict(delete_options, DeleteOptions)
        req.delete_options.CopyFrom(delete_options)
        list_options = self._process_message_or_dict(list_options, ListOptions)
        req.list_options.CopyFrom(list_options)
        self._stub.DeletePipelineCollection(req, metadata=self._get_metadata(headers), timeout=timeout)

    def list_pipeline(self, namespace, list_options=None, list_options_ext=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        List pipeline

        :param namespace: project name
        :type namespace: str
        :param list_options: list options
        :type list_options: Optional[Union[ListOptions, Dict[str, Any]]]
        :param list_options_ext: list options extension
        :type list_options_ext: Optional[Union[CreateOptionsExt, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: list of pipeline
        :rtype: PipelineList

        :example:

        >>> pipelines = APIClient.PipelineService.list_pipeline(namespace='my-project').items
        >>> pipelines = APIClient.PipelineService.list_pipeline(namespace='my-project', list_options={'limit': 5}).items
        """
        req = ListPipelineRequest(namespace=namespace)
        list_options = self._process_message_or_dict(list_options, ListOptions)
        req.list_options.CopyFrom(list_options)

        if list_options_ext is not None:
            # use _process_criterion_operation() to process operation
            # Criterion.match_value is a protobuf Any field, which is not handled
            # properly with _process_message_or_dict()
            operation = CriterionOperation()
            if isinstance(list_options_ext, dict) and 'operation' in list_options_ext:
                operation = self._process_criterion_operation(list_options_ext['operation'])
                del list_options_ext['operation']
            list_options_ext = self._process_message_or_dict(list_options_ext, ListOptionsExt)
            list_options_ext.operation.CopyFrom(operation)
            req.list_options_ext.CopyFrom(list_options_ext)

        resp = self._stub.ListPipeline(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.pipeline_list
