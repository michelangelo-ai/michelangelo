import { useStyletron } from 'baseui';
import { ARTWORK_TYPE } from 'baseui/banner';
import { Spinner } from 'baseui/spinner';
import { ParagraphMedium } from 'baseui/typography';

import { Banner } from '#core/components/banner/banner';
import { Box } from '#core/components/box/box';
import { DateTime } from '#core/components/date-time/date-time';
import { Icon } from '#core/components/icon/icon';
import { TAG_BEHAVIOR, TAG_COLOR, TAG_HIERARCHY, TAG_SIZE } from '#core/components/tag/constants';
import { Tag } from '#core/components/tag/tag';
import { useStudioQuery } from '#core/hooks/use-studio-query';
import { MODEL_KIND_TEXT_MAP } from '../model/constants';
import { DeploymentRevisionCardField } from './deployment-revision-card-field';

import type { ModelRecord } from '../model/types';
import type { ResourceRef } from './types';

const CARD_OVERRIDES = { BoxContainer: { style: { flexBasis: '320px', flexGrow: 1 } } };

/**
 * Renders a card for one of a deployment's revision references (current,
 * candidate, or desired). Resolves the referenced Model CR to show its
 * metadata (owner, type, creation time, source pipeline run); falls back to
 * the bare revision name when the Model cannot be fetched.
 */
export function DeploymentRevisionCard({
  title,
  revision,
  emptyText,
  isLoading,
}: {
  title: string;
  revision?: ResourceRef;
  emptyText: string;
  isLoading?: boolean;
}) {
  const [css, theme] = useStyletron();

  const { data, isLoading: isModelLoading } = useStudioQuery<{ model?: ModelRecord }>({
    queryName: 'GetModel',
    serviceOptions: {
      name: revision?.name,
      ...(revision?.namespace != null && { namespace: revision.namespace }),
    },
    clientOptions: { enabled: Boolean(revision?.name) },
  });

  if (isLoading || (revision?.name && isModelLoading)) {
    return (
      <Box title={title} overrides={CARD_OVERRIDES}>
        <div className={css({ display: 'flex', justifyContent: 'center' })}>
          <Spinner />
        </div>
      </Box>
    );
  }

  if (!revision?.name) {
    return (
      <Box title={title} overrides={CARD_OVERRIDES}>
        <Banner
          kind="info"
          artwork={{
            type: ARTWORK_TYPE.icon,
            icon: () => <Icon name="circleI" size={theme.sizing.scale800} />,
          }}
        >
          {emptyText}
        </Banner>
      </Box>
    );
  }

  const model = data?.model;
  const owner = model?.spec?.owner?.name;
  const kind = model?.spec?.kind;
  const kindLabel = kind != null ? MODEL_KIND_TEXT_MAP[kind] : undefined;
  const creationSeconds = model?.metadata?.creationTimestamp?.seconds;
  const sourcePipelineRun = model?.spec?.sourcePipelineRun?.name;

  return (
    <Box title={title} overrides={CARD_OVERRIDES}>
      <div
        className={css({
          display: 'grid',
          gridTemplateColumns: 'repeat(2, 1fr)',
          columnGap: theme.sizing.scale1200,
          rowGap: theme.sizing.scale900,
        })}
      >
        <DeploymentRevisionCardField label="Model">
          <ParagraphMedium marginTop="0" marginBottom="0">
            {revision.name}
          </ParagraphMedium>
        </DeploymentRevisionCardField>

        <DeploymentRevisionCardField label="Owner">
          <ParagraphMedium marginTop="0" marginBottom="0">
            {owner != null && owner !== '' ? owner : '—'}
          </ParagraphMedium>
        </DeploymentRevisionCardField>

        <DeploymentRevisionCardField label="Creation time">
          <ParagraphMedium marginTop="0" marginBottom="0">
            {creationSeconds != null ? <DateTime timestamp={creationSeconds} /> : '—'}
          </ParagraphMedium>
        </DeploymentRevisionCardField>

        <DeploymentRevisionCardField label="Type">
          {kindLabel != null ? (
            <Tag
              size={TAG_SIZE.xSmall}
              behavior={TAG_BEHAVIOR.selection}
              hierarchy={TAG_HIERARCHY.secondary}
              color={TAG_COLOR.gray}
              closeable={false}
            >
              {kindLabel}
            </Tag>
          ) : (
            <ParagraphMedium marginTop="0" marginBottom="0">
              —
            </ParagraphMedium>
          )}
        </DeploymentRevisionCardField>

        <DeploymentRevisionCardField label="Source pipeline run">
          <ParagraphMedium marginTop="0" marginBottom="0">
            {sourcePipelineRun != null && sourcePipelineRun !== '' ? sourcePipelineRun : '—'}
          </ParagraphMedium>
        </DeploymentRevisionCardField>
      </div>
    </Box>
  );
}
