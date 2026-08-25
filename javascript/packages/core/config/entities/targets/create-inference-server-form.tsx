import { FormDialog } from '#core/components/form/components/form-dialog/form-dialog';
import { SelectField } from '#core/components/form/fields/select/select-field';
import { StringField } from '#core/components/form/fields/string/string-field';
import { combineValidators } from '#core/components/form/validation/combine-validators';
import { maxLength, regex, required } from '#core/components/form/validation/validators';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioMutation } from '#core/hooks/use-studio-mutation/use-studio-mutation';
import { InferenceServerOwnerFields } from './inference-server-owner-fields';
import { BACKEND_TYPE, CONTAINER_BUILD_TEMPLATE, TENANCY_TYPE } from './shared';

import type { CreateActionComponentProps } from '#core/components/actions/types';
import type { InferenceServer } from './types';

const K8S_NAME_PATTERN = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/;

const CONTAINER_BUILD_TEMPLATE_OPTIONS = [
  { id: CONTAINER_BUILD_TEMPLATE.DEFAULT_TRITON, label: 'Triton' },
  { id: CONTAINER_BUILD_TEMPLATE.DEFAULT_TRITON_GPU, label: 'Triton GPU' },
  { id: CONTAINER_BUILD_TEMPLATE.DEFAULT_TRITON_PYTHON, label: 'Triton Python' },
];

export const CreateInferenceServerForm = ({ onClose }: CreateActionComponentProps) => {
  const { projectId } = useStudioParams('base');

  const createInferenceServerMutation = useStudioMutation<InferenceServer, InferenceServer>({
    mutationName: 'CreateInferenceServer',
    successOperations: [
      { type: 'toast', message: 'Inference server created' },
      { type: 'invalidate', targets: ['ListInferenceServer'] },
    ],
  });

  const handleCreate = async (values: InferenceServer) => {
    if (createInferenceServerMutation.isPending) return;
    await createInferenceServerMutation.mutateAsync(values);
  };

  const initialValues: InferenceServer = {
    metadata: {
      name: '',
      namespace: projectId,
    },
    spec: {
      tenancyType: TENANCY_TYPE.DEDICATED,
      backendType: BACKEND_TYPE.TRITON,
      initSpec: {
        resourceSpec: {
          cpu: 1,
          memory: '',
          diskSize: '',
          gpu: 0,
        },
        servingSpec: {
          version: '',
          containerBuildTemplate: CONTAINER_BUILD_TEMPLATE.DEFAULT_TRITON,
        },
        numInstances: 1,
      },
    },
  };

  return (
    <FormDialog<InferenceServer>
      isOpen
      onDismiss={onClose}
      heading="Create target"
      onSubmit={handleCreate}
      submitLabel="Create"
      initialValues={initialValues}
    >
      <StringField
        name="metadata.name"
        label="Name"
        required
        maxLength={63}
        validate={combineValidators(
          required(),
          maxLength(63),
          regex(
            K8S_NAME_PATTERN,
            'Must only contain lowercase alphanumeric characters, "-", and must start and end with an alphanumeric character'
          )
        )}
        caption="Must only contain lowercase alphanumeric characters, '-', and must start and end with an alphanumeric character"
        placeholder="my-inference-server"
      />

      <StringField
        name="targetTypeDisplay"
        label="Target type"
        defaultValue="Inference Server"
        readOnly
      />

      <SelectField
        name="spec.initSpec.servingSpec.containerBuildTemplate"
        label="Service Type"
        required
        validate={required()}
        options={CONTAINER_BUILD_TEMPLATE_OPTIONS}
        clearable={false}
        caption='Set the type of image to use for the new inference server. Keep this value as "Triton" if you are unsure which one to use'
      />

      <InferenceServerOwnerFields />
    </FormDialog>
  );
};
