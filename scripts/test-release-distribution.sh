#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
release_directory=${1:-}
distribution=${2:-}
architecture=${3:-}
if [[ -z $release_directory || -z $distribution || ! -d $release_directory ]]; then
  echo "usage: $0 RELEASE_DIRECTORY DISTRIBUTION amd64|arm64" >&2
  exit 2
fi
case "$architecture" in
  amd64|arm64) ;;
  *) echo "usage: $0 RELEASE_DIRECTORY DISTRIBUTION amd64|arm64" >&2; exit 2 ;;
esac

case "$distribution" in
  debian-12) image="debian:12-slim@sha256:abd67ffcfa541b485a3dff59865ab629aa048a6c613e639d36e7456b0b229241"; expected_id=debian; version_prefix=12; init_system=systemd ;;
  debian-13) image="debian:13-slim@sha256:3a39a0592364683e6bab97937b72cad5a8fa6dcbbee90edb3bb48c7f8e94f258"; expected_id=debian; version_prefix=13; init_system=systemd ;;
  ubuntu-24.04) image="ubuntu:24.04@sha256:561618e2c15bf2397621dd04f96926663a3b5616c189cf7e38db7e82f5c538ea"; expected_id=ubuntu; version_prefix=24.04; init_system=systemd ;;
  ubuntu-26.04) image="ubuntu:26.04@sha256:678c6550cc43645e08669028bc177f50be4e7c5b8cca677067b1914d4afc7a03"; expected_id=ubuntu; version_prefix=26.04; init_system=systemd ;;
  rhel-8) image="registry.access.redhat.com/ubi8/ubi:8.10@sha256:efe7eaa64e1efb79c34d0e9ec4ac6d0a95f512dfa8673c2d0d7dfa78a6787efe"; expected_id=rhel; version_prefix=8; init_system=systemd ;;
  rhel-9) image="registry.access.redhat.com/ubi9/ubi:9.7@sha256:e9a31af6530caffa3551f266c51a0d43b602e8f76a0dc12826dbeebceb487c92"; expected_id=rhel; version_prefix=9; init_system=systemd ;;
  rhel-10)
    case "$architecture" in
      amd64) image="registry.access.redhat.com/ubi10/ubi@sha256:4e0371a552f573a15dbe094b801e1f4a055aec7782fe17dd55f80f94d1db65e9" ;;
      arm64) image="registry.access.redhat.com/ubi10/ubi@sha256:dbfb24a0facb75c7b9e942213b5ca339adfe3b2b08d129fcad96870ca8e41b87" ;;
    esac
    expected_id=rhel; version_prefix=10; init_system=systemd
    ;;
  rocky-8) image="rockylinux/rockylinux:8@sha256:e8a49c5403b687db05d4d67333fa45808fbe74f36e683cec7abb1f7d0f2338c6"; expected_id=rocky; version_prefix=8; init_system=systemd ;;
  rocky-9) image="rockylinux/rockylinux:9@sha256:8101994123cf3d0a8fee517bee7f39e555c7d92bd2d9eb3303cc988a0eeed00f"; expected_id=rocky; version_prefix=9; init_system=systemd ;;
  rocky-10) image="rockylinux/rockylinux:10@sha256:827d37bc128288ccf160ee318bb3cb92d591164cb217e92f8bc61e3982ae1834"; expected_id=rocky; version_prefix=10; init_system=systemd ;;
  almalinux-8) image="almalinux:8@sha256:4a87d2615a770506e204c27d6248ac97f4df67f4e41e2e9c47c81f0ed0be98cb"; expected_id=almalinux; version_prefix=8; init_system=systemd ;;
  almalinux-9) image="almalinux:9@sha256:d2515c769e7b73f95c4fde38c0a505336ff38f14990c0b7253b77060a049a743"; expected_id=almalinux; version_prefix=9; init_system=systemd ;;
  almalinux-10) image="almalinux:10@sha256:cc24bc5b6ac7e284f2f62a07bdaa1b15d3319fdcf46413c6b8fe9fa245068ddd"; expected_id=almalinux; version_prefix=10; init_system=systemd ;;
  centos-stream-9) image="quay.io/centos/centos:stream9@sha256:d323b7623e947245a8eb506fbb0ad0e55eb2ae2d2407b66741a15f372caf9bdc"; expected_id=centos; version_prefix=9; init_system=systemd ;;
  centos-stream-10) image="quay.io/centos/centos:stream10@sha256:301bc4e6d5af2b6e707bec46aa71c21dc91ebc97422373cca37ea98d0e1aed63"; expected_id=centos; version_prefix=10; init_system=systemd ;;
  alpine-3.23) image="alpine:3.23@sha256:fd791d74b68913cbb027c6546007b3f0d3bc45125f797758156952bc2d6daf40"; expected_id=alpine; version_prefix=3.23; init_system=openrc ;;
  alpine-3.24) image="alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b"; expected_id=alpine; version_prefix=3.24; init_system=openrc ;;
  *) echo "unknown release distribution: $distribution" >&2; exit 2 ;;
esac

release_directory=$(cd "$release_directory" && pwd)
version=$(jq -er '.version' "$release_directory/release-manifest.json")
revision=$(jq -er '.revision' "$release_directory/release-manifest.json")
test -x "$release_directory/ipchronicle-agent-linux-$architecture"

if ! docker image inspect --platform "linux/$architecture" "$image" >/dev/null 2>&1; then
  docker pull --platform "linux/$architecture" "$image" >/dev/null
fi
resolved_image=$(docker image inspect "$image" --format '{{index .RepoDigests 0}}')
printf 'Testing %s on %s (%s)\n' "$distribution" "$resolved_image" "$architecture"
docker run --rm --platform "linux/$architecture" \
  --memory 768m --cpus 2 --pids-limit 512 \
  -e EXPECTED_ARCH="$architecture" \
  -e EXPECTED_ID="$expected_id" \
  -e EXPECTED_INIT="$init_system" \
  -e EXPECTED_VERSION_PREFIX="$version_prefix" \
  -e RELEASE_REVISION="$revision" \
  -e RELEASE_VERSION="$version" \
  -v "$release_directory:/release:ro" \
  -v "$script_dir/test-release-distribution-inner.sh:/test-release-distribution-inner.sh:ro" \
  --entrypoint /bin/sh \
  "$image" /test-release-distribution-inner.sh
