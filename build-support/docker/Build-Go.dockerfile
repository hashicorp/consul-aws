# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

ARG GOLANG_VERSION=1.14.1
FROM golang:${GOLANG_VERSION}

ARG GOTOOLS="github.com/magiconair/vendorfmt/cmd/vendorfmt \
   github.com/mitchellh/gox \
   golang.org/x/tools/cmd/cover \
   golang.org/x/tools/cmd/stringer \
   github.com/axw/gocov/gocov \
   gopkg.in/matm/v1/gocov-html"

RUN go get -u -v ${GOTOOLS}

WORKDIR /go-build
