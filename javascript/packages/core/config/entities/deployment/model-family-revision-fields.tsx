import { useState } from 'react';
import { useForm } from 'react-final-form';
import { create, toBinary } from '@bufbuild/protobuf';
import { StringValueSchema } from '@bufbuild/protobuf/wkt';
import { Select } from 'baseui/select';

import { FormControl } from '#core/components/form/components/form-control';
import { useField } from '#core/components/form/hooks/use-field';
import { FormGroup } from '#core/components/form/layout/form-group/form-group';
import { useStudioQuery } from '#core/hooks/use-studio-query';

import type { OnChangeParams } from 'baseui/select';
import type { ModelFamilyListResult, ModelListResult } from './types';

// CriterionOperator.CRITERION_OPERATOR_EQUAL — see michelangelo/api/list.proto.
const CRITERION_OPERATOR_EQUAL = 1;

const modelFamilyListOptionsExt = (modelFamilyName: string) => ({
  operation: {
    criterion: [
      {
        fieldName: 'model.model_family_name',
        operator: CRITERION_OPERATOR_EQUAL,
        matchValue: {
          typeUrl: 'type.googleapis.com/google.protobuf.StringValue',
          value: Array.from(
            toBinary(StringValueSchema, create(StringValueSchema, { value: modelFamilyName }))
          ),
        },
      },
    ],
  },
});

/**
 * Resolves spec.desiredRevision.name from a Model Family -> Model cascade. The deployment
 * controller's AssetPreparationActor fetches this name directly as a Model CR (see
 * go/components/deployment/plugins/common/model.go's FetchModel), so despite the field's
 * name and proto annotation (resource_reference type "michelangelo.uber.com/Revision"),
 * it must be set to the selected Model's own name, not a Revision's. Must be rendered
 * inside FormDialog's <Form> so useForm() can write to that field.
 */
export const ModelFamilyRevisionFields = () => {
  const form = useForm();
  const [modelFamilyName, setModelFamilyName] = useState('');
  const [modelName, setModelName] = useState('');

  // Model itself isn't a form field (there's no `model` key in DeploymentCreateInput) — it drives
  // spec.desiredRevision.name below, so validation is attached there and surfaced on the Model control.
  const { input: revisionInput, meta: revisionMeta } = useField<string>(
    'spec.desiredRevision.name',
    {
      required: true,
      label: 'Model',
    }
  );

  const { data: modelFamilyData, isLoading: isModelFamilyLoading } =
    useStudioQuery<ModelFamilyListResult>({
      queryName: 'ListModelFamily',
      serviceOptions: {},
    });

  const { data: modelData, isLoading: isModelLoading } = useStudioQuery<ModelListResult>({
    queryName: 'ListModel',
    serviceOptions: { listOptionsExt: modelFamilyListOptionsExt(modelFamilyName) },
  });

  const modelFamilyOptions = (modelFamilyData?.modelFamilyList.items ?? []).map((item) => ({
    id: item.metadata.name,
    label: item.spec.name || item.metadata.name,
  }));

  const modelOptions = (modelData?.modelList.items ?? []).map((item) => ({
    id: item.metadata.name,
    label: item.metadata.name,
  }));

  const handleModelFamilyChange = (params: OnChangeParams) => {
    setModelFamilyName(String(params.value[0]?.id ?? ''));
    setModelName('');
    form.change('spec.desiredRevision.name', '');
  };

  const handleModelChange = (params: OnChangeParams) => {
    const nextModelName = String(params.value[0]?.id ?? '');
    setModelName(nextModelName);
    form.change('spec.desiredRevision.name', nextModelName);
  };

  return (
    <FormGroup title="Model" description="The selected model will be deployed">
      <FormControl label="Model family" caption="Model family the deployed model belongs to">
        <Select
          options={modelFamilyOptions}
          value={modelFamilyOptions.filter((option) => option.id === modelFamilyName)}
          onChange={handleModelFamilyChange}
          isLoading={isModelFamilyLoading}
          clearable={false}
        />
      </FormControl>

      <FormControl
        label="Model"
        required
        caption="Search and select the model to deploy"
        error={revisionMeta.touched && revisionMeta.error ? revisionMeta.error : undefined}
      >
        <Select
          options={modelOptions}
          value={modelOptions.filter((option) => option.id === modelName)}
          onChange={handleModelChange}
          onBlur={revisionInput.onBlur}
          isLoading={isModelLoading}
          disabled={!modelFamilyName}
          clearable={false}
          placeholder="Search model to deploy"
        />
      </FormControl>
    </FormGroup>
  );
};
