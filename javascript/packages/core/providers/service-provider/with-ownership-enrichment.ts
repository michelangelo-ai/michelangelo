import type { ProjectResponse, ProjectWithOwner, ServiceContextType, TeamInfo } from './types';

function collectOwningTeamUuids(projects: ProjectWithOwner[]): string[] {
  const uuids = new Set<string>();
  for (const project of projects) {
    const uuid = project.spec?.owner?.owningTeam;
    if (uuid) uuids.add(uuid);
  }
  return [...uuids];
}

function applyTeams(projects: ProjectWithOwner[], teams: Record<string, TeamInfo>) {
  for (const project of projects) {
    const uuid = project.spec?.owner?.owningTeam;
    const team = uuid ? teams[uuid] : undefined;
    if (team && project.spec?.owner) {
      project.spec.owner.team = team;
    }
  }
}

/**
 * Wraps a `request` function to enrich GetProject/ListProject responses with team display
 * info, resolved from owner UUIDs via `resolveTeams`. On resolver failure or a UUID missing
 * from the resolved map, the project is left with its raw `owningTeam` UUID, which the
 * Owner column already falls back to displaying.
 */
export function withOwnershipEnrichment(
  request: ServiceContextType['request'],
  resolveTeams?: (uuids: string[]) => Promise<Record<string, TeamInfo>>
): ServiceContextType['request'] {
  if (!resolveTeams) return request;

  return async (requestId, args, headers) => {
    // cast: request's response type is unknown at this layer; narrowing to the minimal
    // project shape needed to enrich the owner field
    const response = (await request(requestId, args, headers)) as ProjectResponse;

    try {
      if (requestId === 'GetProject' && response?.project) {
        const teams = await resolveTeams(collectOwningTeamUuids([response.project]));
        applyTeams([response.project], teams);
      } else if (requestId === 'ListProject' && response?.projectList?.items) {
        const teams = await resolveTeams(collectOwningTeamUuids(response.projectList.items));
        applyTeams(response.projectList.items, teams);
      }
    } catch {
      // Enrichment is best-effort; leave the raw owningTeam UUID in place on failure.
    }

    return response;
  };
}
