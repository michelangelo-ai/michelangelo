export interface Pipeline {
  metadata: {
    name: string;
    namespace: string;
  };
  spec: {
    owner: {
      name: string;
    };
  };
}
