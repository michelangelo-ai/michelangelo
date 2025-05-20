from michelangelo.gen.api.v2.model_family_svc_pb2_grpc import ModelFamilyServiceStub
from michelangelo.gen.api.v2.model_family_svc_pb2 import (
    CreateModelFamilyRequest,
    GetModelFamilyRequest,
    UpdateModelFamilyRequest,
    DeleteModelFamilyRequest,
    DeleteModelFamilyCollectionRequest,
    ListModelFamilyRequest,
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


class ModelFamilyService(BaseService):

    def __init__(self, context):
        super(ModelFamilyService, self).__init__(context, ModelFamilyServiceStub)

    def create_model_family(self, model_family, create_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Create model family

        :param model_family: model family object
        :type model_family: ModelFamily
        :param create_options: create options
        :type create_options: Optional[Union[CreateOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: created model family
        :rtype: ModelFamily

        :example:

        >>> from michelangelo.gen.api.v2.model_family_pb2 import ModelFamily
        >>> model_family = ModelFamily()
        >>> model_family.namespace = 'my-project'
        >>> model_family.name = 'my-model_family'
        >>> APIClient.ModelFamilyService.create_model_family(model_family)
        """
        req = CreateModelFamilyRequest(model_family=model_family)
        create_options = self._process_message_or_dict(create_options, CreateOptions)
        req.create_options.CopyFrom(create_options)
        resp = self._stub.CreateModelFamily(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.model_family

    def get_model_family(self, namespace, name, get_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Get model family

        :param namespace: project name
        :type namespace: str
        :param name: model family object name
        :type name: str
        :param get_options: get options
        :type get_options: Optional[Union[GetOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: model family
        :rtype: ModelFamily

        :example:

        >>> model_family = APIClient.ModelFamilyService.get_model_family(namespace='my-project', name='my-object')
        """
        req = GetModelFamilyRequest(name=name, namespace=namespace)
        get_options = self._process_message_or_dict(get_options, GetOptions)
        req.get_options.CopyFrom(get_options)
        resp = self._stub.GetModelFamily(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.model_family

    def update_model_family(self, model_family, update_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Update model family

        :param model_family: model family object
        :type model_family: ModelFamily
        :param update_options: update options
        :type update_options: Optional[Union[UpdateOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: updated model family
        :rtype: ModelFamily

        :example:

        >>> model_family = APIClient.ModelFamilyService.get_model_family(namespace='my-project', name='my-object')
        >>> model_family.spec.some_field = 'some_value'
        >>> APIClient.ModelFamilyService.update_model_family(model_family)
        """
        req = UpdateModelFamilyRequest(model_family=model_family)
        update_options = self._process_message_or_dict(update_options, UpdateOptions)
        req.update_options.CopyFrom(update_options)
        resp = self._stub.UpdateModelFamily(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.model_family

    def delete_model_family(self, namespace, name, delete_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Delete model family

        :param namespace: project name
        :type namespace: str
        :param name: model family object name
        :type name: str
        :param delete_options: delete options
        :type delete_options: Optional[Union[DeleteOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :example:

        >>> APIClient.ModelFamilyService.delete_model_family(namespace='my-project', name='my-object')
        """
        req = DeleteModelFamilyRequest(namespace=namespace, name=name)
        delete_options = self._process_message_or_dict(delete_options, DeleteOptions)
        req.delete_options.CopyFrom(delete_options)
        self._stub.DeleteModelFamily(req, metadata=self._get_metadata(headers), timeout=timeout)

    def delete_model_family_collection(self, namespace, delete_options=None, list_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Delete model family collection

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

        >>> APIClient.ModelFamilyService.delete_model_family_collection(namespace='my-project')
        """
        req = DeleteModelFamilyCollectionRequest(namespace=namespace)
        delete_options = self._process_message_or_dict(delete_options, DeleteOptions)
        req.delete_options.CopyFrom(delete_options)
        list_options = self._process_message_or_dict(list_options, ListOptions)
        req.list_options.CopyFrom(list_options)
        self._stub.DeleteModelFamilyCollection(req, metadata=self._get_metadata(headers), timeout=timeout)

    def list_model_family(self, namespace, list_options=None, list_options_ext=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        List model family

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

        :returns: list of model family
        :rtype: ModelFamilyList

        :example:

        >>> model_familys = APIClient.ModelFamilyService.list_model_family(namespace='my-project').items
        >>> model_familys = APIClient.ModelFamilyService.list_model_family(namespace='my-project', list_options={'limit': 5}).items
        """
        req = ListModelFamilyRequest(namespace=namespace)
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

        resp = self._stub.ListModelFamily(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.model_family_list
