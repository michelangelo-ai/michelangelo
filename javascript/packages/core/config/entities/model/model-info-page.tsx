import { useStyletron } from 'baseui';
import { Skeleton } from 'baseui/skeleton';
import { HeadingSmall } from 'baseui/typography';

import { Box } from '#core/components/box/box';
import { StringField } from '#core/components/form/fields/string/string-field';
import { TextareaField } from '#core/components/form/fields/textarea/textarea-field';
import { Form } from '#core/components/form/form';
import { LinksBox } from '#core/components/links-box/links-box';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';

import type { ModelRecord } from './types';

export function ModelInfoPage({ data, isLoading }: { data?: object; isLoading: boolean }) {
  const [css, theme] = useStyletron();
  const { projectId, phase } = useStudioParams('detail');

  // cast: custom detail pages receive the entity as a plain object; narrowing to the
  // expected proto shape for property access; see #1425
  const model = data as ModelRecord | undefined;

  const pipelineRunName = model?.spec?.sourcePipelineRun?.name;
  const links = [
    {
      name: pipelineRunName,
      url: pipelineRunName ? `/${projectId}/${phase}/runs/${pipelineRunName}` : undefined,
    },
  ];

  const modelContextFields = [
    { id: 'model-family', label: 'Model family', value: model?.spec?.modelFamily?.name ?? '' },
    {
      id: 'training-framework',
      label: 'Training framework',
      value: model?.spec?.trainingFramework ?? '',
    },
    { id: 'source-platform', label: 'Source platform', value: model?.spec?.source ?? '' },
  ];

  const modelContextValues = Object.fromEntries(
    modelContextFields.map((field) => [field.id, field.value])
  );

  const trainingSetupFields = [
    {
      id: 'train-evaluation-hive-table',
      label: 'Train evaluation Hive table',
      value: model?.spec?.predictionResult?.trainTableName ?? '',
    },
    {
      id: 'validation-evaluation-hive-table',
      label: 'Validation evaluation Hive table',
      value: model?.spec?.predictionResult?.testTableName ?? '',
    },
  ];

  const trainingSetupValues = Object.fromEntries(
    trainingSetupFields.map((field) => [field.id, field.value])
  );

  return (
    <div className={css({ display: 'flex', flexDirection: 'column', gap: theme.sizing.scale600 })}>
      <LinksBox title="Useful links" links={links} isLoading={isLoading} />

      <Box title="Description">
        {isLoading ? (
          <Skeleton animation height="96px" width="100%" />
        ) : (
          <Form
            onSubmit={() => undefined}
            initialValues={{ description: model?.spec?.description ?? '' }}
          >
            <TextareaField name="description" readOnly rows={4} />
          </Form>
        )}
      </Box>

      <section>
        <HeadingSmall marginTop="0" marginBottom={theme.sizing.scale600}>
          Configuration
        </HeadingSmall>
        <Box title="Model context">
          <Form onSubmit={() => undefined} initialValues={modelContextValues}>
            <div
              className={css({
                display: 'grid',
                gridTemplateColumns: 'repeat(2, 1fr)',
                columnGap: theme.sizing.scale1200,
                rowGap: theme.sizing.scale600,
              })}
            >
              {modelContextFields.map((field) =>
                isLoading ? (
                  <Skeleton key={field.id} animation height="48px" width="100%" />
                ) : (
                  <StringField key={field.id} name={field.id} label={field.label} readOnly />
                )
              )}
            </div>
          </Form>
        </Box>

        <Box
          title="Training setup"
          overrides={{ BoxContainer: { style: { marginTop: theme.sizing.scale600 } } }}
        >
          <Form onSubmit={() => undefined} initialValues={trainingSetupValues}>
            <div
              className={css({
                display: 'grid',
                gridTemplateColumns: 'repeat(2, 1fr)',
                columnGap: theme.sizing.scale1200,
                rowGap: theme.sizing.scale600,
              })}
            >
              {trainingSetupFields.map((field) =>
                isLoading ? (
                  <Skeleton key={field.id} animation height="48px" width="100%" />
                ) : (
                  <StringField key={field.id} name={field.id} label={field.label} readOnly />
                )
              )}
            </div>
          </Form>
        </Box>
      </section>
    </div>
  );
}
