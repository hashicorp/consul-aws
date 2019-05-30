#!/bin/bash

# Description:
#   This script will publish consul-aws containers to Dockerhub. It should only run on tagged
#   branches within CI and all the variables needed are populated either by CircleCI or the Makefile.

#   To publish a new container, make sure the following environment variables are set:
#     * CIRCLE_TAG - the version of the consul-aws binary you want to build an image for
#     * DOCKER_ORG - to the organization of the docker image
#     * DOCKER_IMAGE_NAME - to the name of the docker image

function get_latest_docker_version {
   # Arguments:
   #   $1 - Docker Org
   #   $2 - Docker Image Name
   #
   #
   # Returns:
   #   0 - success (version in the 'latest' container echoed)
   #   1 - 'latest' tag does not exist or label could not be found

   docker_latest=$(docker inspect --format='{{ index .Config.Labels "consul-aws.version" }}' "$1"/"$2":latest 2> /dev/null)

   if [ -z "$docker_latest" ]; then
      return 1
   else
      echo "$docker_latest"
      return 0
   fi
}

function get_latest_docker_minor_version {
   # Arguments:
   #   $1 - Docker Org
   #   $2 - Docker Image Name
   #   $3 - Minor Version Tag
   #
   # Returns:
   #   0 - success (version in the latest minor version container echoed)
   #   1 - tag does not exist or label could not be found
   docker_latest_minor=$(docker inspect --format='{{ index .Config.Labels "consul-aws.version" }}' "$1"/"$2":"$3" 2> /dev/null)

   if [ -z "$docker_latest_minor" ]; then
      return 1
   else
      echo "$docker_latest_minor"
      return 0
   fi
}

function higher_version {
   # Arguments:
   #   $1 - first version to compare
   #   $2 - second version to compare
   #
   # Returns:
   #   higher version of two arguments

   higher_version=$(echo -e "$1\n$2" | sort -rV | head -n 1)
   echo "$higher_version"
}
function main() {
   # check for necessary variables
   : "${CIRCLE_TAG?"Need to set CIRCLE_TAG"}"
   : "${DOCKER_ORG?"Need to set DOCKER_ORG"}"
   : "${DOCKER_IMAGE_NAME?"Need to set DOCKER_IMAGE_NAME"}"

   # trims v from version, ex: v1.2.3 -> 1.2.3; this maps to releases.hashicorp.com
   CURRENT_TAG_VERSION=$(echo "$CIRCLE_TAG" | sed 's/v\(.*\)/\1/')

   # trims the patch part of the git tag to compare
   MINOR_VERSION=${CIRCLE_TAG%.[0-9]*}
   DOCKER_MINOR_TAG="${MINOR_VERSION#v}-latest"

   LATEST_DOCKER_MINOR_VERSION=$(get_latest_docker_minor_version "$DOCKER_ORG" "$DOCKER_IMAGE_NAME" "$DOCKER_MINOR_TAG")
   LATEST_DOCKER_VERSION=$(get_latest_docker_version "$DOCKER_ORG" "$DOCKER_IMAGE_NAME")

   # Login to Dockerhub
   docker login -u "$DOCKER_USER" -p "$DOCKER_PASS"

   # build current branch tag image
   docker build -t "$DOCKER_ORG"/"$DOCKER_IMAGE_NAME":"$CURRENT_TAG_VERSION" --build-arg NAME="$DOCKER_IMAGE_NAME" --build-arg VERSION="$CURRENT_TAG_VERSION" -f "$(pwd)"/build-support/docker/Release.dockerfile "$(pwd)"/build-support/docker
   docker push "$DOCKER_ORG"/"$DOCKER_IMAGE_NAME":"$CURRENT_TAG_VERSION"

   # check to see if the current tag is higher than the latest minor version on dockerhub
   HIGHER_MINOR_VERSION=$(higher_version "$CURRENT_TAG_VERSION" "$LATEST_DOCKER_MINOR_VERSION")

   # if the higher version is the current tag, we tag the current image with the minor tag
   if [ "$HIGHER_MINOR_VERSION" = "$CURRENT_TAG_VERSION" ]; then
      echo "Tagging a new minor latest image"
      docker tag "$DOCKER_ORG"/"$DOCKER_IMAGE_NAME":"$CURRENT_TAG_VERSION" "$DOCKER_ORG"/"$DOCKER_IMAGE_NAME":"$DOCKER_MINOR_TAG"
      docker push "$DOCKER_ORG"/"$DOCKER_IMAGE_NAME":"$DOCKER_MINOR_TAG"
   fi

   # check to see if the current tag is higher than the latest version on dockerhub
   HIGHER_LATEST_VERSION=$(higher_version "$CURRENT_TAG_VERSION" "$LATEST_DOCKER_VERSION")

   # if:
   #   * we didn't find a version from the 'latest' image, it means it doesn't exist
   #   * or if the current tag version is higher than the latest docker one
   # we build latest
   if [ -z "$LATEST_DOCKER_VERSION" ] || [ "$HIGHER_LATEST_VERSION" = "$CURRENT_TAG_VERSION" ]; then
      echo "Tagging a new latest docker image"
      docker tag "$DOCKER_ORG"/"$DOCKER_IMAGE_NAME":"$CURRENT_TAG_VERSION" "$DOCKER_ORG"/"$DOCKER_IMAGE_NAME":latest
      docker push "$DOCKER_ORG"/"$DOCKER_IMAGE_NAME":latest
   fi

   return 0
}
main "$@"
exit $?