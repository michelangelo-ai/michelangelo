import { useStyletron } from 'baseui';

import { Row } from '#core/components/row/row';
import { useStudioQuery } from '#core/hooks/use-studio-query';
import { SHARED_PROJECT_CELL_CONFIG } from './constants';

export function ProjectList() {
  const [css, theme] = useStyletron();

  const { data } = useStudioQuery<{
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
      {data?.projectList.items.map((item, index) => (
        <Row
          overrides={{
            RowItemContainer: {
              style: {
                width: '120px',
              },
            },
          }}
          key={item.metadata.name}
          record={item}
          items={[
            { id: 'metadata.name', label: 'Name', url: item.metadata.name },
            ...SHARED_PROJECT_CELL_CONFIG,
          ].map((cell) => ({
            ...cell,
            ...(index > 0 && { label: undefined }),
          }))}
        />
      ))}
    </div>
  );
}
