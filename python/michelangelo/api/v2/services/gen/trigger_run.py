from michelangelo.gen.api.v2.trigger_run_svc_pb2_grpc import TriggerRunServiceStub
from michelangelo.gen.api.v2.trigger_run_svc_pb2 import (
    CreateTriggerRunRequest,
    GetTriggerRunRequest,
    UpdateTriggerRunRequest,
    DeleteTriggerRunRequest,
    DeleteTriggerRunCollectionRequest,
    ListTriggerRunRequest,
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


class TriggerRunService(BaseService):

    def __init__(self, context):
        super(TriggerRunService, self).__init__(context, TriggerRunServiceStub)

    def create_trigger_run(self, trigger_run, create_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Create trigger run

        :param trigger_run: trigger run object
        :type trigger_run: TriggerRun
        :param create_options: create options
        :type create_options: Optional[Union[CreateOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: created trigger run
        :rtype: TriggerRun

        :example:

        >>> from michelangelo.gen.api.v2.trigger_run_pb2 import TriggerRun
        >>> trigger_run = TriggerRun()
        >>> trigger_run.namespace = 'my-project'
        >>> trigger_run.name = 'my-trigger_run'
        >>> APIClient.TriggerRunService.create_trigger_run(trigger_run)
        """
        req = CreateTriggerRunRequest(trigger_run=trigger_run)
        create_options = self._process_message_or_dict(create_options, CreateOptions)
        req.create_options.CopyFrom(create_options)
        resp = self._stub.CreateTriggerRun(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.trigger_run

    def get_trigger_run(self, namespace, name, get_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Get trigger run

        :param namespace: project name
        :type namespace: str
        :param name: trigger run object name
        :type name: str
        :param get_options: get options
        :type get_options: Optional[Union[GetOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: trigger run
        :rtype: TriggerRun

        :example:

        >>> trigger_run = APIClient.TriggerRunService.get_trigger_run(namespace='my-project', name='my-object')
        """
        req = GetTriggerRunRequest(name=name, namespace=namespace)
        get_options = self._process_message_or_dict(get_options, GetOptions)
        req.get_options.CopyFrom(get_options)
        resp = self._stub.GetTriggerRun(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.trigger_run

    def update_trigger_run(self, trigger_run, update_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Update trigger run

        :param trigger_run: trigger run object
        :type trigger_run: TriggerRun
        :param update_options: update options
        :type update_options: Optional[Union[UpdateOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: updated trigger run
        :rtype: TriggerRun

        :example:

        >>> trigger_run = APIClient.TriggerRunService.get_trigger_run(namespace='my-project', name='my-object')
        >>> trigger_run.spec.some_field = 'some_value'
        >>> APIClient.TriggerRunService.update_trigger_run(trigger_run)
        """
        req = UpdateTriggerRunRequest(trigger_run=trigger_run)
        update_options = self._process_message_or_dict(update_options, UpdateOptions)
        req.update_options.CopyFrom(update_options)
        resp = self._stub.UpdateTriggerRun(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.trigger_run

    def delete_trigger_run(self, namespace, name, delete_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Delete trigger run

        :param namespace: project name
        :type namespace: str
        :param name: trigger run object name
        :type name: str
        :param delete_options: delete options
        :type delete_options: Optional[Union[DeleteOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :example:

        >>> APIClient.TriggerRunService.delete_trigger_run(namespace='my-project', name='my-object')
        """
        req = DeleteTriggerRunRequest(namespace=namespace, name=name)
        delete_options = self._process_message_or_dict(delete_options, DeleteOptions)
        req.delete_options.CopyFrom(delete_options)
        self._stub.DeleteTriggerRun(req, metadata=self._get_metadata(headers), timeout=timeout)

    def delete_trigger_run_collection(self, namespace, delete_options=None, list_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Delete trigger run collection

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

        >>> APIClient.TriggerRunService.delete_trigger_run_collection(namespace='my-project')
        """
        req = DeleteTriggerRunCollectionRequest(namespace=namespace)
        delete_options = self._process_message_or_dict(delete_options, DeleteOptions)
        req.delete_options.CopyFrom(delete_options)
        list_options = self._process_message_or_dict(list_options, ListOptions)
        req.list_options.CopyFrom(list_options)
        self._stub.DeleteTriggerRunCollection(req, metadata=self._get_metadata(headers), timeout=timeout)

    def list_trigger_run(self, namespace, list_options=None, list_options_ext=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        List trigger run

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

        :returns: list of trigger run
        :rtype: TriggerRunList

        :example:

        >>> trigger_runs = APIClient.TriggerRunService.list_trigger_run(namespace='my-project').items
        >>> trigger_runs = APIClient.TriggerRunService.list_trigger_run(namespace='my-project', list_options={'limit': 5}).items
        """
        req = ListTriggerRunRequest(namespace=namespace)
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

        resp = self._stub.ListTriggerRun(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.trigger_run_list
