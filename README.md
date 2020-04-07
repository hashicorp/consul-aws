# Consul-AWS

`consul-aws` syncs the services in an AWS CloudMap namespace to a Consul datacenter. Consul services will be created in AWS CloudMap and the other way around. This enables native service discovery across Consul and AWS CloudMap.

This project is versioned separately from Consul. Supported Consul versions for each feature will be noted below. By versioning this project separately, we can iterate on AWS integrations more quickly and release new versions without forcing Consul users to do a full Consul upgrade.

## Installation

1. Download a pre-compiled, released version from the [Consul-AWS releases page][releases].

1. Extract the binary using `unzip` or `tar`.

1. Move the binary into `$PATH`.

To compile from source, please see the instructions in the [contributing section](#contributing).

## Usage

`consul-aws` can sync from Consul to AWS CloudMap (`-to-aws`), from AWS CloudMap to Consul (`-to-consul`) and both at the same time. No matter which direction is being used `consul-aws` needs to be connected to Consul and AWS CloudMap.

In order to help with connecting to a Consul cluster, `consul-aws` provides all the flags you might need including the possibility to set an ACL token. `consul-aws` loads your AWS configuration from `.aws`, from the instance profile and ENV variables - it supports everything provided by the AWS golang sdk.

Apart from that a AWS CloudMap namespace id has to be provided. This is how `consul-aws` could be invoked to sync both directions:

```shell
$ ./consul-aws sync-catalog -aws-namespace-id ns-hjrgt3bapp7phzff -to-aws -to-consul
```

## Contributing

To build and install `consul-aws` locally, Go version 1.14+ is required.
You will also need to install the Docker engine:

- [Docker for Mac](https://docs.docker.com/engine/installation/mac/)
- [Docker for Windows](https://docs.docker.com/engine/installation/windows/)
- [Docker for Linux](https://docs.docker.com/engine/installation/linux/ubuntulinux/)

Clone the repository:

```shell
$ git clone https://github.com/hashicorp/consul-aws.git
```

To compile the `consul-aws` binary for your local machine:

```shell
$ make dev
```

This will compile the `consul-aws` binary into `bin/consul-aws` as well as your `$GOPATH` and run the test suite.

Or run the following to generate all binaries:

```shell
$ make dist
```

To create a docker image with your local changes:

```shell
$ make dev-docker
```
## Testing

If you just want to run the tests:

```shell
$ make test
```

Or to run a specific test in the suite:

```shell
go test ./... -run SomeTestFunction_name
```

**Note:** To run the sync integration tests, you must specify `INTTEST=1` in your environment and [AWS credentials](https://docs.aws.amazon.com/sdk-for-go/v1/developer-guide/configuring-sdk.html#specifying-credentials).

## Compatibility with Consul

`consul-aws` supports the current version of Consul and the version before. At the time of writing this, it means `1.7` and `1.6`.

[releases]: https://releases.hashicorp.com/consul-aws "Consul-AWS Releases"
