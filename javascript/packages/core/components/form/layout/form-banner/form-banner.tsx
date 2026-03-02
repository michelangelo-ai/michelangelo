import React, { useState } from 'react';

import { Banner } from '#core/components/banner/banner';
import { Icon } from '#core/components/icon/icon';
import { Markdown } from '#core/components/markdown/markdown';

import type { FormBannerProps } from './types';

export const FormBanner: React.FC<FormBannerProps> = ({
  title,
  kind = 'info',
  dismissible = false,
  content,
}) => {
  const [dismissed, setDismissed] = useState(false);

  if (dismissed) return null;

  return (
    <Banner
      title={title}
      kind={kind}
      action={
        dismissible
          ? {
              label: 'Dismiss',
              icon: () => <Icon name="x-filled" />,
              onClick: () => setDismissed(true),
            }
          : undefined
      }
    >
      {typeof content === 'string' ? <Markdown>{content}</Markdown> : content}
    </Banner>
  );
};
