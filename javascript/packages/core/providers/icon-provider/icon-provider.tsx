import React from 'react';

import { IconContext } from './icon-context';

import type { IconProviderContext } from './types';

export const IconProvider: React.FC<{ children: React.ReactNode } & IconProviderContext> = ({
  children,
  ...iconContext
}) => {
  return <IconContext.Provider value={iconContext}>{children}</IconContext.Provider>;
};
