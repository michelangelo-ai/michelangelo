from michelangelo.gen.api.v2.revision_svc_pb2_grpc import RevisionServiceStub
from michelangelo.gen.api.v2.revision_svc_pb2 import (
    CreateRevisionRequest,
    GetRevisionRequest,
    UpdateRevisionRequest,
    DeleteRevisionRequest,
    DeleteRevisionCollectionRequest,
    ListRevisionRequest,
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


class RevisionService(BaseService):

    def __init__(self, context):
        super(RevisionService, self).__init__(context, RevisionServiceStub)

    def create_revision(self, revision, create_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Create revision

        :param revision: revision object
        :type revision: Revision
        :param create_options: create options
        :type create_options: Optional[Union[CreateOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: created revision
        :rtype: Revision

        :example:

        >>> from michelangelo.gen.api.v2.revision_pb2 import Revision
        >>> revision = Revision()
        >>> revision.namespace = 'my-project'
        >>> revision.name = 'my-revision'
        >>> APIClient.RevisionService.create_revision(revision)
        """
        req = CreateRevisionRequest(revision=revision)
        create_options = self._process_message_or_dict(create_options, CreateOptions)
        req.create_options.CopyFrom(create_options)
        resp = self._stub.CreateRevision(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.revision

    def get_revision(self, namespace, name, get_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Get revision

        :param namespace: project name
        :type namespace: str
        :param name: revision object name
        :type name: str
        :param get_options: get options
        :type get_options: Optional[Union[GetOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: revision
        :rtype: Revision

        :example:

        >>> revision = APIClient.RevisionService.get_revision(namespace='my-project', name='my-object')
        """
        req = GetRevisionRequest(name=name, namespace=namespace)
        get_options = self._process_message_or_dict(get_options, GetOptions)
        req.get_options.CopyFrom(get_options)
        resp = self._stub.GetRevision(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.revision

    def update_revision(self, revision, update_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Update revision

        :param revision: revision object
        :type revision: Revision
        :param update_options: update options
        :type update_options: Optional[Union[UpdateOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: updated revision
        :rtype: Revision

        :example:

        >>> revision = APIClient.RevisionService.get_revision(namespace='my-project', name='my-object')
        >>> revision.spec.some_field = 'some_value'
        >>> APIClient.RevisionService.update_revision(revision)
        """
        req = UpdateRevisionRequest(revision=revision)
        update_options = self._process_message_or_dict(update_options, UpdateOptions)
        req.update_options.CopyFrom(update_options)
        resp = self._stub.UpdateRevision(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.revision

    def delete_revision(self, namespace, name, delete_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Delete revision

        :param namespace: project name
        :type namespace: str
        :param name: revision object name
        :type name: str
        :param delete_options: delete options
        :type delete_options: Optional[Union[DeleteOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :example:

        >>> APIClient.RevisionService.delete_revision(namespace='my-project', name='my-object')
        """
        req = DeleteRevisionRequest(namespace=namespace, name=name)
        delete_options = self._process_message_or_dict(delete_options, DeleteOptions)
        req.delete_options.CopyFrom(delete_options)
        self._stub.DeleteRevision(req, metadata=self._get_metadata(headers), timeout=timeout)

    def delete_revision_collection(self, namespace, delete_options=None, list_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Delete revision collection

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

        >>> APIClient.RevisionService.delete_revision_collection(namespace='my-project')
        """
        req = DeleteRevisionCollectionRequest(namespace=namespace)
        delete_options = self._process_message_or_dict(delete_options, DeleteOptions)
        req.delete_options.CopyFrom(delete_options)
        list_options = self._process_message_or_dict(list_options, ListOptions)
        req.list_options.CopyFrom(list_options)
        self._stub.DeleteRevisionCollection(req, metadata=self._get_metadata(headers), timeout=timeout)

    def list_revision(self, namespace, list_options=None, list_options_ext=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        List revision

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

        :returns: list of revision
        :rtype: RevisionList

        :example:

        >>> revisions = APIClient.RevisionService.list_revision(namespace='my-project').items
        >>> revisions = APIClient.RevisionService.list_revision(namespace='my-project', list_options={'limit': 5}).items
        """
        req = ListRevisionRequest(namespace=namespace)
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

        resp = self._stub.ListRevision(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.revision_list


