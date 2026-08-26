export interface TeamInfo {
  id: string;
  displayName: string;
  url: string;
}

/**
 * @description
 * The service context provided to the application to connect to the services injected
 * into the application.
 *
 * @remarks
 * Since the available requestIds are injected into the application, the parameters and
 * return types are unknown.
 */
export type ServiceContextType = {
  request: (requestId: string, args: unknown, headers?: Record<string, string>) => Promise<unknown>;
  /**
   * Resolvers the RPC layer applies to enrich responses before consumers see them, keyed by
   * concern. `team` is the first user of this pattern (owner UUID -> display info); it's
   * designed so a future resolver (e.g. proto `Any` unpacking, or hydrating a resource
   * reference like a model revision) can be added as another optional key here without a
   * breaking change.
   */
  resolvers?: {
    team?: (uuids: string[]) => Promise<Record<string, TeamInfo>>;
  };
};

export interface ProjectWithOwner {
  spec?: {
    owner?: {
      owningTeam?: string;
      team?: TeamInfo;
    };
  };
}

export interface ProjectResponse {
  project?: ProjectWithOwner;
  projectList?: { items?: ProjectWithOwner[] };
}
