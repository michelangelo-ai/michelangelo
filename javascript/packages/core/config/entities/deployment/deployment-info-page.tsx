import { useStyletron } from 'baseui';
import { Input } from 'baseui/input';
import { HeadingSmall } from 'baseui/typography';

import { Box } from '#core/components/box/box';
import { FormControl } from '#core/components/form/components/form-control';
import { LinksBox } from '#core/components/links-box/links-box';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { DeploymentRevisionCard } from './deployment-revision-card';
import { TARGET_TYPE_LABELS } from './shared';

import type { DeploymentRecord } from './types';

export function DeploymentInfoPage({ data }: { data?: object; isLoading: boolean }) {
  const [css, theme] = useStyletron();
  const { projectId, phase } = useStudioParams('detail');

  // cast: custom detail pages receive the entity as a plain object; narrowing to the
  // expected proto shape for property access; see #1425
  const deployment = data as DeploymentRecord | undefined;

  const target = deployment?.spec?.target;
  const targetName = target?.case === 'inferenceServer' ? target.value?.name : undefined;

  const links = [{ name: targetName, url: `/${projectId}/${phase}/targets/${targetName}` }];

  const targetType = deployment?.spec?.definition?.type;
  const trafficType = deployment?.metadata?.labels?.stage ?? '';
  const environment = deployment?.spec?.selector?.matchLabels?.environment;
  const partitions = deployment?.spec?.selector?.matchExpressions?.[0]?.values?.join(', ') ?? '';

  const configurationFields = [
    {
      label: 'Type of deployment',
      value: targetType != null ? (TARGET_TYPE_LABELS[targetType] ?? '') : '',
    },
    { label: 'Traffic type', value: trafficType },
    {
      label: 'Environment (Experimental, all environments by default)',
      value: environment ?? '* (all)',
      disabled: !environment,
    },
    { label: 'Partition (Experimental)', value: partitions, disabled: !partitions },
  ];

  return (
    <div className={css({ display: 'flex', flexDirection: 'column', gap: theme.sizing.scale600 })}>
      <LinksBox title="Useful links" links={links} />

      <section>
        <HeadingSmall marginTop="0" marginBottom={theme.sizing.scale600}>
          Key status indicators
        </HeadingSmall>
        <div className={css({ display: 'flex', gap: theme.sizing.scale600, flexWrap: 'wrap' })}>
          <DeploymentRevisionCard
            title="Current model in production"
            revision={deployment?.status?.currentRevision}
            emptyText="No currently deployed model"
          />
          <DeploymentRevisionCard
            title="Candidate model"
            revision={deployment?.status?.candidateRevision}
            emptyText="No model currently being deployed"
          />
          <DeploymentRevisionCard
            title="Desired model to be deployed"
            revision={deployment?.spec?.desiredRevision}
            emptyText="No model configured to be deployed"
          />
        </div>
      </section>

      <section>
        <HeadingSmall marginTop="0" marginBottom={theme.sizing.scale600}>
          Configuration
        </HeadingSmall>
        <Box>
          <div
            className={css({
              display: 'grid',
              gridTemplateColumns: 'repeat(2, 1fr)',
              columnGap: theme.sizing.scale1200,
              rowGap: theme.sizing.scale600,
            })}
          >
            {configurationFields.map((field) => (
              <FormControl key={field.label} label={field.label}>
                <Input value={field.value} readOnly disabled={field.disabled ?? false} />
              </FormControl>
            ))}
          </div>
        </Box>
      </section>
    </div>
  );
}
