import React from 'react';

import { Execution } from '#core/components/views/execution/execution';

import type {
  CustomDetailPageConfig,
  ExecutionDetailPageConfig,
} from '#core/components/views/detail-view/types/detail-view-schema-types';
import type { PageRendererProps } from './types';

export function DetailViewPageRenderer<T extends object = object>({
  page,
  data,
  isLoading,
}: PageRendererProps<T>) {
  switch (page.type) {
    case 'custom':
      return React.createElement((page as CustomDetailPageConfig).component, { data, isLoading });

    case 'execution':
      return <Execution schema={page as ExecutionDetailPageConfig} data={data ?? {}} />;

    default:
      return <div>Page type '{page.type}' not yet supported</div>;
  }
}
