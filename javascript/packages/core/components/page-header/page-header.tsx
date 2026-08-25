import { useStyletron } from 'baseui';
import { Button, KIND, SHAPE, SIZE } from 'baseui/button';
import { HeadingSmall } from 'baseui/typography';

import { DescriptionText } from '#core/components/description-text';
import { Icon } from '#core/components/icon/icon';

import type { PageHeaderProps } from './types';

/**
 * Page header component for displaying page titles, icons, descriptions, and documentation links.
 *
 * @example
 * ```tsx
 * // Phase header with all features
 * <PageHeader
 *   icon="chartLine"
 *   label="Train & Evaluate"
 *   description="Train machine learning models and evaluate their performance"
 *   docUrl="https://docs.example.com/train"
 * />
 *
 * // Simple header without icon
 * <PageHeader label="My Page Title" />
 * ```
 */
export function PageHeader({ icon, docUrl, label, description }: PageHeaderProps) {
  const [css, theme] = useStyletron();

  return (
    <div
      className={css({ display: 'flex', justifyContent: 'space-between', alignItems: 'center' })}
    >
      <div
        className={css({
          display: 'grid',
          gridTemplateColumns: icon ? 'auto 1fr' : '1fr',
          gridTemplateRows: 'auto auto',
          columnGap: theme.sizing.scale500,
          rowGap: theme.sizing.scale100,
          alignItems: 'center',
        })}
      >
        {icon && <Icon name={icon} size={theme.sizing.scale600} />}
        <HeadingSmall as="h2" className={css({ margin: 0 })}>
          {label}
        </HeadingSmall>
        {description && icon && <div />}
        {description && (
          <DescriptionText>
            {description}
            {docUrl && (
              <Button
                aria-label="Learn more"
                kind={KIND.tertiary}
                onClick={() => window.open(docUrl, '_blank')}
                shape={SHAPE.circle}
                size={SIZE.mini}
                overrides={{
                  BaseButton: {
                    style: {
                      // Default Button height includes container for hover effect, which results in
                      // unwanted vertical space between the description text and the label
                      height: theme.typography.ParagraphSmall.lineHeight,
                      width: theme.typography.ParagraphSmall.lineHeight,
                    },
                  },
                }}
              >
                <Icon name="arrowLaunch" size={theme.sizing.scale500} />
              </Button>
            )}
          </DescriptionText>
        )}
      </div>
    </div>
  );
}
