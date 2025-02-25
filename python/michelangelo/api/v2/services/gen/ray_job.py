from michelangelo.gen.api.v2.ray_job_svc_pb2_grpc import RayJobServiceStub
from michelangelo.gen.api.v2.ray_job_svc_pb2 import (
    CreateRayJobRequest,
    GetRayJobRequest,
    UpdateRayJobRequest,
    DeleteRayJobRequest,
    DeleteRayJobCollectionRequest,
    ListRayJobRequest,
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


class RayJobService(BaseService):

    def __init__(self, context):
        super(RayJobService, self).__init__(context, RayJobServiceStub)

    def create_ray_job(self, ray_job, create_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Create ray job

        :param ray_job: ray job object
        :type ray_job: RayJob
        :param create_options: create options
        :type create_options: Optional[Union[CreateOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: created ray job
        :rtype: RayJob

        :example:

        >>> from michelangelo.gen.api.v2.ray_job_pb2 import RayJob
        >>> ray_job = RayJob()
        >>> ray_job.namespace = 'my-project'
        >>> ray_job.name = 'my-ray_job'
        >>> APIClient.RayJobService.create_ray_job(ray_job)
        """
        req = CreateRayJobRequest(ray_job=ray_job)
        create_options = self._process_message_or_dict(create_options, CreateOptions)
        req.create_options.CopyFrom(create_options)
        resp = self._stub.CreateRayJob(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.ray_job

    def get_ray_job(self, namespace, name, get_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Get ray job

        :param namespace: project name
        :type namespace: str
        :param name: ray job object name
        :type name: str
        :param get_options: get options
        :type get_options: Optional[Union[GetOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: ray job
        :rtype: RayJob

        :example:

        >>> ray_job = APIClient.RayJobService.get_ray_job(namespace='my-project', name='my-object')
        """
        req = GetRayJobRequest(name=name, namespace=namespace)
        get_options = self._process_message_or_dict(get_options, GetOptions)
        req.get_options.CopyFrom(get_options)
        resp = self._stub.GetRayJob(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.ray_job

    def update_ray_job(self, ray_job, update_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Update ray job

        :param ray_job: ray job object
        :type ray_job: RayJob
        :param update_options: update options
        :type update_options: Optional[Union[UpdateOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: updated ray job
        :rtype: RayJob

        :example:

        >>> ray_job = APIClient.RayJobService.get_ray_job(namespace='my-project', name='my-object')
        >>> ray_job.spec.some_field = 'some_value'
        >>> APIClient.RayJobService.update_ray_job(ray_job)
        """
        req = UpdateRayJobRequest(ray_job=ray_job)
        update_options = self._process_message_or_dict(update_options, UpdateOptions)
        req.update_options.CopyFrom(update_options)
        resp = self._stub.UpdateRayJob(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.ray_job

    def delete_ray_job(self, namespace, name, delete_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Delete ray job

        :param namespace: project name
        :type namespace: str
        :param name: ray job object name
        :type name: str
        :param delete_options: delete options
        :type delete_options: Optional[Union[DeleteOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :example:

        >>> APIClient.RayJobService.delete_ray_job(namespace='my-project', name='my-object')
        """
        req = DeleteRayJobRequest(namespace=namespace, name=name)
        delete_options = self._process_message_or_dict(delete_options, DeleteOptions)
        req.delete_options.CopyFrom(delete_options)
        self._stub.DeleteRayJob(req, metadata=self._get_metadata(headers), timeout=timeout)

    def delete_ray_job_collection(self, namespace, delete_options=None, list_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Delete ray job collection

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

        >>> APIClient.RayJobService.delete_ray_job_collection(namespace='my-project')
        """
        req = DeleteRayJobCollectionRequest(namespace=namespace)
        delete_options = self._process_message_or_dict(delete_options, DeleteOptions)
        req.delete_options.CopyFrom(delete_options)
        list_options = self._process_message_or_dict(list_options, ListOptions)
        req.list_options.CopyFrom(list_options)
        self._stub.DeleteRayJobCollection(req, metadata=self._get_metadata(headers), timeout=timeout)

    def list_ray_job(self, namespace, list_options=None, list_options_ext=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        List ray job

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

        :returns: list of ray job
        :rtype: RayJobList

        :example:

        >>> ray_jobs = APIClient.RayJobService.list_ray_job(namespace='my-project').items
        >>> ray_jobs = APIClient.RayJobService.list_ray_job(namespace='my-project', list_options={'limit': 5}).items
        """
        req = ListRayJobRequest(namespace=namespace)
        list_options = self._process_message_or_dict(list_options, ListOptions)
        req.list_options.CopyFrom(list_options)

        if list_options_ext is not None:
            operation = CriterionOperation()
            if isinstance(list_options_ext, dict) and 'operation' in list_options_ext:
                operation = self._process_criterion_operation(list_options_ext['operation'])
                del list_options_ext['operation']
            list_options_ext = self._process_message_or_dict(list_options_ext, ListOptionsExt)
            list_options_ext.operation.CopyFrom(operation)
            req.list_options_ext.CopyFrom(list_options_ext)

        resp = self._stub.ListRayJob(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.ray_job_list
