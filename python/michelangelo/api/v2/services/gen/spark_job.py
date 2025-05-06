from michelangelo.gen.api.v2.spark_job_svc_pb2_grpc import SparkJobServiceStub
from michelangelo.gen.api.v2.spark_job_svc_pb2 import (
    CreateSparkJobRequest,
    GetSparkJobRequest,
    UpdateSparkJobRequest,
    DeleteSparkJobRequest,
    DeleteSparkJobCollectionRequest,
    ListSparkJobRequest,
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


class SparkJobService(BaseService):

    def __init__(self, context):
        super(SparkJobService, self).__init__(context, SparkJobServiceStub)

    def create_spark_job(self, spark_job, create_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Create spark job

        :param spark_job: spark job object
        :type spark_job: SparkJob
        :param create_options: create options
        :type create_options: Optional[Union[CreateOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: created spark job
        :rtype: SparkJob

        :example:

        >>> from michelangelo.gen.api.v2.spark_job_pb2 import SparkJob
        >>> spark_job = SparkJob()
        >>> spark_job.namespace = 'my-project'
        >>> spark_job.name = 'my-spark_job'
        >>> APIClient.SparkJobService.create_spark_job(spark_job)
        """
        req = CreateSparkJobRequest(spark_job=spark_job)
        create_options = self._process_message_or_dict(create_options, CreateOptions)
        req.create_options.CopyFrom(create_options)
        resp = self._stub.CreateSparkJob(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.spark_job

    def get_spark_job(self, namespace, name, get_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Get spark job

        :param namespace: project name
        :type namespace: str
        :param name: spark job object name
        :type name: str
        :param get_options: get options
        :type get_options: Optional[Union[GetOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: spark job
        :rtype: SparkJob

        :example:

        >>> spark_job = APIClient.SparkJobService.get_spark_job(namespace='my-project', name='my-object')
        """
        req = GetSparkJobRequest(name=name, namespace=namespace)
        get_options = self._process_message_or_dict(get_options, GetOptions)
        req.get_options.CopyFrom(get_options)
        resp = self._stub.GetSparkJob(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.spark_job

    def update_spark_job(self, spark_job, update_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Update spark job

        :param spark_job: spark job object
        :type spark_job: SparkJob
        :param update_options: update options
        :type update_options: Optional[Union[UpdateOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: updated spark job
        :rtype: SparkJob

        :example:

        >>> spark_job = APIClient.SparkJobService.get_spark_job(namespace='my-project', name='my-object')
        >>> spark_job.spec.some_field = 'some_value'
        >>> APIClient.SparkJobService.update_spark_job(spark_job)
        """
        req = UpdateSparkJobRequest(spark_job=spark_job)
        update_options = self._process_message_or_dict(update_options, UpdateOptions)
        req.update_options.CopyFrom(update_options)
        resp = self._stub.UpdateSparkJob(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.spark_job

    def delete_spark_job(self, namespace, name, delete_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Delete spark job

        :param namespace: project name
        :type namespace: str
        :param name: spark job object name
        :type name: str
        :param delete_options: delete options
        :type delete_options: Optional[Union[DeleteOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :example:

        >>> APIClient.SparkJobService.delete_spark_job(namespace='my-project', name='my-object')
        """
        req = DeleteSparkJobRequest(namespace=namespace, name=name)
        delete_options = self._process_message_or_dict(delete_options, DeleteOptions)
        req.delete_options.CopyFrom(delete_options)
        self._stub.DeleteSparkJob(req, metadata=self._get_metadata(headers), timeout=timeout)

    def delete_spark_job_collection(self, namespace, delete_options=None, list_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Delete spark job collection

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

        >>> APIClient.SparkJobService.delete_spark_job_collection(namespace='my-project')
        """
        req = DeleteSparkJobCollectionRequest(namespace=namespace)
        delete_options = self._process_message_or_dict(delete_options, DeleteOptions)
        req.delete_options.CopyFrom(delete_options)
        list_options = self._process_message_or_dict(list_options, ListOptions)
        req.list_options.CopyFrom(list_options)
        self._stub.DeleteSparkJobCollection(req, metadata=self._get_metadata(headers), timeout=timeout)

    def list_spark_job(self, namespace, list_options=None, list_options_ext=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        List spark job

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

        :returns: list of spark job
        :rtype: SparkJobList

        :example:

        >>> spark_jobs = APIClient.SparkJobService.list_spark_job(namespace='my-project').items
        >>> spark_jobs = APIClient.SparkJobService.list_spark_job(namespace='my-project', list_options={'limit': 5}).items
        """
        req = ListSparkJobRequest(namespace=namespace)
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

        resp = self._stub.ListSparkJob(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.spark_job_list
