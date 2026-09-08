export interface ProjectOwnerData {
  spec?: {
    owner?: {
      team?: { displayName?: string; url?: string };
      owningTeam?: string;
    };
  };
}
