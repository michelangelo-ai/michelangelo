import { AppNavBar } from 'baseui/app-nav-bar';

import { Link } from '#core/components/link/link';
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
          <AppNavBar
            title={
              <Link
                href="/"
                overrides={{ Link: { style: { ':hover': { textDecoration: 'unset' } } } }}
              >
                Michelangelo Studio
              </Link>
            }
          />
          <Router />
        </IconProvider>
      </ServiceProvider>
    </ThemeProvider>
  );
}

export { useStudioQuery } from '#core/hooks/use-studio-query';
export { ServiceProvider } from '#core/providers/service-provider/service-provider';

export { useCellToString } from '#core/components/cell/use-cell-to-string';
export { cellTooltipHOC } from '#core/components/cell/components/tooltip/cell-tooltip-hoc';
export { DefaultCellRenderer } from '#core/components/cell/renderers/default-cell-renderer';
export { useGetCellRenderer } from '#core/components/cell/use-get-cell-renderer';
export * from '#core/components/cell/types';
export { CellType } from '#core/components/cell/constants';
export { useCellStyles } from '#core/components/cell/hooks';

export { BooleanCell } from '#core/components/cell/renderers/boolean/boolean-cell';
export { DateCell } from '#core/components/cell/renderers/date/date-cell';
export { DescriptionCell } from '#core/components/cell/renderers/description/description-cell';
export { DescriptionHierarchy } from '#core/components/cell/renderers/description/constants';
export { LinkCell } from '#core/components/cell/renderers/link/link-cell';
export { MultiCell } from '#core/components/cell/renderers/multi/multi-cell';
export { StateCell } from '#core/components/cell/renderers/state/state-cell';
export { TagCell } from '#core/components/cell/renderers/tag/tag-cell';
export { TextCell } from '#core/components/cell/renderers/text/text-cell';
export { TypeCell } from '#core/components/cell/renderers/type/type-cell';

export { Box } from '#core/components/box/box';
export * from '#core/components/box/styled-components';
export { DateTime } from '#core/components/date-time/date-time';
export { DescriptionText } from '#core/components/description-text';
export { HelpTooltip } from '#core/components/help-tooltip';
export { Link } from '#core/components/link/link';
export * from '#core/components/link/styled-components';
export { Markdown } from '#core/components/markdown/markdown';
export { Row } from '#core/components/row/row';
export type { RowCell, RowProps } from '#core/components/row/types';
export { Tag } from '#core/components/tag/tag';
export * from '#core/components/tag/constants';
export type { TagColor, TagHierarchy, TagBehavior, TagSize } from '#core/components/tag/types';
export { TruncatedText } from '#core/components/truncated-text/truncated-text';

export { Icon } from '#core/components/icon/icon';
export { IconKind } from '#core/components/icon/types';
export { IconProvider } from '#core/providers/icon-provider/icon-provider';
export * from '#core/providers/icon-provider/types';

export { ThemeProvider };

export { UserProvider } from '#core/providers/user-provider/user-provider';

export { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
export * from '#core/hooks/routing/use-studio-params/types';
export { useURLQueryString } from '#core/hooks/routing/use-url-query-string';

export * from '#core/utils/object-utils';
export * from '#core/utils/string-utils';
export * from '#core/utils/time-utils';

export * from '#core/types/common/studio-types';
export * from '#core/types/common/view-types';

// Cell Provider
export { CellProvider } from '#core/providers/cell-provider/cell-provider';
export { useCellProvider } from '#core/providers/cell-provider/use-cell-provider';
export type { CellContextType } from '#core/providers/cell-provider/types';

// Error Provider
export { ErrorProvider } from '#core/providers/error-provider/error-provider';
export { useApplicationError } from '#core/providers/error-provider/use-application-error';
export { GrpcStatusCode } from '#core/constants/grpc-status-codes';
