export type DevProfile = {
  username: string;
  name: string;
  email: string;
  avatarUrl: string;
  /** Raw `?email=` override used to produce this profile, if any; tracked to invalidate the cache when it changes even if the username hasn't. */
  emailOverride?: string;
};

export type GithubUserResponse = {
  name: string | null;
  email: string | null;
  avatar_url: string;
};

export type UseDevProfileResult = Partial<DevProfile> & { loading: boolean };
