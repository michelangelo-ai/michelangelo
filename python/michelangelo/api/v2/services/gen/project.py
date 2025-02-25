from michelangelo.gen.api.v2.project_svc_pb2_grpc import ProjectServiceStub
from michelangelo.gen.api.v2.project_svc_pb2 import (
    CreateProjectRequest,
    GetProjectRequest,
    UpdateProjectRequest,
    DeleteProjectRequest,
    DeleteProjectCollectionRequest,
    ListProjectRequest,
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


class ProjectService(BaseService):

    def __init__(self, context):
        super(ProjectService, self).__init__(context, ProjectServiceStub)

    def create_project(self, project, create_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Create project

        :param project: project object
        :type project: Project
        :param create_options: create options
        :type create_options: Optional[Union[CreateOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: created project
        :rtype: Project

        :example:

        >>> from michelangelo.gen.api.v2.project_pb2 import Project
        >>> project = Project()
        >>> project.namespace = 'my-project'
        >>> project.name = 'my-project'
        >>> APIClient.ProjectService.create_project(project)
        """
        req = CreateProjectRequest(project=project)
        create_options = self._process_message_or_dict(create_options, CreateOptions)
        req.create_options.CopyFrom(create_options)
        resp = self._stub.CreateProject(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.project

    def get_project(self, namespace, name, get_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Get project

        :param namespace: project name
        :type namespace: str
        :param name: project object name
        :type name: str
        :param get_options: get options
        :type get_options: Optional[Union[GetOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: project
        :rtype: Project

        :example:

        >>> project = APIClient.ProjectService.get_project(namespace='my-project', name='my-object')
        """
        req = GetProjectRequest(name=name, namespace=namespace)
        get_options = self._process_message_or_dict(get_options, GetOptions)
        req.get_options.CopyFrom(get_options)
        resp = self._stub.GetProject(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.project

    def update_project(self, project, update_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Update project

        :param project: project object
        :type project: Project
        :param update_options: update options
        :type update_options: Optional[Union[UpdateOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: updated project
        :rtype: Project

        :example:

        >>> project = APIClient.ProjectService.get_project(namespace='my-project', name='my-object')
        >>> project.spec.some_field = 'some_value'
        >>> APIClient.ProjectService.update_project(project)
        """
        req = UpdateProjectRequest(project=project)
        update_options = self._process_message_or_dict(update_options, UpdateOptions)
        req.update_options.CopyFrom(update_options)
        resp = self._stub.UpdateProject(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.project

    def delete_project(self, namespace, name, delete_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Delete project

        :param namespace: project name
        :type namespace: str
        :param name: project object name
        :type name: str
        :param delete_options: delete options
        :type delete_options: Optional[Union[DeleteOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :example:

        >>> APIClient.ProjectService.delete_project(namespace='my-project', name='my-object')
        """
        req = DeleteProjectRequest(namespace=namespace, name=name)
        delete_options = self._process_message_or_dict(delete_options, DeleteOptions)
        req.delete_options.CopyFrom(delete_options)
        self._stub.DeleteProject(req, metadata=self._get_metadata(headers), timeout=timeout)

    def delete_project_collection(self, namespace, delete_options=None, list_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Delete project collection

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

        >>> APIClient.ProjectService.delete_project_collection(namespace='my-project')
        """
        req = DeleteProjectCollectionRequest(namespace=namespace)
        delete_options = self._process_message_or_dict(delete_options, DeleteOptions)
        req.delete_options.CopyFrom(delete_options)
        list_options = self._process_message_or_dict(list_options, ListOptions)
        req.list_options.CopyFrom(list_options)
        self._stub.DeleteProjectCollection(req, metadata=self._get_metadata(headers), timeout=timeout)

    def list_project(self, namespace, list_options=None, list_options_ext=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        List project

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

        :returns: list of project
        :rtype: ProjectList

        :example:

        >>> projects = APIClient.ProjectService.list_project(namespace='my-project').items
        >>> projects = APIClient.ProjectService.list_project(namespace='my-project', list_options={'limit': 5}).items
        """
        req = ListProjectRequest(namespace=namespace)
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

        resp = self._stub.ListProject(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.project_list
