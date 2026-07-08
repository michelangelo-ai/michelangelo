from michelangelo.gen.api.v2.evaluation_report_svc_pb2_grpc import (
    EvaluationReportServiceStub,
)
from michelangelo.gen.api.v2.evaluation_report_svc_pb2 import (
    CreateEvaluationReportRequest,
    DeleteEvaluationReportCollectionRequest,
    DeleteEvaluationReportRequest,
    GetEvaluationReportRequest,
    ListEvaluationReportRequest,
    UpdateEvaluationReportRequest,
)
from michelangelo.gen.api.list_pb2 import CriterionOperation, ListOptionsExt
from michelangelo.gen.k8s.io.apimachinery.pkg.apis.meta.v1.generated_pb2 import (
    CreateOptions,
    DeleteOptions,
    GetOptions,
    ListOptions,
    UpdateOptions,
)

from ..base import BaseService, _TIMEOUT_SECONDS


class EvaluationReportService(BaseService):
    """CRUD client for the EvaluationReportService gRPC API.

    Provides create, read, update, delete, and list operations over
    ``EvaluationReport`` resources stored in the Michelangelo backend.

    **Concept mapping** (for users familiar with experiment-tracking platforms):

    +-----------------------------+---------------------+-------------------+
    | Michelangelo                | MLflow              | W&B               |
    +=============================+=====================+===================+
    | ``metadata.namespace``      | Experiment name     | entity/project    |
    +-----------------------------+---------------------+-------------------+
    | ``metadata.name``           | Run name            | Run name          |
    +-----------------------------+---------------------+-------------------+
    | Chart with one data point   | ``log_metric()``    | ``wandb.log()``   |
    +-----------------------------+---------------------+-------------------+

    Requires ``MA_API_SERVER`` to be set in the environment before the first
    RPC call (the channel is lazily initialized). Access this service through
    ``APIClient.EvaluationReportService`` rather than instantiating directly.

    Example::

        import os
        os.environ["MA_API_SERVER"] = "localhost:50051"
        from michelangelo.api.v2 import APIClient
        from michelangelo.gen.api.v2.evaluation_report_pb2 import (
            EvaluationReport,
            EvaluationReportSpec,
        )

        report = EvaluationReport(spec=EvaluationReportSpec(title="Q1 Eval"))
        report.metadata.namespace = "my-project"  # analogous to MLflow experiment
        report.metadata.name = "q1-eval"
        APIClient.EvaluationReportService.create_evaluation_report(report)

    See also:
        :class:`~michelangelo.workflow.tasks.functions.eval_report_sinks.api.APIClientEvalReportSink`
    """

    def __init__(self, context):
        super(EvaluationReportService, self).__init__(
            context, EvaluationReportServiceStub
        )

    def create_evaluation_report(
        self,
        evaluation_report,
        create_options=None,
        headers=None,
        timeout=_TIMEOUT_SECONDS,
    ):
        """Create an evaluation report.

        Args:
            evaluation_report (EvaluationReport): Evaluation report to create.
                Set ``metadata.namespace`` (your project/experiment) and
                ``metadata.name`` (unique report identifier) before calling.
            create_options (CreateOptions | dict | None): Optional creation
                options proto or equivalent dict.
            headers (dict[str, str] | None): Optional gRPC request headers.
            timeout (int): RPC timeout in seconds. Defaults to 60.

        Returns:
            EvaluationReport: The created evaluation report as confirmed by
            the server.

        Example::

            from michelangelo.gen.api.v2.evaluation_report_pb2 import EvaluationReport
            report = EvaluationReport()
            report.metadata.namespace = 'my-project'
            report.metadata.name = 'q1-eval'
            APIClient.EvaluationReportService.create_evaluation_report(report)
        """
        req = CreateEvaluationReportRequest(evaluation_report=evaluation_report)
        create_options = self._process_message_or_dict(create_options, CreateOptions)
        req.create_options.CopyFrom(create_options)
        resp = self._stub.CreateEvaluationReport(
            req, metadata=self._get_metadata(headers), timeout=timeout
        )
        return resp.evaluation_report

    def get_evaluation_report(
        self, namespace, name, get_options=None, headers=None, timeout=_TIMEOUT_SECONDS
    ):
        """Get an evaluation report by namespace and name.

        Args:
            namespace (str): Namespace that owns the report (analogous to
                MLflow experiment name or W&B entity/project).
            name (str): Name of the evaluation report.
            get_options (GetOptions | dict | None): Optional retrieval options.
            headers (dict[str, str] | None): Optional gRPC request headers.
            timeout (int): RPC timeout in seconds. Defaults to 60.

        Returns:
            EvaluationReport: The retrieved evaluation report.

        Example::

            report = APIClient.EvaluationReportService.get_evaluation_report(
                namespace='my-project', name='q1-eval'
            )
        """
        req = GetEvaluationReportRequest(name=name, namespace=namespace)
        get_options = self._process_message_or_dict(get_options, GetOptions)
        req.get_options.CopyFrom(get_options)
        resp = self._stub.GetEvaluationReport(
            req, metadata=self._get_metadata(headers), timeout=timeout
        )
        return resp.evaluation_report

    def update_evaluation_report(
        self,
        evaluation_report,
        update_options=None,
        headers=None,
        timeout=_TIMEOUT_SECONDS,
    ):
        """Update an evaluation report.

        Args:
            evaluation_report (EvaluationReport): Evaluation report with
                updated fields.
            update_options (UpdateOptions | dict | None): Optional update options.
            headers (dict[str, str] | None): Optional gRPC request headers.
            timeout (int): RPC timeout in seconds. Defaults to 60.

        Returns:
            EvaluationReport: The updated evaluation report as confirmed by
            the server.

        Example::

            report = APIClient.EvaluationReportService.get_evaluation_report(
                namespace='my-project', name='q1-eval'
            )
            report.spec.title = 'Updated Title'
            APIClient.EvaluationReportService.update_evaluation_report(report)
        """
        req = UpdateEvaluationReportRequest(evaluation_report=evaluation_report)
        update_options = self._process_message_or_dict(update_options, UpdateOptions)
        req.update_options.CopyFrom(update_options)
        resp = self._stub.UpdateEvaluationReport(
            req, metadata=self._get_metadata(headers), timeout=timeout
        )
        return resp.evaluation_report

    def delete_evaluation_report(
        self,
        namespace,
        name,
        delete_options=None,
        headers=None,
        timeout=_TIMEOUT_SECONDS,
    ):
        """Delete an evaluation report.

        Args:
            namespace (str): Namespace that owns the report (analogous to
                MLflow experiment name or W&B entity/project).
            name (str): Name of the evaluation report to delete.
            delete_options (DeleteOptions | dict | None): Optional deletion options.
            headers (dict[str, str] | None): Optional gRPC request headers.
            timeout (int): RPC timeout in seconds. Defaults to 60.

        Example::

            APIClient.EvaluationReportService.delete_evaluation_report(
                namespace='my-project', name='q1-eval'
            )
        """
        req = DeleteEvaluationReportRequest(namespace=namespace, name=name)
        delete_options = self._process_message_or_dict(delete_options, DeleteOptions)
        req.delete_options.CopyFrom(delete_options)
        self._stub.DeleteEvaluationReport(
            req, metadata=self._get_metadata(headers), timeout=timeout
        )

    def delete_evaluation_report_collection(
        self,
        namespace,
        delete_options=None,
        list_options=None,
        headers=None,
        timeout=_TIMEOUT_SECONDS,
    ):
        """Delete a collection of evaluation reports in a namespace.

        .. warning::
            This is a bulk destructive operation. Omitting ``list_options``
            deletes **all** reports in the namespace. Pass a ``list_options``
            filter to scope the deletion.

        Args:
            namespace (str): Namespace to delete reports from (analogous to
                MLflow experiment name or W&B entity/project).
            delete_options (DeleteOptions | dict | None): Optional deletion options.
            list_options (ListOptions | dict | None): Optional list filter to
                select which reports to delete. Omit to delete all reports in
                the namespace.
            headers (dict[str, str] | None): Optional gRPC request headers.
            timeout (int): RPC timeout in seconds. Defaults to 60.

        Example::

            APIClient.EvaluationReportService.delete_evaluation_report_collection(
                namespace='my-project'
            )
        """
        req = DeleteEvaluationReportCollectionRequest(namespace=namespace)
        delete_options = self._process_message_or_dict(delete_options, DeleteOptions)
        req.delete_options.CopyFrom(delete_options)
        list_options = self._process_message_or_dict(list_options, ListOptions)
        req.list_options.CopyFrom(list_options)
        self._stub.DeleteEvaluationReportCollection(
            req, metadata=self._get_metadata(headers), timeout=timeout
        )

    def list_evaluation_report(
        self,
        namespace,
        list_options=None,
        list_options_ext=None,
        headers=None,
        timeout=_TIMEOUT_SECONDS,
    ):
        """List evaluation reports in a namespace.

        Args:
            namespace (str): Namespace to list reports from (analogous to
                MLflow experiment name or W&B entity/project).
            list_options (ListOptions | dict | None): Optional list options
                (pagination, field selectors).
            list_options_ext (ListOptionsExt | dict | None): Extended list
                options including ``CriterionOperation`` filters. Example::

                    list_options_ext={
                        "operation": {
                            "criterion": [{
                                "field_name": "metadata.name",
                                "operator": "EQUALS",
                                "match_value": "q1-eval",
                            }]
                        }
                    }

            headers (dict[str, str] | None): Optional gRPC request headers.
            timeout (int): RPC timeout in seconds. Defaults to 60.

        Returns:
            EvaluationReportList: Proto containing an ``items`` list of
            ``EvaluationReport`` messages.

        Example::

            reports = APIClient.EvaluationReportService.list_evaluation_report(
                namespace='my-project'
            ).items
        """
        req = ListEvaluationReportRequest(namespace=namespace)
        list_options = self._process_message_or_dict(list_options, ListOptions)
        req.list_options.CopyFrom(list_options)

        if list_options_ext is not None:
            operation = CriterionOperation()
            if isinstance(list_options_ext, dict) and "operation" in list_options_ext:
                list_options_ext = dict(list_options_ext)  # defensive copy
                operation = self._process_criterion_operation(
                    list_options_ext.pop("operation")
                )
            list_options_ext = self._process_message_or_dict(
                list_options_ext, ListOptionsExt
            )
            list_options_ext.operation.CopyFrom(operation)
            req.list_options_ext.CopyFrom(list_options_ext)

        resp = self._stub.ListEvaluationReport(
            req, metadata=self._get_metadata(headers), timeout=timeout
        )
        return resp.evaluation_report_list
