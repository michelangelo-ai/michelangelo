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

import type { CreateActionComponentProps } from '#core/components/actions/types';
import type { DeploymentCreateInput, InferenceServerListResult } from './types';

export const CreateDeploymentForm = ({ onClose }: CreateActionComponentProps) => {
  const { projectId } = useStudioParams('base');

  const { data, isLoading } = useStudioQuery<InferenceServerListResult>({
    queryName: 'ListInferenceServer',
    serviceOptions: {},
  });

  const inferenceServerOptions = (data?.inferenceServerList.items ?? []).map((item) => ({
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

  const handleCreate = async (values: DeploymentCreateInput) => {
    if (createDeploymentMutation.isPending) return;
    const { modelFamilyName: _modelFamilyName, ...specRest } = values.spec;
    await createDeploymentMutation.mutateAsync({
      ...values,
      spec: {
        ...specRest,
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
      name: '',
      namespace: projectId,
    },
    spec: {
      desiredRevision: { name: '', namespace: projectId },
      target: { case: 'inferenceServer', value: { name: '', namespace: projectId } },
      strategy: { rolloutStrategy: { case: 'rolling', value: { incrementPercentage: 0 } } },
      definition: { type: TARGET_TYPE.INFERENCE_SERVER },
    },
  };

  return (
    <FormDialog<DeploymentCreateInput>
      isOpen
      onDismiss={onClose}
      heading="Create deployment"
      onSubmit={handleCreate}
      submitLabel="Create"
      initialValues={initialValues}
    >
      <StringField
        name="metadata.name"
        label="Name"
        required
        maxLength={K8S_NAME_MAX_LENGTH}
        validate={combineValidators(
          required(),
          maxLength(K8S_NAME_MAX_LENGTH),
          regex(K8S_NAME_PATTERN, K8S_NAME_RULES_MESSAGE)
        )}
        caption={K8S_NAME_RULES_MESSAGE}
        placeholder="e.g. my-deployment"
      />

      <SelectField
        name="spec.target.value.name"
        label="Inference server"
        required
        validate={required()}
        options={inferenceServerOptions}
        isLoading={isLoading}
        clearable={false}
        caption="The target inference server this deployment will serve traffic to"
      />

      <ModelFamilyRevisionFields />
    </FormDialog>
  );
};
