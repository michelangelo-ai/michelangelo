import { withOwnershipEnrichment } from '../with-ownership-enrichment';

describe('withOwnershipEnrichment', () => {
  it('passes through unchanged when no resolver is registered', async () => {
    const request = vi
      .fn()
      .mockResolvedValue({ project: { spec: { owner: { owningTeam: 'uuid-1' } } } });
    const wrapped = withOwnershipEnrichment(request);

    const response = await wrapped('GetProject', {});

    expect(response).toEqual({ project: { spec: { owner: { owningTeam: 'uuid-1' } } } });
  });

  it('enriches a GetProject response with the resolved team', async () => {
    const team = { id: 'team-1', displayName: 'Team One', url: 'https://example.com/team-1' };
    const request = vi
      .fn()
      .mockResolvedValue({ project: { spec: { owner: { owningTeam: 'uuid-1' } } } });
    const resolveTeams = vi.fn().mockResolvedValue({ 'uuid-1': team });
    const wrapped = withOwnershipEnrichment(request, resolveTeams);

    const response = await wrapped('GetProject', {});

    expect(resolveTeams).toHaveBeenCalledWith(['uuid-1']);
    expect(response).toEqual({ project: { spec: { owner: { owningTeam: 'uuid-1', team } } } });
  });

  it('batches all owning team UUIDs in a single resolver call for ListProject', async () => {
    const team = { id: 'team-1', displayName: 'Team One', url: 'https://example.com/team-1' };
    const request = vi.fn().mockResolvedValue({
      projectList: {
        items: [
          { spec: { owner: { owningTeam: 'uuid-1' } } },
          { spec: { owner: { owningTeam: 'uuid-2' } } },
        ],
      },
    });
    const resolveTeams = vi.fn().mockResolvedValue({ 'uuid-1': team, 'uuid-2': team });
    const wrapped = withOwnershipEnrichment(request, resolveTeams);

    const response = await wrapped('ListProject', {});

    expect(resolveTeams).toHaveBeenCalledTimes(1);
    expect(resolveTeams).toHaveBeenCalledWith(['uuid-1', 'uuid-2']);
    expect(response).toEqual({
      projectList: {
        items: [
          { spec: { owner: { owningTeam: 'uuid-1', team } } },
          { spec: { owner: { owningTeam: 'uuid-2', team } } },
        ],
      },
    });
  });

  it('leaves the raw owningTeam UUID in place when the resolver throws', async () => {
    const request = vi
      .fn()
      .mockResolvedValue({ project: { spec: { owner: { owningTeam: 'uuid-1' } } } });
    const resolveTeams = vi.fn().mockRejectedValue(new Error('lookup failed'));
    const wrapped = withOwnershipEnrichment(request, resolveTeams);

    const response = await wrapped('GetProject', {});

    expect(response).toEqual({ project: { spec: { owner: { owningTeam: 'uuid-1' } } } });
  });

  it('leaves the raw owningTeam UUID in place when the resolver omits a UUID', async () => {
    const request = vi
      .fn()
      .mockResolvedValue({ project: { spec: { owner: { owningTeam: 'uuid-1' } } } });
    const resolveTeams = vi.fn().mockResolvedValue({});
    const wrapped = withOwnershipEnrichment(request, resolveTeams);

    const response = await wrapped('GetProject', {});

    expect(response).toEqual({ project: { spec: { owner: { owningTeam: 'uuid-1' } } } });
  });

  it('does not enrich responses for other RPCs', async () => {
    const request = vi.fn().mockResolvedValue({ pipeline: { spec: {} } });
    const resolveTeams = vi.fn();
    const wrapped = withOwnershipEnrichment(request, resolveTeams);

    const response = await wrapped('GetPipeline', {});

    expect(resolveTeams).not.toHaveBeenCalled();
    expect(response).toEqual({ pipeline: { spec: {} } });
  });
});
