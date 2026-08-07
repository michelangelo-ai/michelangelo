import { useStyletron } from 'baseui';
import { HeadingXSmall, LabelSmall, ParagraphMedium, ParagraphSmall } from 'baseui/typography';

import type { ResourceRef } from './types';

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
}: {
  title: string;
  revision?: ResourceRef;
  emptyText: string;
}) {
  const [css, theme] = useStyletron();

  const cardStyles = css({
    flexBasis: '320px',
    flexGrow: 1,
    padding: theme.sizing.scale700,
    borderTopLeftRadius: theme.borders.radius300,
    borderTopRightRadius: theme.borders.radius300,
    borderBottomLeftRadius: theme.borders.radius300,
    borderBottomRightRadius: theme.borders.radius300,
    borderTopStyle: 'solid',
    borderRightStyle: 'solid',
    borderBottomStyle: 'solid',
    borderLeftStyle: 'solid',
    borderTopWidth: '1px',
    borderRightWidth: '1px',
    borderBottomWidth: '1px',
    borderLeftWidth: '1px',
    borderTopColor: theme.colors.borderOpaque,
    borderRightColor: theme.colors.borderOpaque,
    borderBottomColor: theme.colors.borderOpaque,
    borderLeftColor: theme.colors.borderOpaque,
  });

  const cardTitle = (
    <HeadingXSmall marginTop="0" marginBottom={theme.sizing.scale600}>
      {title}
    </HeadingXSmall>
  );

  if (!revision?.name) {
    return (
      <div className={cardStyles}>
        {cardTitle}
        <ParagraphSmall marginTop="0" marginBottom="0" color={theme.colors.contentSecondary}>
          {emptyText}
        </ParagraphSmall>
      </div>
    );
  }

  return (
    <div className={cardStyles}>
      {cardTitle}
      <LabelSmall color={theme.colors.contentSecondary} marginBottom={theme.sizing.scale200}>
        Revision
      </LabelSmall>
      <ParagraphMedium marginTop="0" marginBottom="0">
        {revision.name}
      </ParagraphMedium>
    </div>
  );
}
