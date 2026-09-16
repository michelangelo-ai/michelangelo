import { useStyletron } from 'baseui';
import { Skeleton } from 'baseui/skeleton';
import { HeadingSmall } from 'baseui/typography';

import { Box } from '#core/components/box/box';
import { CircleExclamationMark } from '#core/components/illustrations/circle-exclamation-mark/circle-exclamation-mark';
import { CircleExclamationMarkKind } from '#core/components/illustrations/circle-exclamation-mark/types';
import { Signpost } from '#core/components/signpost/signpost';
import { TextEditor } from '#core/components/text-editor/text-editor';

import type { Pipeline } from './types';

/**
 * Roughly 15 lines at the editor's 14px font plus its content padding. Longer documents
 * scroll inside the editor instead of stretching the page.
 */
const EDITOR_HEIGHT = '320px';

/**
 * Information tab for a pipeline. Shows the registered manifest as a whole (type, file path,
 * UniFlow artifacts, triggers, and the unpacked content when mactl attached one) rather than
 * only `manifest.content`, because pipelines applied from YAML carry no content at all.
 */
export function PipelineInfoPage({ data, isLoading }: { data?: object; isLoading: boolean }) {
  const [, theme] = useStyletron();

  // cast: custom detail pages receive the entity as a plain object; narrowing to the
  // expected proto shape for property access; see #1425
  const pipeline = data as Pipeline | undefined;
  const manifest = pipeline?.spec?.manifest;

  if (isLoading) {
    return <Skeleton animation height={EDITOR_HEIGHT} width="100%" />;
  }

  if (!manifest || Object.keys(manifest).length === 0) {
    return (
      <Signpost
        illustration={
          <CircleExclamationMark
            height={theme.sizing.scale1600}
            width={theme.sizing.scale1600}
            kind={CircleExclamationMarkKind.PRIMARY}
          />
        }
        title="No manifest available"
        description="This pipeline was registered without a manifest"
      />
    );
  }

  return (
    <section>
      <HeadingSmall marginTop="0" marginBottom={theme.sizing.scale600}>
        Configuration
      </HeadingSmall>
      <Box title="Manifest">
        <TextEditor
          value={JSON.stringify(manifest, serializeBigInt, 2)}
          language="json"
          readOnly
          foldable
          height={EDITOR_HEIGHT}
        />
      </Box>
    </section>
  );
}

/**
 * protobuf-es decodes int64 fields (e.g. a trigger batch policy's `wait.seconds`) to bigint,
 * which JSON.stringify rejects. Render them as their decimal string instead.
 */
function serializeBigInt(_key: string, value: unknown): unknown {
  return typeof value === 'bigint' ? value.toString() : value;
}
