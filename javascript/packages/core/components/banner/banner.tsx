import React from 'react';
import { mergeOverrides } from 'baseui';
import { Banner as BaseBanner } from 'baseui/banner';

import type { BannerProps } from 'baseui/banner';

export const Banner: React.FC<BannerProps> = (props) => {
  const { overrides = {}, children, ...rest } = props;

  const mergedOverrides = mergeOverrides(overrides, {
    Root: {
      style: {
        marginTop: 0,
        marginRight: 0,
        marginBottom: 0,
        marginLeft: 0,
      },
    },
  });

  return (
    <BaseBanner
      {...rest}
      overrides={{
        BelowContent: mergedOverrides.BelowContent,
        LeadingContent: mergedOverrides.LeadingContent,
        Message: mergedOverrides.Message,
        MessageContent: mergedOverrides.MessageContent,
        Root: mergedOverrides.Root,
        Title: mergedOverrides.Title,
        TrailingContent: mergedOverrides.TrailingContent,
        TrailingButtonContainer: mergedOverrides.TrailingButtonContainer,
        TrailingIconButton: mergedOverrides.TrailingIconButton,
      }}
    >
      {children}
    </BaseBanner>
  );
};
