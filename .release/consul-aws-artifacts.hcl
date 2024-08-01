# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: BUSL-1.1

schema = 1
artifacts {
  zip = [
    "consul-aws_${version}_darwin_amd64.zip",
    "consul-aws_${version}_darwin_arm64.zip",
    "consul-aws_${version}_freebsd_386.zip",
    "consul-aws_${version}_freebsd_amd64.zip",
    "consul-aws_${version}_freebsd_arm.zip",
    "consul-aws_${version}_linux_386.zip",
    "consul-aws_${version}_linux_amd64.zip",
    "consul-aws_${version}_linux_arm.zip",
    "consul-aws_${version}_linux_arm64.zip",
    "consul-aws_${version}_netbsd_386.zip",
    "consul-aws_${version}_netbsd_amd64.zip",
    "consul-aws_${version}_netbsd_arm.zip",
    "consul-aws_${version}_openbsd_386.zip",
    "consul-aws_${version}_openbsd_amd64.zip",
    "consul-aws_${version}_openbsd_arm.zip",
    "consul-aws_${version}_solaris_amd64.zip",
    "consul-aws_${version}_windows_386.zip",
    "consul-aws_${version}_windows_amd64.zip",
  ]
  container = [
    "consul-aws_release-default_linux_amd64_${version}_${commit_sha}.docker.dev.tar",
    "consul-aws_release-default_linux_amd64_${version}_${commit_sha}.docker.tar",
    "consul-aws_release-default_linux_arm64_${version}_${commit_sha}.docker.dev.tar",
    "consul-aws_release-default_linux_arm64_${version}_${commit_sha}.docker.tar",
  ]
}
