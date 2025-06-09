import { AppNavBar } from 'baseui/app-nav-bar';

import { IconProvider } from '#core/providers/icon-provider/icon-provider';
import { ServiceProvider } from '#core/providers/service-provider/service-provider';
import { Router } from '#core/router/router';
import { ThemeProvider } from '#core/themes/provider';

import type { IconProviderContext } from '#core/providers/icon-provider/types';
import type { ServiceContextType } from '#core/providers/service-provider/types';

import '#core/styles/main.css';
// TODO: Relocate the Props interface once the contents of the
// packages/core/index.tsx file are moved to a final location
type Props = {
  dependencies: {
    service: ServiceContextType;
    theme: IconProviderContext;
  };
};

export function CoreApp({ dependencies }: Props) {
  return (
    <ThemeProvider>
      <ServiceProvider {...dependencies.service}>
        <IconProvider icons={dependencies.theme.icons}>
          <AppNavBar title="Michelangelo Studio" />
          <Router />
        </IconProvider>
      </ServiceProvider>
    </ThemeProvider>
  );
}

export { useStudioQuery } from '#core/hooks/use-studio-query';
export { ServiceProvider } from '#core/providers/service-provider/service-provider';

export { cellToString } from '#core/components/cell/cell-to-string';
export { DefaultCellRenderer } from '#core/components/cell/renderers/default-cell-renderer';
export { getCellRenderer } from '#core/components/cell/get-cell-renderer';
export * from '#core/components/cell/types';

export { BooleanCell } from '#core/components/cell/renderers/boolean/boolean-cell';
export { DateCell } from '#core/components/cell/renderers/date/date-cell';
export { DescriptionCell } from '#core/components/cell/renderers/description/description-cell';
export { DescriptionHierarchy } from '#core/components/cell/renderers/description/constants';
export { LinkCell } from '#core/components/cell/renderers/link/link-cell';
export { MultiCell } from '#core/components/cell/renderers/multi/multi-cell';
export { StateCell } from '#core/components/cell/renderers/state/state-cell';
export { TextCell } from '#core/components/cell/renderers/text/text-cell';
export { TypeCell } from '#core/components/cell/renderers/type/type-cell';

export { Box } from '#core/components/box/box';
export { DateTime } from '#core/components/date-time/date-time';
export { DescriptionText } from '#core/components/description-text';
export { HelpTooltip } from '#core/components/help-tooltip';
export { Link } from '#core/components/link/link';
export { Markdown } from '#core/components/markdown/markdown';
export { Row } from '#core/components/row/row';
export { Tag } from '#core/components/tag/tag';
export { TruncatedText } from '#core/components/truncated-text/truncated-text';

export { Icon } from '#core/components/icon/icon';
export { IconKind } from '#core/components/icon/types';
export { IconProvider } from '#core/providers/icon-provider/icon-provider';

export { UserProvider } from '#core/providers/user-provider/user-provider';

export * from '#core/utils/object-utils';
export * from '#core/utils/string-utils';
export * from '#core/utils/time-utils';
