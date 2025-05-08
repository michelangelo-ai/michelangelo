from michelangelo.gen.api.v2.cached_output_svc_pb2_grpc import CachedOutputServiceStub
from michelangelo.gen.api.v2.cached_output_svc_pb2 import (
    CreateCachedOutputRequest,
    GetCachedOutputRequest,
    UpdateCachedOutputRequest,
    DeleteCachedOutputRequest,
    DeleteCachedOutputCollectionRequest,
    ListCachedOutputRequest,
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


class CachedOutputService(BaseService):

    def __init__(self, context):
        super(CachedOutputService, self).__init__(context, CachedOutputServiceStub)

    def create_cached_output(self, cached_output, create_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Create cached output

        :param cached_output: cached output object
        :type cached_output: CachedOutput
        :param create_options: create options
        :type create_options: Optional[Union[CreateOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: created cached output
        :rtype: CachedOutput

        :example:

        >>> from michelangelo.gen.api.v2.cached_output_pb2 import CachedOutput
        >>> cached_output = CachedOutput()
        >>> cached_output.namespace = 'my-project'
        >>> cached_output.name = 'my-cached_output'
        >>> APIClient.CachedOutputService.create_cached_output(cached_output)
        """
        req = CreateCachedOutputRequest(cached_output=cached_output)
        create_options = self._process_message_or_dict(create_options, CreateOptions)
        req.create_options.CopyFrom(create_options)
        resp = self._stub.CreateCachedOutput(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.cached_output

    def get_cached_output(self, namespace, name, get_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Get cached output

        :param namespace: project name
        :type namespace: str
        :param name: cached output object name
        :type name: str
        :param get_options: get options
        :type get_options: Optional[Union[GetOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: cached output
        :rtype: CachedOutput

        :example:

        >>> cached_output = APIClient.CachedOutputService.get_cached_output(namespace='my-project', name='my-object')
        """
        req = GetCachedOutputRequest(name=name, namespace=namespace)
        get_options = self._process_message_or_dict(get_options, GetOptions)
        req.get_options.CopyFrom(get_options)
        resp = self._stub.GetCachedOutput(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.cached_output

    def update_cached_output(self, cached_output, update_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Update cached output

        :param cached_output: cached output object
        :type cached_output: CachedOutput
        :param update_options: update options
        :type update_options: Optional[Union[UpdateOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: updated cached output
        :rtype: CachedOutput

        :example:

        >>> cached_output = APIClient.CachedOutputService.get_cached_output(namespace='my-project', name='my-object')
        >>> cached_output.spec.some_field = 'some_value'
        >>> APIClient.CachedOutputService.update_cached_output(cached_output)
        """
        req = UpdateCachedOutputRequest(cached_output=cached_output)
        update_options = self._process_message_or_dict(update_options, UpdateOptions)
        req.update_options.CopyFrom(update_options)
        resp = self._stub.UpdateCachedOutput(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.cached_output

    def delete_cached_output(self, namespace, name, delete_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Delete cached output

        :param namespace: project name
        :type namespace: str
        :param name: cached output object name
        :type name: str
        :param delete_options: delete options
        :type delete_options: Optional[Union[DeleteOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :example:

        >>> APIClient.CachedOutputService.delete_cached_output(namespace='my-project', name='my-object')
        """
        req = DeleteCachedOutputRequest(namespace=namespace, name=name)
        delete_options = self._process_message_or_dict(delete_options, DeleteOptions)
        req.delete_options.CopyFrom(delete_options)
        self._stub.DeleteCachedOutput(req, metadata=self._get_metadata(headers), timeout=timeout)

    def delete_cached_output_collection(self, namespace, delete_options=None, list_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Delete cached output collection

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

        >>> APIClient.CachedOutputService.delete_cached_output_collection(namespace='my-project')
        """
        req = DeleteCachedOutputCollectionRequest(namespace=namespace)
        delete_options = self._process_message_or_dict(delete_options, DeleteOptions)
        req.delete_options.CopyFrom(delete_options)
        list_options = self._process_message_or_dict(list_options, ListOptions)
        req.list_options.CopyFrom(list_options)
        self._stub.DeleteCachedOutputCollection(req, metadata=self._get_metadata(headers), timeout=timeout)

    def list_cached_output(self, namespace, list_options=None, list_options_ext=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        List cached output

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

        :returns: list of cached output
        :rtype: CachedOutputList

        :example:

        >>> cached_outputs = APIClient.CachedOutputService.list_cached_output(namespace='my-project').items
        >>> cached_outputs = APIClient.CachedOutputService.list_cached_output(namespace='my-project', list_options={'limit': 5}).items
        """
        req = ListCachedOutputRequest(namespace=namespace)
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

        resp = self._stub.ListCachedOutput(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.cached_output_list
