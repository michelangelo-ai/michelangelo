import { renderHook } from '@testing-library/react';

import { ServiceProvider } from '../service-provider';
import { useServiceProvider } from '../use-service-provider';

function renderRequest(props: Parameters<typeof ServiceProvider>[0]) {
  const { result } = renderHook(() => useServiceProvider(), {
    wrapper: ({ children }) => <ServiceProvider {...props}>{children}</ServiceProvider>,
  });
  return result.current.request;
}

describe('ServiceProvider ownership enrichment', () => {
  it('passes requests through unchanged when no team resolver is registered', async () => {
    const request = vi
      .fn()
      .mockResolvedValue({ project: { spec: { owner: { owningTeam: 'uuid-1' } } } });

    const response = await renderRequest({ children: null, request })('GetProject', {});

    expect(response).toEqual({ project: { spec: { owner: { owningTeam: 'uuid-1' } } } });
  });

  it('enriches a GetProject response with the team resolved via a registered resolver', async () => {
    const team = { id: 'uuid-1', displayName: 'Team One', url: 'https://example.com/team-1' };
    const request = vi
      .fn()
      .mockResolvedValue({ project: { spec: { owner: { owningTeam: 'uuid-1' } } } });
    const resolveTeams = vi.fn().mockResolvedValue({ 'uuid-1': team });

    const response = await renderRequest({
      children: null,
      request,
      resolvers: { team: resolveTeams },
    })('GetProject', {});

    expect(resolveTeams).toHaveBeenCalledWith(['uuid-1']);
    expect(response).toEqual({ project: { spec: { owner: { owningTeam: 'uuid-1', team } } } });
  });

  it('batches all owning team UUIDs into a single resolver call for ListProject and assigns each project its own team', async () => {
    const teamOne = { id: 'uuid-1', displayName: 'Team One', url: 'https://example.com/team-1' };
    const teamTwo = { id: 'uuid-2', displayName: 'Team Two', url: 'https://example.com/team-2' };
    const request = vi.fn().mockResolvedValue({
      projectList: {
        items: [
          { spec: { owner: { owningTeam: 'uuid-1' } } },
          { spec: { owner: { owningTeam: 'uuid-2' } } },
        ],
      },
    });
    const resolveTeams = vi.fn().mockResolvedValue({ 'uuid-1': teamOne, 'uuid-2': teamTwo });

    const response = await renderRequest({
      children: null,
      request,
      resolvers: { team: resolveTeams },
    })('ListProject', {});

    expect(resolveTeams).toHaveBeenCalledTimes(1);
    expect(resolveTeams).toHaveBeenCalledWith(['uuid-1', 'uuid-2']);
    expect(response).toEqual({
      projectList: {
        items: [
          { spec: { owner: { owningTeam: 'uuid-1', team: teamOne } } },
          { spec: { owner: { owningTeam: 'uuid-2', team: teamTwo } } },
        ],
      },
    });
  });

  it('leaves the raw owningTeam UUID in place when the resolver throws', async () => {
    const request = vi
      .fn()
      .mockResolvedValue({ project: { spec: { owner: { owningTeam: 'uuid-1' } } } });
    const resolveTeams = vi.fn().mockRejectedValue(new Error('lookup failed'));

    const response = await renderRequest({
      children: null,
      request,
      resolvers: { team: resolveTeams },
    })('GetProject', {});

    expect(response).toEqual({ project: { spec: { owner: { owningTeam: 'uuid-1' } } } });
  });

  it('leaves the raw owningTeam UUID in place when the resolver omits it from the result', async () => {
    const request = vi
      .fn()
      .mockResolvedValue({ project: { spec: { owner: { owningTeam: 'uuid-1' } } } });
    const resolveTeams = vi.fn().mockResolvedValue({});

    const response = await renderRequest({
      children: null,
      request,
      resolvers: { team: resolveTeams },
    })('GetProject', {});

    expect(response).toEqual({ project: { spec: { owner: { owningTeam: 'uuid-1' } } } });
  });

  it('does not enrich responses for other RPCs', async () => {
    const request = vi.fn().mockResolvedValue({ pipeline: { spec: {} } });
    const resolveTeams = vi.fn();

    const response = await renderRequest({
      children: null,
      request,
      resolvers: { team: resolveTeams },
    })('GetPipeline', {});

    expect(resolveTeams).not.toHaveBeenCalled();
    expect(response).toEqual({ pipeline: { spec: {} } });
  });
});
