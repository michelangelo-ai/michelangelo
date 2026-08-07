import { useStyletron } from 'baseui';
import { Skeleton } from 'baseui/skeleton';
import { HeadingSmall } from 'baseui/typography';

import { Box } from '#core/components/box/box';
import { StringField } from '#core/components/form/fields/string/string-field';
import { Form } from '#core/components/form/form';
import { LinksBox } from '#core/components/links-box/links-box';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { TARGET_TYPE_LABELS } from './shared';

import type { DeploymentRecord } from './types';

export function DeploymentInfoPage({ data, isLoading }: { data?: object; isLoading: boolean }) {
  const [css, theme] = useStyletron();
  const { projectId, phase } = useStudioParams('detail');

  // cast: custom detail pages receive the entity as a plain object; narrowing to the
  // expected proto shape for property access; see #1425
  const deployment = data as DeploymentRecord | undefined;

  const target = deployment?.spec?.target;
  const targetName = target?.case === 'inferenceServer' ? target.value?.name : undefined;
  const links = [{ name: targetName, url: `/${projectId}/${phase}/targets/${targetName}` }];
  const targetType = deployment?.spec?.definition?.type;

  const configurationFields = [
    {
      id: 'type-of-deployment',
      label: 'Type of deployment',
      value: targetType != null ? (TARGET_TYPE_LABELS[targetType] ?? '') : '',
    },
  ];

  const configurationValues = Object.fromEntries(
    configurationFields.map((field) => [field.id, field.value])
  );

  return (
    <div className={css({ display: 'flex', flexDirection: 'column', gap: theme.sizing.scale600 })}>
      <LinksBox title="Useful links" links={links} isLoading={isLoading} />

      <section>
        <HeadingSmall marginTop="0" marginBottom={theme.sizing.scale600}>
          Configuration
        </HeadingSmall>
        <Box>
          <Form onSubmit={() => undefined} initialValues={configurationValues}>
            <div
              className={css({
                display: 'grid',
                gridTemplateColumns: 'repeat(2, 1fr)',
                columnGap: theme.sizing.scale1200,
                rowGap: theme.sizing.scale600,
              })}
            >
              {configurationFields.map((field) =>
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
