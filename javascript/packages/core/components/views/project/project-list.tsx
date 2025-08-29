import { useStyletron } from 'baseui';

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
      <Table
        data={data?.projectList.items ?? []}
        columns={SHARED_PROJECT_CELL_CONFIG}
        loading={isLoading}
        enableStickySides
      />
    </div>
  );
}
