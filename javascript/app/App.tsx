import { CoreApp } from '@michelangelo/core';
import { buildRPCQueryHooks, RpcProvider } from '@michelangelo/rpc';

const dependencies = {
  service: {
    ...buildRPCQueryHooks(),
  },
};

export function App() {
  return (
    <RpcProvider>
      <CoreApp dependencies={dependencies} />
    </RpcProvider>
  );
}
