load('@plugin', __buildkite__='buildkite')

def release(pipeline, branch, commit):
    submitted = __buildkite__.trigger_build(pipeline, branch=branch, commit=commit, message='Model release', env={'RELEASE_CHANNEL': 'staging'}, metadata={'source': 'uniflow'})
    snapshot = __buildkite__.get_build(pipeline, submitted['number'])
    completed = __buildkite__.wait_for_build(pipeline, submitted['number'], timeout_seconds=1800, poll_interval=15)
    return {'snapshot': snapshot, 'completed': completed}