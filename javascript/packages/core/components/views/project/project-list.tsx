import { useStyletron } from 'baseui';

import { PageHeader } from '#core/components/page-header/page-header';
import { Table } from '#core/components/table/table';
import { useStudioQuery } from '#core/hooks/use-studio-query';
import { SHARED_PROJECT_CELL_CONFIG } from './constants';

export function ProjectList() {
  const [css, theme] = useStyletron();

  const { data, isLoading } = useStudioQuery<{
    projectList: {
      items: Array<{
        metadata: {
          name: string;
        };
        spec: {
          description: string;
          owner: {
            owningTeam: string;
          };
          tier: string;
        };
      }>;
    };
  }>({
    queryName: 'ListProject',
    serviceOptions: { namespace: '' },
  });

  return (
    <div className={css({ marginTop: theme.sizing.scale400 })}>
      <div
        className={css({ paddingTop: theme.sizing.scale400, paddingBottom: theme.sizing.scale400 })}
      >
        <PageHeader label="All Projects" />
      </div>
      <Table
        data={data?.projectList.items ?? []}
        columns={SHARED_PROJECT_CELL_CONFIG}
        loading={isLoading}
      />
    </div>
  );
}
