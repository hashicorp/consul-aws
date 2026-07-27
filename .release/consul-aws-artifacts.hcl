# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

schema = 1
artifacts {
  container = [
    "consul-aws_release-default_linux_amd64_${version}_${commit_sha}.docker.dev.tar",
    "consul-aws_release-default_linux_amd64_${version}_${commit_sha}.docker.tar",
    "consul-aws_release-default_linux_arm64_${version}_${commit_sha}.docker.dev.tar",
    "consul-aws_release-default_linux_arm64_${version}_${commit_sha}.docker.tar",
  ]
}
