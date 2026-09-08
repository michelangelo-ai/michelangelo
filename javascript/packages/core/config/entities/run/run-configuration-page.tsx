import { useStyletron } from 'baseui';
import { Skeleton } from 'baseui/skeleton';
import { HeadingSmall } from 'baseui/typography';

import { Box } from '#core/components/box/box';
import { CircleExclamationMark } from '#core/components/illustrations/circle-exclamation-mark/circle-exclamation-mark';
import { CircleExclamationMarkKind } from '#core/components/illustrations/circle-exclamation-mark/types';
import { Signpost } from '#core/components/signpost/signpost';
import { TextEditor } from '#core/components/text-editor/text-editor';

import type { PipelineRun } from './types';

export function RunConfigurationPage({ data, isLoading }: { data?: object; isLoading: boolean }) {
  const [, theme] = useStyletron();

  // cast: custom detail pages receive the entity as a plain object; narrowing to the
  // expected proto shape for property access; see #1425
  const run = data as PipelineRun | undefined;
  const sourcePipeline = run?.status?.sourcePipeline;
  const manifest =
    sourcePipeline?.pipeline?.spec?.manifest ??
    sourcePipeline?.draftPipeline?.spec?.manifest ??
    run?.spec?.pipelineSpec?.manifest;
  const config = manifest?.content?.value;

  if (isLoading) {
    return <Skeleton animation height="400px" width="100%" />;
  }

  if (!config) {
    return (
      <Signpost
        illustration={
          <CircleExclamationMark
            height={theme.sizing.scale1600}
            width={theme.sizing.scale1600}
            kind={CircleExclamationMarkKind.PRIMARY}
          />
        }
        title="No configuration available"
        description="This pipeline run has no configuration attached to its manifest"
      />
    );
  }

  return (
    <section>
      <HeadingSmall marginTop="0" marginBottom={theme.sizing.scale600}>
        General
      </HeadingSmall>
      <Box title="Manifest content">
        <TextEditor
          value={JSON.stringify(config, null, 2)}
          language="json"
          readOnly
          foldable
          height="auto"
        />
      </Box>
    </section>
  );
}
