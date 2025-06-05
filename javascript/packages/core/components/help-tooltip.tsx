import { ReactNode } from 'react';
import { useStyletron } from 'baseui';
import { ACCESSIBILITY_TYPE, PLACEMENT, StatefulTooltip } from 'baseui/tooltip';

import { Icon } from '#core/components/icon/icon';
import { IconKind } from '#core/components/icon/types';
import { Markdown } from '#core/components/markdown/markdown';

export function HelpTooltip({ text }: { text: string | ReactNode }) {
  const [css] = useStyletron();

  return (
    <StatefulTooltip
      showArrow
      returnFocus
      autoFocus
      placement={PLACEMENT.top}
      accessibilityType={ACCESSIBILITY_TYPE.tooltip}
      content={() => (
        <div className={css({ maxWidth: '400px' })}>
          <Markdown>{text}</Markdown>
        </div>
      )}
    >
      <span className={css({ cursor: 'help', display: 'flex' })}>
        <Icon kind={IconKind.TERTIARY} name="circleI" title="help" />
      </span>
    </StatefulTooltip>
  );
}
