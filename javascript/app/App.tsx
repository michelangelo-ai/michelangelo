import { CoreApp } from '@michelangelo/core';
import { request } from '@michelangelo/rpc';

const dependencies = {
  service: {
    request,
  },
};

export function App() {
  return <CoreApp dependencies={dependencies} />;
}
