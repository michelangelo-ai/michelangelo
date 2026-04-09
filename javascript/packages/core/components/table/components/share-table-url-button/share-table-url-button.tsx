import { useState } from 'react';
import { Button, KIND, SIZE } from 'baseui/button';
import { useStyletron } from 'baseui';

import { Icon } from '#core/components/icon/icon';

import type { TableState } from '#core/components/table/types/table-types';

type ShareTableUrlButtonProps = {
  buildShareUrl: (state: Partial<TableState>) => string;
  currentState: Partial<TableState>;
};

export function ShareTableUrlButton({ buildShareUrl, currentState }: ShareTableUrlButtonProps) {
  const [css, theme] = useStyletron();
  const [copied, setCopied] = useState(false);

  const handleClick = () => {
    const url = buildShareUrl(currentState);
    void navigator.clipboard.writeText(url).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  };

  return (
    <Button kind={KIND.secondary} size={SIZE.compact} onClick={handleClick}>
      <Icon name={copied ? 'circleCheck' : 'arrowLaunch'} />
      <span className={css({ marginLeft: theme.sizing.scale300 })}>
        {copied ? 'Copied!' : 'Share'}
      </span>
    </Button>
  );
}
