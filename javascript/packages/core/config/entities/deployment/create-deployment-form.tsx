import { FormDialog } from '#core/components/form/components/form-dialog/form-dialog';
import { RadioField } from '#core/components/form/fields/radio/radio-field';
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
import { TARGET_TYPE, TARGET_TYPE_LABELS } from './shared';

import type { CreateActionComponentProps } from '#core/components/actions/types';
import type { DeploymentCreateInput, InferenceServerListResult } from './types';

const DEPLOYMENT_TYPE_OPTIONS = [
  { value: 'online', label: TARGET_TYPE_LABELS[TARGET_TYPE.INFERENCE_SERVER] },
  { value: 'offline', label: TARGET_TYPE_LABELS[TARGET_TYPE.OFFLINE] },
];

const parseDeploymentType = (value?: string) =>
  value === 'offline' ? TARGET_TYPE.OFFLINE : TARGET_TYPE.INFERENCE_SERVER;

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
    const { modelFamilyName: _modelFamilyName, deploymentType, ...specRest } = values.spec;
    await createDeploymentMutation.mutateAsync({
      ...values,
      spec: {
        ...specRest,
        desiredRevision: { ...values.spec.desiredRevision, namespace: projectId },
        target: {
          case: 'inferenceServer',
          value: { ...values.spec.target.value, namespace: projectId },
        },
        definition: { type: parseDeploymentType(deploymentType) },
      },
    });
  };

  const initialValues: DeploymentCreateInput = {
    metadata: {
      name: '',
      namespace: projectId,
    },
    spec: {
      deploymentType: 'online',
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
        placeholder="my-deployment"
      />

      <RadioField
        name="spec.deploymentType"
        label="Type of deployment"
        required
        disabled
        options={DEPLOYMENT_TYPE_OPTIONS}
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
