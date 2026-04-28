import React from 'react';
import { mergeOverrides } from 'baseui';
import { Banner as BaseBanner } from 'baseui/banner';

import type { BannerProps } from 'baseui/banner';

export const Banner: React.FC<BannerProps> = (props) => {
  const { overrides = {}, children, ...rest } = props;

  const mergedOverrides = mergeOverrides(
    {
      Root: {
        style: {
          marginTop: 0,
          marginRight: 0,
          marginBottom: 0,
          marginLeft: 0,
        },
      },
    },
    overrides
  );

  return (
    <BaseBanner {...rest} overrides={mergedOverrides}>
      {children}
    </BaseBanner>
  );
};
