import { useStyletron } from 'baseui';
import { Button, SIZE } from 'baseui/button';
import { ACCESSIBILITY_TYPE, PLACEMENT, StatefulTooltip } from 'baseui/tooltip';

import { Icon } from '#core/components/icon/icon';

import type { ButtonProps } from 'baseui/button';
import type { ActionConfig, Data } from '#core/components/actions/types';

type ActionButtonProps<T extends Data> = {
  action: ActionConfig<T>;
  onClick: () => void;
  loading?: boolean;
  kind: ButtonProps['kind'];
  shape?: ButtonProps['shape'];
  overrides?: ButtonProps['overrides'];
};

export function ActionButton<T extends Data>({
  action,
  onClick,
  loading,
  kind,
  shape,
  overrides,
}: ActionButtonProps<T>) {
  const [css, theme] = useStyletron();
  // Rules are evaluated in order; the first matching one disables the action.
  const disabledRule = action.disabled?.find((rule) => rule.condition);

  const button = (
    <Button
      kind={kind}
      shape={shape}
      size={SIZE.compact}
      isLoading={loading}
      disabled={!!disabledRule}
      overrides={overrides}
      startEnhancer={
        action.display.icon
          ? () => <Icon name={action.display.icon} size={theme.sizing.scale550} color="inherit" />
          : undefined
      }
      onClick={onClick}
    >
      {action.display.label}
    </Button>
  );

  if (!disabledRule?.message) return button;

  // A native disabled <button> doesn't emit pointer events, so anchor the
  // tooltip on a wrapping span to keep the explanation reachable on hover.
  return (
    <StatefulTooltip
      content={disabledRule.message}
      accessibilityType={ACCESSIBILITY_TYPE.tooltip}
      placement={PLACEMENT.bottom}
      showArrow
    >
      <span className={css({ display: 'inline-flex' })}>{button}</span>
    </StatefulTooltip>
  );
}
