export type DevProfile = {
  username: string;
  name: string;
  email: string;
  avatarUrl: string;
  /** `?email=` override in effect when this profile was cached, if any. */
  emailOverride?: string;
};

export type GithubUserResponse = {
  name: string | null;
  email: string | null;
  avatar_url: string;
};

export type UseDevProfileResult = Partial<DevProfile> & { loading: boolean };
