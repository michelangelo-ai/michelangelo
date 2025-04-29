import { CoreApp } from '@michelangelo/core';
import { buildRPCQueryHooks, RpcProvider } from '@michelangelo/rpc';

export function App() {
  const { useQuery } = buildRPCQueryHooks();

  return (
    <RpcProvider>
      <CoreApp useQuery={useQuery} />
    </RpcProvider>
  );
}
