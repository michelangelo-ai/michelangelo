from michelangelo.gen.api.v2.pipeline_run_svc_pb2_grpc import PipelineRunServiceStub
from michelangelo.gen.api.v2.pipeline_run_svc_pb2 import (
    CreatePipelineRunRequest,
    GetPipelineRunRequest,
    UpdatePipelineRunRequest,
    DeletePipelineRunRequest,
    DeletePipelineRunCollectionRequest,
    ListPipelineRunRequest,
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


class PipelineRunService(BaseService):

    def __init__(self, context):
        super(PipelineRunService, self).__init__(context, PipelineRunServiceStub)

    def create_pipeline_run(self, pipeline_run, create_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Create pipeline run

        :param pipeline_run: pipeline run object
        :type pipeline_run: PipelineRun
        :param create_options: create options
        :type create_options: Optional[Union[CreateOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: created pipeline run
        :rtype: PipelineRun

        :example:

        >>> from michelangelo.gen.api.v2.pipeline_run_pb2 import PipelineRun
        >>> pipeline_run = PipelineRun()
        >>> pipeline_run.namespace = 'my-project'
        >>> pipeline_run.name = 'my-pipeline_run'
        >>> APIClient.PipelineRunService.create_pipeline_run(pipeline_run)
        """
        req = CreatePipelineRunRequest(pipeline_run=pipeline_run)
        create_options = self._process_message_or_dict(create_options, CreateOptions)
        req.create_options.CopyFrom(create_options)
        resp = self._stub.CreatePipelineRun(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.pipeline_run

    def get_pipeline_run(self, namespace, name, get_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Get pipeline run

        :param namespace: project name
        :type namespace: str
        :param name: pipeline run object name
        :type name: str
        :param get_options: get options
        :type get_options: Optional[Union[GetOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: pipeline run
        :rtype: PipelineRun

        :example:

        >>> pipeline_run = APIClient.PipelineRunService.get_pipeline_run(namespace='my-project', name='my-object')
        """
        req = GetPipelineRunRequest(name=name, namespace=namespace)
        get_options = self._process_message_or_dict(get_options, GetOptions)
        req.get_options.CopyFrom(get_options)
        resp = self._stub.GetPipelineRun(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.pipeline_run

    def update_pipeline_run(self, pipeline_run, update_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Update pipeline run

        :param pipeline_run: pipeline run object
        :type pipeline_run: PipelineRun
        :param update_options: update options
        :type update_options: Optional[Union[UpdateOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: updated pipeline run
        :rtype: PipelineRun

        :example:

        >>> pipeline_run = APIClient.PipelineRunService.get_pipeline_run(namespace='my-project', name='my-object')
        >>> pipeline_run.spec.some_field = 'some_value'
        >>> APIClient.PipelineRunService.update_pipeline_run(pipeline_run)
        """
        req = UpdatePipelineRunRequest(pipeline_run=pipeline_run)
        update_options = self._process_message_or_dict(update_options, UpdateOptions)
        req.update_options.CopyFrom(update_options)
        resp = self._stub.UpdatePipelineRun(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.pipeline_run

    def delete_pipeline_run(self, namespace, name, delete_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Delete pipeline run

        :param namespace: project name
        :type namespace: str
        :param name: pipeline run object name
        :type name: str
        :param delete_options: delete options
        :type delete_options: Optional[Union[DeleteOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :example:

        >>> APIClient.PipelineRunService.delete_pipeline_run(namespace='my-project', name='my-object')
        """
        req = DeletePipelineRunRequest(namespace=namespace, name=name)
        delete_options = self._process_message_or_dict(delete_options, DeleteOptions)
        req.delete_options.CopyFrom(delete_options)
        self._stub.DeletePipelineRun(req, metadata=self._get_metadata(headers), timeout=timeout)

    def delete_pipeline_run_collection(self, namespace, delete_options=None, list_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Delete pipeline run collection

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

        >>> APIClient.PipelineRunService.delete_pipeline_run_collection(namespace='my-project')
        """
        req = DeletePipelineRunCollectionRequest(namespace=namespace)
        delete_options = self._process_message_or_dict(delete_options, DeleteOptions)
        req.delete_options.CopyFrom(delete_options)
        list_options = self._process_message_or_dict(list_options, ListOptions)
        req.list_options.CopyFrom(list_options)
        self._stub.DeletePipelineRunCollection(req, metadata=self._get_metadata(headers), timeout=timeout)

    def list_pipeline_run(self, namespace, list_options=None, list_options_ext=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        List pipeline run

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

        :returns: list of pipeline run
        :rtype: PipelineRunList

        :example:

        >>> pipeline_runs = APIClient.PipelineRunService.list_pipeline_run(namespace='my-project').items
        >>> pipeline_runs = APIClient.PipelineRunService.list_pipeline_run(namespace='my-project', list_options={'limit': 5}).items
        """
        req = ListPipelineRunRequest(namespace=namespace)
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

        resp = self._stub.ListPipelineRun(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.pipeline_run_list
