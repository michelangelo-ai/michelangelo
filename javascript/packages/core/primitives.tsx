// Lower-level building blocks for consumers that need to compose their own app
// shell or render components outside of CoreApp's config-driven views.
//
// The main entrypoint (`@michelangelo-ai/core`) exposes the public API: CoreApp,
// Table, Form, DetailView, Execution, and their configuration types. This
// entrypoint provides the providers and components underneath — for consumers
// that need direct access to the pieces CoreApp assembles internally.
//
// New integrations should prefer CoreApp with `dependencies` over mounting
// these providers directly.

// Providers
export { ServiceProvider } from '#core/providers/service-provider/service-provider';
export { ErrorProvider } from '#core/providers/error-provider/error-provider';
export { ThemeProvider } from '#core/themes/theme-provider';
export { IconProvider } from '#core/providers/icon-provider/icon-provider';
export { IconKind } from '#core/components/icon/types';
export * from '#core/providers/icon-provider/types';
export { UserProvider } from '#core/providers/user-provider/user-provider';
export { CellProvider } from '#core/providers/cell-provider/cell-provider';
export { useCellProvider } from '#core/providers/cell-provider/use-cell-provider';
export type { CellContextType } from '#core/providers/cell-provider/types';

// Components
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
export { Banner } from '#core/components/banner/banner';
export { Icon } from '#core/components/icon/icon';
