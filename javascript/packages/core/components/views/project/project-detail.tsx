import { Row } from '#core/components/row/row';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioQuery } from '#core/hooks/use-studio-query';
import { CellType } from '#core/components/cell/constants';
import { useStyletron } from 'baseui';

export function ProjectDetail() {
  const [css, theme] = useStyletron();
  const { projectId } = useStudioParams('base');
  const { data } = useStudioQuery<{ project: Record<string, string> }>({
    queryName: 'GetProject',
    serviceOptions: {
      name: projectId,
      namespace: projectId,
    },
  });

  return (
    <div className={css({ marginTop: theme.sizing.scale400 })}>
      <Row
        items={[
          {
            id: 'metadata.name',
            label: 'Name',
            items: [
              {
                id: 'metadata.name',
              },
              {
                id: 'spec.description',
                type: CellType.DESCRIPTION,
              },
            ],
          },
          {
            id: 'metadata.creationTimestamp.seconds',
            label: 'Created',
            type: CellType.DATE,
          },
          {
            id: 'spec.owner.owningTeam',
            label: 'Owner',
          },
          {
            id: 'spec.tier',
            label: 'Tier',
            type: CellType.TAG,
          },
        ]}
        record={data?.project}
      />
    </div>
  );
}
