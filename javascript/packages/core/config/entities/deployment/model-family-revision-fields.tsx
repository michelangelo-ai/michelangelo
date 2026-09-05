import { useEffect, useRef } from 'react';
import { useForm } from 'react-final-form';

import { SelectField } from '#core/components/form/fields/select/select-field';
import { useField } from '#core/components/form/hooks/use-field';
import { FormGroup } from '#core/components/form/layout/form-group/form-group';
import { useStudioQuery } from '#core/hooks/use-studio-query';

import type { ModelFamilyListResult, ModelListResult } from './types';

type ModelFamilyRevisionFieldsProps = {
  modelFamilyReadOnly?: boolean;
};

// CriterionOperator.CRITERION_OPERATOR_EQUAL — see michelangelo/api/list.proto.
const CRITERION_OPERATOR_EQUAL = 1;

const modelFamilyListOptionsExt = (modelFamilyName: string) => ({
  operation: {
    criterion: [
      {
        fieldName: 'model.model_family_name',
        operator: CRITERION_OPERATOR_EQUAL,
        matchValue: modelFamilyName,
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
export const ModelFamilyRevisionFields = ({
  modelFamilyReadOnly = false,
}: ModelFamilyRevisionFieldsProps) => {
  const form = useForm();

  const { input: modelFamilyInput } = useField<string>('spec.modelFamilyName');
  const modelFamilyName = modelFamilyInput.value ?? '';

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

  // Reset the downstream Model selection whenever the Model family changes. Skipped on
  // mount so a form opened with both values prefilled (update mode) keeps its Model.
  const previousFamilyName = useRef(modelFamilyName);
  useEffect(() => {
    if (previousFamilyName.current === modelFamilyName) return;
    previousFamilyName.current = modelFamilyName;
    form.change('spec.desiredRevision.name', '');
  }, [form, modelFamilyName]);

  return (
    <FormGroup title="Model" description="The selected model will be deployed">
      <SelectField
        name="spec.modelFamilyName"
        label="Model family"
        caption="Model family the deployed model belongs to"
        options={modelFamilyOptions}
        isLoading={isModelFamilyLoading}
        clearable={false}
        readOnly={modelFamilyReadOnly}
      />

      <SelectField
        name="spec.desiredRevision.name"
        label="Model"
        required
        caption="Search and select the model to deploy"
        options={modelOptions}
        isLoading={isModelLoading}
        disabled={!modelFamilyName}
        clearable={false}
      />
    </FormGroup>
  );
};
