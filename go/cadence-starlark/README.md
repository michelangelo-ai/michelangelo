## Cadence Starlark OSS Get Started

### Run Cadence:

- Run Cadence locally via Docker Compose: https://engwiki.uberinternal.com/display/TE0CADENCE/Run+On+Laptop
- Create "default" domain:
    - `docker run -it --rm --network host ubercadence/cli:master --domain default domain register --retention 1`
    - http://localhost:8088/domains/default/settings

--

### Run Starlark Worker:

- `bazel run server_main`

### Build Client and run  *.star file:

- Build the Client: `bazel build client_main`
- Run *.star file:
  `$WORKSPACE_ROOT/bazel-bin/src/code.uber.internal/uberai/michelangelo/starlark/oss/cadence-starlark/client_main/client_main_/client_main run --file ./integration_test/testdata/ping.star`