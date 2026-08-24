import { useStyletron } from 'baseui';
import { Skeleton } from 'baseui/skeleton';
import { HeadingSmall, ParagraphMedium } from 'baseui/typography';

import { Box } from '#core/components/box/box';
import { LinksBox } from '#core/components/links-box/links-box';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';

import type { ModelRecord } from './types';

/**
 * Information tab for the Model detail page.
 *
 * Shows a link to the pipeline run that produced the model and the model's free-text
 * description. A table listing the deployments currently running this model is not included
 * here — it needs its own cross-entity query and loading/empty states, and is planned as a
 * follow-up to this tab.
 */
export function ModelInfoPage({ data, isLoading }: { data?: object; isLoading: boolean }) {
  const [css, theme] = useStyletron();
  const { projectId, phase } = useStudioParams('detail');

  // cast: custom detail pages receive the entity as a plain object; narrowing to the
  // expected proto shape for property access; see #1425
  const model = data as ModelRecord | undefined;

  const sourceRunName = model?.spec?.sourcePipelineRun?.name;
  const links = [{ name: sourceRunName, url: `/${projectId}/${phase}/runs/${sourceRunName}` }];

  return (
    <div className={css({ display: 'flex', flexDirection: 'column', gap: theme.sizing.scale600 })}>
      <LinksBox title="Useful links" links={links} isLoading={isLoading} />

      <section>
        <HeadingSmall marginTop="0" marginBottom={theme.sizing.scale600}>
          Description
        </HeadingSmall>
        <Box>
          {isLoading ? (
            <Skeleton animation height="24px" width="100%" />
          ) : (
            <ParagraphMedium margin="0">
              {model?.spec?.description ?? 'No description provided.'}
            </ParagraphMedium>
          )}
        </Box>
      </section>
    </div>
  );
}
