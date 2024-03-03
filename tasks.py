from invoke import run, task

@task
def test(ctx):
    """Run unit tests."""
    run("go test ./pkg/...")

@task(help={
    "env": "Specify in which environment to run the linter . Default 'container'. Supported: 'container','host'"
})
def lint(ctx, env="container"):
    """Run linter.

    By default, this will run a golangci-lint docker image against the code.
    However, in some environments (such as the MetalLB CI), it may be more
    convenient to install the golangci-lint binaries on the host. This can be
    achieved by running `inv lint --env host`.
    """
    version = "1.55.2"
    golangci_cmd = "golangci-lint run --timeout 10m0s ./..."

    if env == "container":
        run("docker run --rm -v $(git rev-parse --show-toplevel):/app -w /app golangci/golangci-lint:v{} {}".format(version, golangci_cmd), echo=True)
    elif env == "host":
        run("curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v{}".format(version))
        run(golangci_cmd)
    else:
        raise Exit(message="Unsupported linter environment: {}". format(env))
