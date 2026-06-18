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
      return React.createElement((page as CustomDetailPageConfig).component, { data, isLoading }); // cast: type discriminant ensures page is CustomDetailPageConfig in this case branch

    case 'execution':
      return <Execution schema={page as ExecutionDetailPageConfig} data={data ?? {}} />; // cast: type discriminant ensures page is ExecutionDetailPageConfig in this case branch

    case 'table': {
      const tablePage = page as TableDetailPageConfig<T>; // cast: type discriminant ensures page is TableDetailPageConfig in this case branch
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
