from michelangelo.gen.api.v2.model_svc_pb2_grpc import ModelServiceStub
from michelangelo.gen.api.v2.model_svc_pb2 import (
    CreateModelRequest,
    GetModelRequest,
    UpdateModelRequest,
    DeleteModelRequest,
    DeleteModelCollectionRequest,
    ListModelRequest,
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


class ModelService(BaseService):

    def __init__(self, context):
        super(ModelService, self).__init__(context, ModelServiceStub)

    def create_model(self, model, create_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Create model

        :param model: model object
        :type model: Model
        :param create_options: create options
        :type create_options: Optional[Union[CreateOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: created model
        :rtype: Model

        :example:

        >>> from michelangelo.gen.api.v2.model_pb2 import Model
        >>> model = Model()
        >>> model.namespace = 'my-project'
        >>> model.name = 'my-model'
        >>> APIClient.ModelService.create_model(model)
        """
        req = CreateModelRequest(model=model)
        create_options = self._process_message_or_dict(create_options, CreateOptions)
        req.create_options.CopyFrom(create_options)
        resp = self._stub.CreateModel(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.model

    def get_model(self, namespace, name, get_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Get model

        :param namespace: project name
        :type namespace: str
        :param name: model object name
        :type name: str
        :param get_options: get options
        :type get_options: Optional[Union[GetOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: model
        :rtype: Model

        :example:

        >>> model = APIClient.ModelService.get_model(namespace='my-project', name='my-object')
        """
        req = GetModelRequest(name=name, namespace=namespace)
        get_options = self._process_message_or_dict(get_options, GetOptions)
        req.get_options.CopyFrom(get_options)
        resp = self._stub.GetModel(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.model

    def update_model(self, model, update_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Update model

        :param model: model object
        :type model: Model
        :param update_options: update options
        :type update_options: Optional[Union[UpdateOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: updated model
        :rtype: Model

        :example:

        >>> model = APIClient.ModelService.get_model(namespace='my-project', name='my-object')
        >>> model.spec.some_field = 'some_value'
        >>> APIClient.ModelService.update_model(model)
        """
        req = UpdateModelRequest(model=model)
        update_options = self._process_message_or_dict(update_options, UpdateOptions)
        req.update_options.CopyFrom(update_options)
        resp = self._stub.UpdateModel(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.model

    def delete_model(self, namespace, name, delete_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Delete model

        :param namespace: project name
        :type namespace: str
        :param name: model object name
        :type name: str
        :param delete_options: delete options
        :type delete_options: Optional[Union[DeleteOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :example:

        >>> APIClient.ModelService.delete_model(namespace='my-project', name='my-object')
        """
        req = DeleteModelRequest(namespace=namespace, name=name)
        delete_options = self._process_message_or_dict(delete_options, DeleteOptions)
        req.delete_options.CopyFrom(delete_options)
        self._stub.DeleteModel(req, metadata=self._get_metadata(headers), timeout=timeout)

    def delete_model_collection(self, namespace, delete_options=None, list_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Delete model collection

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

        >>> APIClient.ModelService.delete_model_collection(namespace='my-project')
        """
        req = DeleteModelCollectionRequest(namespace=namespace)
        delete_options = self._process_message_or_dict(delete_options, DeleteOptions)
        req.delete_options.CopyFrom(delete_options)
        list_options = self._process_message_or_dict(list_options, ListOptions)
        req.list_options.CopyFrom(list_options)
        self._stub.DeleteModelCollection(req, metadata=self._get_metadata(headers), timeout=timeout)

    def list_model(self, namespace, list_options=None, list_options_ext=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        List model

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

        :returns: list of model
        :rtype: ModelList

        :example:

        >>> models = APIClient.ModelService.list_model(namespace='my-project').items
        >>> models = APIClient.ModelService.list_model(namespace='my-project', list_options={'limit': 5}).items
        """
        req = ListModelRequest(namespace=namespace)
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

        resp = self._stub.ListModel(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.model_list
