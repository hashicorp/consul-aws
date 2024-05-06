# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# This Dockerfile contains multiple targets.
# Use 'docker build --target=<name> .' to build one.
#
# Every target has a BIN_NAME argument that must be provided via --build-arg=BIN_NAME=<name>
# when building.

# Pull in dumb-init from alpine, as our distroless release image doesn't have a
# package manager and there's no RPM package for UBI.
FROM alpine:latest AS dumb-init
RUN apk add dumb-init

# release-default release image
# -----------------------------------
FROM gcr.io/distroless/base-debian11 AS release-default

ARG BIN_NAME=consul-aws
ENV BIN_NAME=$BIN_NAME
# PRODUCT_* variables are set by the hashicorp/actions-build-docker action.
ARG PRODUCT_VERSION
ARG PRODUCT_REVISION
ARG PRODUCT_NAME=$BIN_NAME
ENV PRODUCT_NAME=$PRODUCT_NAME

# TARGETARCH and TARGETOS are set automatically when --platform is provided.
ARG TARGETOS
ARG TARGETARCH

LABEL name=${BIN_NAME}\
      maintainer="Consul Team <consul@hashicorp.com>" \
      vendor="HashiCorp" \
      version=${PRODUCT_VERSION} \
      release=${PRODUCT_REVISION} \
      revision=${PRODUCT_REVISION} \
      summary="Consul AWS is a tool for bi-directional sync between AWS CloudMap and Consul." \
      licenses="MPL-2.0" \
      description="Consul AWS is a tool for bi-directional sync between AWS CloudMap and Consul."

COPY --from=dumb-init /usr/bin/dumb-init /usr/local/bin/
COPY dist/$TARGETOS/$TARGETARCH/$BIN_NAME /usr/local/bin/
COPY LICENSE /licenses/copyright.txt
COPY LICENSE /usr/share/doc/$PRODUCT_NAME/LICENSE.txt

USER 100

ENTRYPOINT ["/usr/local/bin/dumb-init", "/usr/local/bin/consul-aws"]

# Red Hat UBI-based image
# This image is based on the Red Hat UBI base image, and has the necessary
# labels, license file, and non-root user.
# -----------------------------------
FROM registry.access.redhat.com/ubi9-minimal:9.3 as release-ubi

ARG BIN_NAME=consul-aws
ENV BIN_NAME=$BIN_NAME
# PRODUCT_* variables are set by the hashicorp/actions-build-docker action.
ARG PRODUCT_VERSION
ARG PRODUCT_REVISION
ARG PRODUCT_NAME=$BIN_NAME
ENV PRODUCT_NAME=$PRODUCT_NAME
# TARGETARCH and TARGETOS are set automatically when --platform is provided.
ARG TARGETOS
ARG TARGETARCH

LABEL name=${BIN_NAME}\
      maintainer="Consul Team <consul@hashicorp.com>" \
      vendor="HashiCorp" \
      version=${PRODUCT_VERSION} \
      release=${PRODUCT_REVISION} \
      revision=${PRODUCT_REVISION} \
      summary="Consul AWS is a tool for bi-directional sync between AWS CloudMap and Consul." \
      licenses="MPL-2.0" \
      description="Consul AWS is a tool for bi-directional sync between AWS CloudMap and Consul."

RUN microdnf install -y shadow-utils

# Create a non-root user to run the software.
RUN groupadd --gid 1000 $PRODUCT_NAME && \
    adduser --uid 100 --system -g $PRODUCT_NAME $PRODUCT_NAME && \
    usermod -a -G root $PRODUCT_NAME

COPY --from=dumb-init /usr/bin/dumb-init /usr/local/bin/
COPY dist/$TARGETOS/$TARGETARCH/$BIN_NAME /usr/local/bin/
COPY LICENSE /licenses/copyright.txt
COPY LICENSE /usr/share/doc/$PRODUCT_NAME/LICENSE.txt

USER 100
ENTRYPOINT ["/usr/local/bin/dumb-init", "/usr/local/bin/consul-aws"]

# ===================================
#
#   Set default target to 'release-default'.
#
# ===================================
FROM release-default
