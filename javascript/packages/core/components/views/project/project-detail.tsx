import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useQueryProvider } from '#core/providers/query-provider/use-query-provider';

export function ProjectDetail() {
  const { useQuery } = useQueryProvider();
  const { projectId } = useStudioParams('base');
  const { data } = useQuery<{ project: Record<string, unknown> }>('GetProject', {
    name: projectId,
    namespace: projectId,
  });

  // The project type will not be directly exposed to the @michelangelo/core package.
  // eslint-disable-next-line @typescript-eslint/dot-notation
  return <div>{data?.project?.metadata?.['name']}</div>;
}
