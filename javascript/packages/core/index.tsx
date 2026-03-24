import { AppNavBar } from 'baseui/app-nav-bar';

import { Link } from '#core/components/link/link';
import { ErrorProvider } from '#core/providers/error-provider/error-provider';
import { IconProvider } from '#core/providers/icon-provider/icon-provider';
import { ServiceProvider } from '#core/providers/service-provider/service-provider';
import { Router } from '#core/router/router';
import { ThemeProvider } from '#core/themes/provider';

import type { ErrorContextValue } from '#core/providers/error-provider/types';
import type { IconProviderContext } from '#core/providers/icon-provider/types';
import type { ServiceContextType } from '#core/providers/service-provider/types';

import '#core/styles/main.css';
// TODO: Relocate the Props interface once the contents of the
// packages/core/index.tsx file are moved to a final location
type Props = {
  dependencies: {
    error: ErrorContextValue;
    service: ServiceContextType;
    theme: IconProviderContext;
  };
};

export function CoreApp({ dependencies }: Props) {
  return (
    <ThemeProvider icons={dependencies.theme.icons}>
      <ServiceProvider {...dependencies.service}>
        <ErrorProvider {...dependencies.error}>
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
        </ErrorProvider>
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
export { Banner } from '#core/components/banner/banner';

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
export { ApplicationError } from '#core/types/error-types';
export type { ErrorNormalizer } from '#core/types/error-types';
export { ErrorProvider } from '#core/providers/error-provider/error-provider';
export { useErrorNormalizer } from '#core/providers/error-provider/use-error-normalizer';
export { GrpcStatusCode } from '#core/constants/grpc-status-codes';

// Interpolation
export { interpolate } from '#core/interpolation/interpolate';
export { Interpolation } from '#core/interpolation/base';
export { StringInterpolation } from '#core/interpolation/string-interpolation';
export { FunctionInterpolation } from '#core/interpolation/function-interpolation';
export { useInterpolationResolver } from '#core/interpolation/use-interpolation-resolver';
export type {
  InterpolationContext,
  InterpolationContextExtensions,
  Interpolatable,
  UserDataSources,
} from '#core/interpolation/types';
export { isInterpolation } from '#core/interpolation/utils/is-interpolation';
export { hasInterpolationProperty } from '#core/interpolation/utils/has-interpolation-property';
export { clearUnresolvedInterpolations } from '#core/interpolation/utils/clear-unresolved-interpolations';

// Interpolation Providers
export { InterpolationProvider } from '#core/providers/interpolation-provider/interpolation-provider';
export { RepeatedLayoutProvider } from '#core/providers/repeated-layout-provider/repeated-layout-provider';
export { useRepeatedLayoutContext } from '#core/providers/repeated-layout-provider/use-repeated-layout-context';
export type { RepeatedLayoutState } from '#core/providers/repeated-layout-provider/types';

// Table
export { Table } from '#core/components/table/table';
export { useLocalStorageTableState } from '#core/components/table/plugins/state-persistence/use-local-storage-table-state';
export { useTableSelectionContext } from '#core/components/table/plugins/selection/table-selection-context';
export { FilterMode } from '#core/components/table/components/filter/types';
export type { ColumnConfig } from '#core/components/table/types/column-types';
export type { TableProps } from '#core/components/table/types/table-types';

// Execution View
export { Execution } from '#core/components/views/execution/execution';
export type {
  ExecutionDetailViewSchema,
  ExecutionOverrides,
  Task,
  TaskState,
} from '#core/components/views/execution/types';
export type { TaskListRendererProps } from '#core/components/views/execution/components/types';
export { TASK_STATE } from '#core/components/views/execution/constants';
export { TaskListRenderer } from '#core/components/views/execution/components/task-list-renderer';
export { TaskStepCard } from '#core/components/views/execution/components/task-step-card/task-step-card';

// Form
export { Form } from '#core/components/form/form';
export { useForm } from '#core/components/form/hooks/use-form';
export { useFormState } from '#core/components/form/hooks/use-form-state';
export { useField } from '#core/components/form/hooks/use-field';
export type { FormState } from '#core/components/form/types';
export { FormControl } from '#core/components/form/components/form-control';
export { FormErrorBanner } from '#core/components/form/components/form-error-banner/form-error-banner';
export { FormDialog } from '#core/components/form/components/form-dialog/form-dialog';
export { AddButton } from '#core/components/form/components/add-button/add-button';
export { SubmitButton } from '#core/components/form/components/submit-button/submit-button';

// Form Fields
export { StringField } from '#core/components/form/fields/string/string-field';
export { SelectField } from '#core/components/form/fields/select/select-field';
export { BooleanField } from '#core/components/form/fields/boolean/boolean-field';
export { RadioField } from '#core/components/form/fields/radio/radio-field';
export { TextareaField } from '#core/components/form/fields/textarea/textarea-field';
export { UrlField } from '#core/components/form/fields/url/url-field';
export { NumberField } from '#core/components/form/fields/number/number-field';
export { CheckboxField } from '#core/components/form/fields/checkbox/checkbox-field';
export { DateField } from '#core/components/form/fields/date/date-field';
export { DATE_FORMAT } from '#core/components/form/fields/date/types';
export { MarkdownField } from '#core/components/form/fields/markdown/markdown-field';

// Form Layout
export { FormGroup } from '#core/components/form/layout/form-group/form-group';
export { FormRow } from '#core/components/form/layout/form-row/form-row';
export { FormColumn } from '#core/components/form/layout/form-column/form-column';
export { FormBanner } from '#core/components/form/layout/form-banner/form-banner';
export { FormStep } from '#core/components/form/layout/form-step/form-step';
export { ArrayFormRow } from '#core/components/form/layout/array-form-row/array-form-row';
export { ArrayFormGroup } from '#core/components/form/layout/array-form-group/array-form-group';
