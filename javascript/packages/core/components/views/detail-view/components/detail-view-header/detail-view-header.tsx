import { useStyletron } from 'baseui';
import { Button, KIND, SHAPE, SIZE } from 'baseui/button';

import { ActionsButtons } from '#core/components/actions/actions-buttons/actions-buttons';
import { Icon } from '#core/components/icon/icon';
import { ELLIPSIS_STYLES } from '#core/styles/constants';
import { DetailHeaderContainer } from './styled-components';

import type { Theme } from 'baseui/theme';
import type { DetailViewHeaderProps } from './types';

export function DetailViewHeader({
  subtitle,
  title,
  onGoBack,
  children,
  actions,
  record,
  loading,
}: DetailViewHeaderProps) {
  const [css, theme] = useStyletron();

  return (
    <DetailHeaderContainer>
      <div
        className={css({
          display: 'flex',
          gap: theme.sizing.scale800,
          justifyContent: 'flex-start',
          alignItems: 'center',
        })}
      >
        <h5 className={css({ margin: 0, maxWidth: '50%' })}>
          {subtitle && (
            <div
              className={css({
                ...theme.typography.LabelSmall,
                color: theme.colors.contentTertiary,
                marginBottom: theme.sizing.scale300,
              })}
            >
              {subtitle}
            </div>
          )}
          <div
            className={css({ display: 'flex', alignItems: 'center', gap: theme.sizing.scale300 })}
          >
            {onGoBack && (
              <Button
                aria-label="Go back"
                onClick={onGoBack}
                kind={KIND.tertiary}
                shape={SHAPE.circle}
                size={SIZE.compact}
                overrides={{
                  BaseButton: {
                    style: ({ $theme }: { $theme: Theme }) => ({
                      flexShrink: 0,
                      ':hover': {
                        backgroundColor: $theme.colors.contentInverseSecondary,
                      },
                    }),
                  },
                }}
              >
                <Icon name="arrowLeft" size={theme.sizing.scale700} />
              </Button>
            )}
            {/* TODO: #349 Integrate with TruncatedText component */}
            <div className={css({ ...theme.typography.HeadingSmall, ...ELLIPSIS_STYLES })}>
              {title}
            </div>
          </div>
        </h5>
        {actions && (
          <div className={css({ marginLeft: 'auto', flexShrink: 0 })}>
            <ActionsButtons actions={actions} record={record ?? {}} loading={loading} />
          </div>
        )}
      </div>

      {children}
    </DetailHeaderContainer>
  );
}
