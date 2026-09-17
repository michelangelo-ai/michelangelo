import { FormDialog } from '#core/components/form/components/form-dialog/form-dialog';
import { SelectField } from '#core/components/form/fields/select/select-field';
import { StringField } from '#core/components/form/fields/string/string-field';
import { combineValidators } from '#core/components/form/validation/combine-validators';
import { maxLength, regex, required } from '#core/components/form/validation/validators';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioMutation } from '#core/hooks/use-studio-mutation/use-studio-mutation';
import { useStudioQuery } from '#core/hooks/use-studio-query';
import {
  K8S_NAME_MAX_LENGTH,
  K8S_NAME_PATTERN,
  K8S_NAME_RULES_MESSAGE,
} from '#core/utils/crd-utils';
import { ModelFamilyRevisionFields } from './model-family-revision-fields';
import { TARGET_TYPE } from './shared';

import type { DeploymentCreateInput, DeploymentRecord, InferenceServerListResult } from './types';

type DeploymentFormProps = { onClose: () => void } & (
  | { mode: 'create'; record?: never }
  | { mode: 'update'; record: DeploymentRecord }
);

/**
 * The single source of truth for the deployment form fields, shared by the create and
 * update flows so the two can never drift apart.
 *
 * Update mode prefills every field from the record, locks the identifying fields as
 * read-only, and submits the full record with the newly selected model as
 * desiredRevision so the controller rolls it out.
 */
export const DeploymentForm = ({ mode, record, onClose }: DeploymentFormProps) => {
  const { projectId } = useStudioParams('base');
  const isUpdate = mode === 'update';

  const currentModelName = record?.spec?.desiredRevision?.name;

  const { data: serverData, isLoading: isServersLoading } =
    useStudioQuery<InferenceServerListResult>({
      queryName: 'ListInferenceServer',
      serviceOptions: {},
    });

  const inferenceServerOptions = (serverData?.inferenceServerList.items ?? []).map((item) => ({
    id: item.metadata.name,
    label: item.metadata.name,
  }));

  const createDeploymentMutation = useStudioMutation<DeploymentCreateInput, DeploymentCreateInput>({
    mutationName: 'CreateDeployment',
    successOperations: [
      { type: 'toast', message: 'Deployment created' },
      { type: 'invalidate', targets: ['ListDeployment'] },
    ],
  });

  const updateDeploymentMutation = useStudioMutation<DeploymentRecord, DeploymentRecord>({
    mutationName: 'UpdateDeployment',
    successOperations: [
      { type: 'toast', message: 'Deployment update has begun' },
      // The controller reconciles the spec change into status asynchronously; the delay
      // gives it time to start the rollout before the refetch.
      { type: 'invalidate', targets: ['GetDeployment', 'ListDeployment'], delayMs: 2000 },
    ],
  });

  const handleDeploymentSubmit = async (values: DeploymentCreateInput) => {
    if (isUpdate) {
      if (updateDeploymentMutation.isPending) return;
      await updateDeploymentMutation.mutateAsync({
        ...record,
        spec: {
          ...record?.spec,
          desiredRevision: { name: values.spec.desiredRevision.name, namespace: projectId },
        },
      });
      return;
    }

    if (createDeploymentMutation.isPending) return;
    await createDeploymentMutation.mutateAsync({
      ...values,
      spec: {
        ...values.spec,
        modelFamily: { ...values.spec.modelFamily, namespace: projectId },
        desiredRevision: { ...values.spec.desiredRevision, namespace: projectId },
        target: {
          case: 'inferenceServer',
          value: { ...values.spec.target.value, namespace: projectId },
        },
      },
    });
  };

  const initialValues: DeploymentCreateInput = {
    metadata: {
      name: record?.metadata?.name ?? '',
      namespace: projectId,
    },
    spec: {
      modelFamily: { name: record?.spec?.modelFamily?.name ?? '', namespace: projectId },
      desiredRevision: { name: currentModelName ?? '', namespace: projectId },
      target: {
        case: 'inferenceServer',
        value: { name: record?.spec?.target?.value?.name ?? '', namespace: projectId },
      },
      strategy: { rolloutStrategy: { case: 'rolling', value: { incrementPercentage: 0 } } },
      definition: { type: record?.spec?.definition?.type ?? TARGET_TYPE.INFERENCE_SERVER },
    },
  };

  return (
    <FormDialog<DeploymentCreateInput>
      isOpen
      onDismiss={onClose}
      heading={isUpdate ? 'Update deployment' : 'Create deployment'}
      onSubmit={handleDeploymentSubmit}
      submitLabel={isUpdate ? 'Update' : 'Create'}
      initialValues={initialValues}
    >
      <StringField
        name="metadata.name"
        label="Name"
        required
        readOnly={isUpdate}
        maxLength={K8S_NAME_MAX_LENGTH}
        validate={combineValidators(
          required(),
          maxLength(K8S_NAME_MAX_LENGTH),
          regex(K8S_NAME_PATTERN, K8S_NAME_RULES_MESSAGE)
        )}
        caption={K8S_NAME_RULES_MESSAGE}
        placeholder="my-deployment"
      />

      <SelectField
        name="spec.target.value.name"
        label="Inference server"
        required
        readOnly={isUpdate}
        validate={required()}
        options={inferenceServerOptions}
        isLoading={isServersLoading}
        clearable={false}
        caption="The target inference server this deployment will serve traffic to"
      />

      <ModelFamilyRevisionFields modelFamilyReadOnly={isUpdate} />
    </FormDialog>
  );
};
