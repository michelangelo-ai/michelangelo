import React from 'react';

import { Execution } from '#core/components/views/execution/execution';
import { DetailViewTablePage } from './pages/detail-view-table-page/detail-view-table-page';

import type { PageRendererProps } from './types';

export function DetailViewPageRenderer<T extends object = object>({
  page,
  data,
  isLoading,
}: PageRendererProps<T>) {
  switch (page.type) {
    case 'custom':
      return React.createElement(page.component, { data, isLoading });

    case 'execution':
      return <Execution schema={page} data={data ?? {}} />;

    case 'table': {
      const tablePage = page;
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
      // cast: page is never in the type-safe path; runtime guard for unknown page types from
      // external config
      return <div>Page type &apos;{(page as { type: string }).type}&apos; not yet supported</div>;
  }
}
