SERVICE_TEMPLATE = '''
from michelangelo.gen.api.{{group_name}}.{{crd_snake}}_svc_pb2_grpc import {{crd_camel}}ServiceStub
from michelangelo.gen.api.{{group_name}}.{{crd_snake}}_svc_pb2 import (
    Create{{crd_camel}}Request,
    Get{{crd_camel}}Request,
    Update{{crd_camel}}Request,
    Delete{{crd_camel}}Request,
    Delete{{crd_camel}}CollectionRequest,
    List{{crd_camel}}Request,
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


class {{crd_camel}}Service(BaseService):

    def __init__(self, context):
        super({{crd_camel}}Service, self).__init__(context, {{crd_camel}}ServiceStub)

    def create_{{crd_snake}}(self, {{crd_snake}}, create_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Create {{crd_name}}

        :param {{crd_snake}}: {{crd_name}} object
        :type {{crd_snake}}: {{crd_camel}}
        :param create_options: create options
        :type create_options: Optional[Union[CreateOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: created {{crd_name}}
        :rtype: {{crd_camel}}

        :example:

        >>> from michelangelo.gen.api.v2.{{crd_snake}}_pb2 import {{crd_camel}}
        >>> {{crd_snake}} = {{crd_camel}}()
        >>> {{crd_snake}}.namespace = 'my-project'
        >>> {{crd_snake}}.name = 'my-{{crd_snake}}'
        >>> APIClient.{{crd_camel}}Service.create_{{crd_snake}}({{crd_snake}})
        """
        req = Create{{crd_camel}}Request({{crd_snake}}={{crd_snake}})
        create_options = self._process_message_or_dict(create_options, CreateOptions)
        req.create_options.CopyFrom(create_options)
        resp = self._stub.Create{{crd_camel}}(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.{{crd_snake}}

    def get_{{crd_snake}}(self, namespace, name, get_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Get {{crd_name}}

        :param namespace: project name
        :type namespace: str
        :param name: {{crd_name}} object name
        :type name: str
        :param get_options: get options
        :type get_options: Optional[Union[GetOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: {{crd_name}}
        :rtype: {{crd_camel}}

        :example:

        >>> {{crd_snake}} = APIClient.{{crd_camel}}Service.get_{{crd_snake}}(namespace='my-project', name='my-object')
        """
        req = Get{{crd_camel}}Request(name=name, namespace=namespace)
        get_options = self._process_message_or_dict(get_options, GetOptions)
        req.get_options.CopyFrom(get_options)
        resp = self._stub.Get{{crd_camel}}(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.{{crd_snake}}

    def update_{{crd_snake}}(self, {{crd_snake}}, update_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Update {{crd_name}}

        :param {{crd_snake}}: {{crd_name}} object
        :type {{crd_snake}}: {{crd_camel}}
        :param update_options: update options
        :type update_options: Optional[Union[UpdateOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :returns: updated {{crd_name}}
        :rtype: {{crd_camel}}

        :example:

        >>> {{crd_snake}} = APIClient.{{crd_camel}}Service.get_{{crd_snake}}(namespace='my-project', name='my-object')
        >>> {{crd_snake}}.spec.some_field = 'some_value'
        >>> APIClient.{{crd_camel}}Service.update_{{crd_snake}}({{crd_snake}})
        """
        req = Update{{crd_camel}}Request({{crd_snake}}={{crd_snake}})
        update_options = self._process_message_or_dict(update_options, UpdateOptions)
        req.update_options.CopyFrom(update_options)
        resp = self._stub.Update{{crd_camel}}(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.{{crd_snake}}

    def delete_{{crd_snake}}(self, namespace, name, delete_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Delete {{crd_name}}

        :param namespace: project name
        :type namespace: str
        :param name: {{crd_name}} object name
        :type name: str
        :param delete_options: delete options
        :type delete_options: Optional[Union[DeleteOptions, Dict[str, Any]]]
        :param headers: request headers
        :type headers: Optional[Dict[str, str]]
        :param timeout: timeout in seconds, default is 60
        :type timeout: int

        :example:

        >>> APIClient.{{crd_camel}}Service.delete_{{crd_snake}}(namespace='my-project', name='my-object')
        """
        req = Delete{{crd_camel}}Request(namespace=namespace, name=name)
        delete_options = self._process_message_or_dict(delete_options, DeleteOptions)
        req.delete_options.CopyFrom(delete_options)
        self._stub.Delete{{crd_camel}}(req, metadata=self._get_metadata(headers), timeout=timeout)

    def delete_{{crd_snake}}_collection(self, namespace, delete_options=None, list_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        Delete {{crd_name}} collection

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

        >>> APIClient.{{crd_camel}}Service.delete_{{crd_snake}}_collection(namespace='my-project')
        """
        req = Delete{{crd_camel}}CollectionRequest(namespace=namespace)
        delete_options = self._process_message_or_dict(delete_options, DeleteOptions)
        req.delete_options.CopyFrom(delete_options)
        list_options = self._process_message_or_dict(list_options, ListOptions)
        req.list_options.CopyFrom(list_options)
        self._stub.Delete{{crd_camel}}Collection(req, metadata=self._get_metadata(headers), timeout=timeout)

    def list_{{crd_snake}}(self, namespace, list_options=None, list_options_ext=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """
        List {{crd_name}}

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

        :returns: list of {{crd_name}}
        :rtype: {{crd_camel}}List

        :example:

        >>> {{crd_snake}}s = APIClient.{{crd_camel}}Service.list_{{crd_snake}}(namespace='my-project').items
        >>> {{crd_snake}}s = APIClient.{{crd_camel}}Service.list_{{crd_snake}}(namespace='my-project', list_options={'limit': 5}).items
        """
        req = List{{crd_camel}}Request(namespace=namespace)
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

        resp = self._stub.List{{crd_camel}}(req, metadata=self._get_metadata(headers), timeout=timeout)
        return resp.{{crd_snake}}_list

'''.lstrip()


INIT_TEMPLATE = """
import importlib
import re


class ServicesGen(object):
{%- for crd in crds %}
    {{crd['crd_camel']}}Service = None
{%- endfor %}

    @classmethod
    def init(cls, context):
        services = filter(lambda x: not x.startswith('__') and x.endswith('Service'), cls.__dict__.keys())

        pattern = re.compile(r'(?<!^)(?=[A-Z])')
        for service in services:
            crd = pattern.sub('_', service).lower().rpartition('_service')[0]
            m = importlib.import_module('michelangelo.api.v2.services.gen.{}'.format(crd))
            setattr(cls, service, getattr(m, service)(context))

""".lstrip()
