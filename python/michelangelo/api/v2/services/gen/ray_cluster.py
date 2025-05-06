from michelangelo.gen.api.v2.ray_cluster_svc_pb2_grpc import RayClusterServiceStub
from michelangelo.gen.api.v2.ray_cluster_svc_pb2 import (
    CreateRayClusterRequest,
    GetRayClusterRequest,
    UpdateRayClusterRequest,
    DeleteRayClusterRequest,
    DeleteRayClusterCollectionRequest,
    ListRayClusterRequest,
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


class RayClusterService(BaseService):

    def __init__(self, context):
        super(RayClusterService, self).__init__(context, RayClusterServiceStub)

    def create_ray_cluster(self, ray_cluster, create_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Create ray cluster

        :param ray_cluster: ray cluster object
        :type ray_cluster: RayCluster
        :param create_options: create options
        :type create_options: Optional[Union[CreateOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: created ray cluster
        :rtype: RayCluster

        :example:

        >>> from michelangelo.gen.api.v2.ray_cluster_pb2 import RayCluster
        >>> ray_cluster = RayCluster()
        >>> ray_cluster.namespace = 'my-project'
        >>> ray_cluster.name = 'my-ray_cluster'
        >>> APIClient.RayClusterService.create_ray_cluster(ray_cluster)
        """
        req = CreateRayClusterRequest(ray_cluster=ray_cluster)
        create_options = self._process_message_or_dict(create_options, CreateOptions)
        req.create_options.CopyFrom(create_options)
        resp = self._stub.CreateRayCluster(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.ray_cluster

    def get_ray_cluster(self, namespace, name, get_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Get ray cluster

        :param namespace: project name
        :type namespace: str
        :param name: ray cluster object name
        :type name: str
        :param get_options: get options
        :type get_options: Optional[Union[GetOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: ray cluster
        :rtype: RayCluster

        :example:

        >>> ray_cluster = APIClient.RayClusterService.get_ray_cluster(namespace='my-project', name='my-object')
        """
        req = GetRayClusterRequest(name=name, namespace=namespace)
        get_options = self._process_message_or_dict(get_options, GetOptions)
        req.get_options.CopyFrom(get_options)
        resp = self._stub.GetRayCluster(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.ray_cluster

    def update_ray_cluster(self, ray_cluster, update_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Update ray cluster

        :param ray_cluster: ray cluster object
        :type ray_cluster: RayCluster
        :param update_options: update options
        :type update_options: Optional[Union[UpdateOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: updated ray cluster
        :rtype: RayCluster

        :example:

        >>> ray_cluster = APIClient.RayClusterService.get_ray_cluster(namespace='my-project', name='my-object')
        >>> ray_cluster.spec.some_field = 'some_value'
        >>> APIClient.RayClusterService.update_ray_cluster(ray_cluster)
        """
        req = UpdateRayClusterRequest(ray_cluster=ray_cluster)
        update_options = self._process_message_or_dict(update_options, UpdateOptions)
        req.update_options.CopyFrom(update_options)
        resp = self._stub.UpdateRayCluster(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.ray_cluster

    def delete_ray_cluster(self, namespace, name, delete_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Delete ray cluster

        :param namespace: project name
        :type namespace: str
        :param name: ray cluster object name
        :type name: str
        :param delete_options: delete options
        :type delete_options: Optional[Union[DeleteOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :example:

        >>> APIClient.RayClusterService.delete_ray_cluster(namespace='my-project', name='my-object')
        """
        req = DeleteRayClusterRequest(namespace=namespace, name=name)
        delete_options = self._process_message_or_dict(delete_options, DeleteOptions)
        req.delete_options.CopyFrom(delete_options)
        self._stub.DeleteRayCluster(req, metadata=self._get_metadata(headers), timeout=timeout)

    def delete_ray_cluster_collection(self, namespace, delete_options=None, list_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Delete ray cluster collection

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

        >>> APIClient.RayClusterService.delete_ray_cluster_collection(namespace='my-project')
        """
        req = DeleteRayClusterCollectionRequest(namespace=namespace)
        delete_options = self._process_message_or_dict(delete_options, DeleteOptions)
        req.delete_options.CopyFrom(delete_options)
        list_options = self._process_message_or_dict(list_options, ListOptions)
        req.list_options.CopyFrom(list_options)
        self._stub.DeleteRayClusterCollection(req, metadata=self._get_metadata(headers), timeout=timeout)

    def list_ray_cluster(self, namespace, list_options=None, list_options_ext=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        List ray cluster

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

        :returns: list of ray cluster
        :rtype: RayClusterList

        :example:

        >>> ray_clusters = APIClient.RayClusterService.list_ray_cluster(namespace='my-project').items
        >>> ray_clusters = APIClient.RayClusterService.list_ray_cluster(namespace='my-project', list_options={'limit': 5}).items
        """
        req = ListRayClusterRequest(namespace=namespace)
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

        resp = self._stub.ListRayCluster(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.ray_cluster_list
