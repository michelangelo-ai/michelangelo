import { useEffect, useState } from 'react';
import { useForm } from 'react-final-form';
import { create, toBinary } from '@bufbuild/protobuf';
import { StringValueSchema } from '@bufbuild/protobuf/wkt';
import { Select } from 'baseui/select';

import { FormControl } from '#core/components/form/components/form-control';
import { useField } from '#core/components/form/hooks/use-field';
import { FormGroup } from '#core/components/form/layout/form-group/form-group';
import { useStudioQuery } from '#core/hooks/use-studio-query';

import type { OnChangeParams } from 'baseui/select';
import type { ModelFamilyListResult, ModelListResult, RevisionListResult } from './types';

// CriterionOperator.CRITERION_OPERATOR_EQUAL — see michelangelo/api/list.proto.
const CRITERION_OPERATOR_EQUAL = 1;

// Filters the ListModel RPC to the given model family server-side, via
// ListOptionsExt.operation, instead of fetching every Model and filtering in JS.
// go/storage/mysql/mysql.go's indexPathToKeyMaps is nil in this deployment (OSS
// default, "permissive" mode), so field names pass straight through to SQL after
// stripping the "model." CRD prefix — the fieldName must equal the actual MySQL
// column name. Model.spec.model_family is a composite ResourceIdentifier index
// (proto/api/v2/model.proto: `key: "model_family"`), which go/kubeproto/util/index.go
// flattens into "model_family_name"/"model_family_namespace" columns, so the
// namespaced subfield here is "model.model_family_name".
//
// matchValue is built as a plain {typeUrl, value} object rather than a real Any
// message (e.g. via anyPack): serviceOptions passes through useStudioQuery's
// interpolation resolver, which walks every plain object/array in the payload
// (resolve-interpolations.ts's isRecord treats any non-array object as walkable) —
// including a Uint8Array's own enumerable index properties — and rebuilds it as a
// plain `{0: byte, 1: byte, ...}` object, losing its ArrayBuffer backing. A plain
// number array survives that walk untouched, and @bufbuild/protobuf's create()
// converts it back into a real Uint8Array when building the request message.
// value must hold the serialized StringValue bytes (not the bare model family
// string) since Any.value is itself a length-delimited proto-encoded field.
// Encoding that Any to JSON also requires StringValueSchema to be registered in
// packages/rpc/services.ts's typeRegistry — without it, toJson throws
// "cannot encode message google.protobuf.Any to JSON" before any request fires.
const modelFamilyListOptionsExt = (modelFamilyName: string) => ({
  operation: {
    criterion: [
      {
        fieldName: 'model.model_family_name',
        operator: CRITERION_OPERATOR_EQUAL,
        matchValue: {
          typeUrl: 'type.googleapis.com/google.protobuf.StringValue',
          value: Array.from(toBinary(StringValueSchema, create(StringValueSchema, { value: modelFamilyName }))),
        },
      },
    ],
  },
});

/**
 * Resolves the Revision field from a Model Family -> Model cascade instead of asking for a
 * Revision name directly: selecting a Model auto-fills spec.desiredRevision.name with that
 * model's most recently created Revision. Must be rendered inside FormDialog's <Form> so
 * useForm() can write to the Revision field.
 */
export const ModelFamilyRevisionFields = () => {
  const form = useForm();
  const [modelFamilyName, setModelFamilyName] = useState('');
  const [modelName, setModelName] = useState('');

  // Model itself isn't a form field (there's no `model` key in DeploymentCreateInput) — it drives
  // spec.desiredRevision.name below, so validation is attached there and surfaced on the Model control.
  const { input: revisionInput, meta: revisionMeta } = useField<string>('spec.desiredRevision.name', {
    required: true,
    label: 'Model',
  });

  const { data: modelFamilyData, isLoading: isModelFamilyLoading } =
    useStudioQuery<ModelFamilyListResult>({
      queryName: 'ListModelFamily',
      serviceOptions: {},
    });

  const { data: modelData, isLoading: isModelLoading } = useStudioQuery<ModelListResult>({
    queryName: 'ListModel',
    serviceOptions: { listOptionsExt: modelFamilyListOptionsExt(modelFamilyName) },
  });

  const { data: revisionData } = useStudioQuery<RevisionListResult>({
    queryName: 'ListRevision',
    serviceOptions: {},
  });

  const modelFamilyOptions = (modelFamilyData?.modelFamilyList.items ?? []).map((item) => ({
    id: item.metadata.name,
    label: item.spec.name || item.metadata.name,
  }));

  const modelOptions = (modelData?.modelList.items ?? []).map((item) => ({
    id: item.metadata.name,
    label: item.metadata.name,
  }));

  useEffect(() => {
    if (!modelName) {
      form.change('spec.desiredRevision.name', '');
      return;
    }

    const latestRevision = (revisionData?.revisionList.items ?? [])
      .filter((item) => item.spec.baseResource?.name === modelName)
      .reduce<RevisionListResult['revisionList']['items'][number] | undefined>(
        (latest, candidate) => {
          const latestTime = latest?.metadata.creationTimestamp?.toDate().getTime() ?? 0;
          const candidateTime = candidate.metadata.creationTimestamp?.toDate().getTime() ?? 0;
          return candidateTime >= latestTime ? candidate : latest;
        },
        undefined
      );

    form.change('spec.desiredRevision.name', latestRevision?.metadata.name ?? '');
  }, [modelName, revisionData, form]);

  const handleModelFamilyChange = (params: OnChangeParams) => {
    setModelFamilyName(String(params.value[0]?.id ?? ''));
    setModelName('');
  };

  const handleModelChange = (params: OnChangeParams) => {
    setModelName(String(params.value[0]?.id ?? ''));
  };

  return (
    <FormGroup title="Model" description="The latest revision of the selected model will be deployed">
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
