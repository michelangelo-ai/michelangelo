from michelangelo.gen.api.v2.deployment_svc_pb2_grpc import DeploymentServiceStub
from michelangelo.gen.api.v2.deployment_svc_pb2 import (
    CreateDeploymentRequest,
    GetDeploymentRequest,
    UpdateDeploymentRequest,
    DeleteDeploymentRequest,
    DeleteDeploymentCollectionRequest,
    ListDeploymentRequest,
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


class DeploymentService(BaseService):

    def __init__(self, context):
        super(DeploymentService, self).__init__(context, DeploymentServiceStub)

    def create_deployment(self, deployment, create_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Create deployment

        :param deployment: deployment object
        :type deployment: Deployment
        :param create_options: create options
        :type create_options: Optional[Union[CreateOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: created deployment
        :rtype: Deployment

        :example:

        >>> from michelangelo.gen.api.v2.deployment_pb2 import Deployment
        >>> deployment = Deployment()
        >>> deployment.namespace = 'my-project'
        >>> deployment.name = 'my-deployment'
        >>> APIClient.DeploymentService.create_deployment(deployment)
        """
        req = CreateDeploymentRequest(deployment=deployment)
        create_options = self._process_message_or_dict(create_options, CreateOptions)
        req.create_options.CopyFrom(create_options)
        resp = self._stub.CreateDeployment(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.deployment

    def get_deployment(self, namespace, name, get_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Get deployment

        :param namespace: project name
        :type namespace: str
        :param name: deployment object name
        :type name: str
        :param get_options: get options
        :type get_options: Optional[Union[GetOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: deployment
        :rtype: Deployment

        :example:

        >>> deployment = APIClient.DeploymentService.get_deployment(namespace='my-project', name='my-object')
        """
        req = GetDeploymentRequest(name=name, namespace=namespace)
        get_options = self._process_message_or_dict(get_options, GetOptions)
        req.get_options.CopyFrom(get_options)
        resp = self._stub.GetDeployment(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.deployment

    def update_deployment(self, deployment, update_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Update deployment

        :param deployment: deployment object
        :type deployment: Deployment
        :param update_options: update options
        :type update_options: Optional[Union[UpdateOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: updated deployment
        :rtype: Deployment

        :example:

        >>> deployment = APIClient.DeploymentService.get_deployment(namespace='my-project', name='my-object')
        >>> deployment.spec.some_field = 'some_value'
        >>> APIClient.DeploymentService.update_deployment(deployment)
        """
        req = UpdateDeploymentRequest(deployment=deployment)
        update_options = self._process_message_or_dict(update_options, UpdateOptions)
        req.update_options.CopyFrom(update_options)
        resp = self._stub.UpdateDeployment(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.deployment

    def delete_deployment(self, namespace, name, delete_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Delete deployment

        :param namespace: project name
        :type namespace: str
        :param name: deployment object name
        :type name: str
        :param delete_options: delete options
        :type delete_options: Optional[Union[DeleteOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :example:

        >>> APIClient.DeploymentService.delete_deployment(namespace='my-project', name='my-object')
        """
        req = DeleteDeploymentRequest(namespace=namespace, name=name)
        delete_options = self._process_message_or_dict(delete_options, DeleteOptions)
        req.delete_options.CopyFrom(delete_options)
        self._stub.DeleteDeployment(req, metadata=self._get_metadata(headers), timeout=timeout)

    def delete_deployment_collection(self, namespace, delete_options=None, list_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Delete deployment collection

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

        >>> APIClient.DeploymentService.delete_deployment_collection(namespace='my-project')
        """
        req = DeleteDeploymentCollectionRequest(namespace=namespace)
        delete_options = self._process_message_or_dict(delete_options, DeleteOptions)
        req.delete_options.CopyFrom(delete_options)
        list_options = self._process_message_or_dict(list_options, ListOptions)
        req.list_options.CopyFrom(list_options)
        self._stub.DeleteDeploymentCollection(req, metadata=self._get_metadata(headers), timeout=timeout)

    def list_deployment(self, namespace, list_options=None, list_options_ext=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        List deployment

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

        :returns: list of deployment
        :rtype: DeploymentList

        :example:

        >>> deployments = APIClient.DeploymentService.list_deployment(namespace='my-project').items
        >>> deployments = APIClient.DeploymentService.list_deployment(namespace='my-project', list_options={'limit': 5}).items
        """
        req = ListDeploymentRequest(namespace=namespace)
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

        resp = self._stub.ListDeployment(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.deployment_list

