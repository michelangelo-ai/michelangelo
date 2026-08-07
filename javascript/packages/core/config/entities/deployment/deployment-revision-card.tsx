import { useStyletron } from 'baseui';
import { ARTWORK_TYPE } from 'baseui/banner';
import { Spinner } from 'baseui/spinner';
import { LabelSmall, ParagraphMedium } from 'baseui/typography';

import { Banner } from '#core/components/banner/banner';
import { Box } from '#core/components/box/box';
import { Icon } from '#core/components/icon/icon';

import type { ResourceRef } from './types';

const CARD_OVERRIDES = { BoxContainer: { style: { flexBasis: '320px', flexGrow: 1 } } };

/**
 * Renders a card for one of a deployment's revision references (current,
 * candidate, or desired). Shows only what is present on the Deployment CR
 * itself — the revision name — since OSS has no producer of Model revisions to
 * resolve richer model metadata from.
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

  if (isLoading) {
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

  return (
    <Box title={title} overrides={CARD_OVERRIDES}>
      <LabelSmall color={theme.colors.contentSecondary} marginBottom={theme.sizing.scale200}>
        Revision
      </LabelSmall>
      <ParagraphMedium marginTop="0" marginBottom="0">
        {revision.name}
      </ParagraphMedium>
    </Box>
  );
}
