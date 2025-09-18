import React from 'react';

import { Execution } from '#core/components/views/execution/execution';
import { DetailViewTablePage } from './pages/detail-view-table-page/detail-view-table-page';

import type {
  CustomDetailPageConfig,
  ExecutionDetailPageConfig,
  TableDetailPageConfig,
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

    case 'table': {
      const tablePage = page as TableDetailPageConfig<T>;
      return (
        <DetailViewTablePage<T>
          isDetailViewLoading={isLoading}
          queryConfig={tablePage.queryConfig}
          tableConfig={tablePage.tableConfig}
          pageId={tablePage.id}
        />
      );
    }

    default:
      return <div>Page type '{page.type}' not yet supported</div>;
  }
}
