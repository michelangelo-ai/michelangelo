load("@plugin", "buildkite")

def call(operation, parameters):
    return getattr(buildkite, operation)(**parameters)

def invalid_metadata_key():
    return buildkite.trigger_build("smoke-test", metadata = {1: "value"})
