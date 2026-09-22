import { useMemo } from 'react';
import { Input } from 'baseui/input';

import { FormControl } from '#core/components/form/components/form-control';
import { FormDialog } from '#core/components/form/components/form-dialog/form-dialog';
import { NumberField } from '#core/components/form/fields/number/number-field';
import { SelectField } from '#core/components/form/fields/select/select-field';
import { StringField } from '#core/components/form/fields/string/string-field';
import { FormGroup } from '#core/components/form/layout/form-group/form-group';
import { FormRow } from '#core/components/form/layout/form-row/form-row';
import { combineValidators } from '#core/components/form/validation/combine-validators';
import { maxLength, min, regex, required } from '#core/components/form/validation/validators';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioMutation } from '#core/hooks/use-studio-mutation/use-studio-mutation';
import {
  K8S_NAME_MAX_LENGTH,
  K8S_NAME_PATTERN,
  K8S_NAME_RULES_MESSAGE,
} from '#core/utils/crd-utils';
import {
  InferenceServerClusterFields,
  useRegisteredClusters,
} from './inference-server-cluster-fields';
import { InferenceServerOwnerFields } from './inference-server-owner-fields';
import {
  BACKEND_TYPE,
  BACKEND_TYPE_OPTIONS,
  CONTAINER_BUILD_TEMPLATE,
  TENANCY_TYPE,
} from './shared';

import type { CreateActionComponentProps } from '#core/components/actions/types';
import type { InferenceServer } from './types';

const CONTAINER_BUILD_TEMPLATE_OPTIONS = [
  { id: CONTAINER_BUILD_TEMPLATE.DEFAULT_TRITON, label: 'Triton' },
];

/** Kubernetes quantity, e.g. "4Gi", "512Mi", "100G". */
const K8S_QUANTITY_PATTERN = /^[0-9]+(\.[0-9]+)?(Ki|Mi|Gi|Ti|K|M|G|T)?$/;
const K8S_QUANTITY_MESSAGE = 'Use a Kubernetes quantity such as 4Gi or 512Mi';

export const CreateInferenceServerForm = ({ onClose }: CreateActionComponentProps) => {
  const { projectId } = useStudioParams('base');
  const { clusters, isLoading: isLoadingClusters } = useRegisteredClusters();

  const createInferenceServerMutation = useStudioMutation<InferenceServer, InferenceServer>({
    mutationName: 'CreateInferenceServer',
    successOperations: [
      { type: 'toast', message: 'Inference server created' },
      { type: 'invalidate', targets: ['ListInferenceServer'], delayMs: 2000 },
    ],
  });

  const handleCreate = async (values: InferenceServer) => {
    if (createInferenceServerMutation.isPending) return;
    await createInferenceServerMutation.mutateAsync(values);
  };

  // Kept referentially stable so React Final Form doesn't reinitialize (and discard typed
  // values) on re-renders such as the cluster list resolving.
  const initialValues = useMemo<InferenceServer>(
    () => ({
      metadata: {
        name: '',
        namespace: projectId,
      },
      spec: {
        tenancyType: TENANCY_TYPE.DEDICATED,
        backendType: BACKEND_TYPE.TRITON,
        initSpec: {
          resourceSpec: {
            cpu: 2,
            memory: '4Gi',
            diskSize: '',
            gpu: 0,
          },
          servingSpec: {
            version: '',
            containerBuildTemplate: CONTAINER_BUILD_TEMPLATE.DEFAULT_TRITON,
          },
          numInstances: 1,
        },
        clusterTargets: [],
      },
    }),
    [projectId]
  );

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
        maxLength={K8S_NAME_MAX_LENGTH}
        validate={combineValidators(
          required(),
          maxLength(K8S_NAME_MAX_LENGTH),
          regex(K8S_NAME_PATTERN, K8S_NAME_RULES_MESSAGE)
        )}
        caption={K8S_NAME_RULES_MESSAGE}
        placeholder="e.g. my-inference-server"
      />

      <FormControl label="Target type">
        <Input id="targetTypeDisplay" value="Inference Server" readOnly />
      </FormControl>

      <SelectField
        name="spec.backendType"
        label="Backend type"
        required
        validate={required()}
        options={BACKEND_TYPE_OPTIONS}
        clearable={false}
        caption="Serving framework. Triton is the only backend enabled in this environment."
      />

      <InferenceServerClusterFields clusters={clusters} isLoading={isLoadingClusters} />

      <FormGroup
        title="Initialization"
        description="Applied once when the server is provisioned on each cluster target."
      >
        <FormRow>
          <SelectField
            name="spec.initSpec.servingSpec.containerBuildTemplate"
            label="Service type"
            required
            validate={required()}
            options={CONTAINER_BUILD_TEMPLATE_OPTIONS}
            clearable={false}
            caption='Image type for the server. Keep "Triton" if unsure.'
          />
          <StringField
            name="spec.initSpec.servingSpec.version"
            label="Server version"
            placeholder="e.g. latest"
            caption="Server version or prediction service git sha. Leave empty to let the service choose."
          />
        </FormRow>
        <FormRow>
          <NumberField
            name="spec.initSpec.resourceSpec.cpu"
            label="CPU cores"
            required
            validate={combineValidators(required(), min(1))}
          />
          <StringField
            name="spec.initSpec.resourceSpec.memory"
            label="Memory"
            required
            validate={combineValidators(
              required(),
              regex(K8S_QUANTITY_PATTERN, K8S_QUANTITY_MESSAGE)
            )}
            placeholder="e.g. 4Gi"
          />
          <NumberField name="spec.initSpec.resourceSpec.gpu" label="GPUs" validate={min(0)} />
          <NumberField
            name="spec.initSpec.numInstances"
            label="Replicas"
            required
            validate={combineValidators(required(), min(1))}
          />
        </FormRow>
      </FormGroup>

      <InferenceServerOwnerFields />
    </FormDialog>
  );
};
