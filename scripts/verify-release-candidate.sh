#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root_dir="$(cd "$script_dir/.." && pwd)"
# shellcheck disable=SC1091
source "$script_dir/release-images.env"

directory=${1:-}
if [ -z "$directory" ] || [ ! -d "$directory" ]; then
  echo "usage: $0 RELEASE_DIRECTORY" >&2
  exit 2
fi
directory=$(cd "$directory" && pwd)

for command_name in docker jq; do
  command -v "$command_name" >/dev/null 2>&1 || {
    echo "$command_name is required to verify a release candidate" >&2
    exit 1
  }
done

container_user="$(id -u):$(id -g)"
scratch_directory=$(mktemp -d)
loaded_image_refs=()
cleanup() {
  for image_ref in "${loaded_image_refs[@]}"; do
    docker image rm "$image_ref" >/dev/null 2>&1 || true
  done
  rm -rf "$scratch_directory"
}
trap cleanup EXIT HUP INT TERM
docker run --rm --user "$container_user" \
  -e HOME=/tmp -e GOCACHE=/tmp/go-build -e GOMODCACHE=/tmp/go-mod \
  -v "$root_dir:/src:ro" -v "$scratch_directory:/scratch" -w /src "$GO_IMAGE" \
  sh -ceu 'CGO_ENABLED=0 go build -trimpath -buildvcs=false -o /scratch/ipchronicle-release-tool ./cmd/ipchronicle-release-tool'
release_tool="$scratch_directory/ipchronicle-release-tool"
summary=$("$release_tool" verify --directory "$directory")
version=$(jq -er '.version' <<<"$summary")
revision=$(jq -er '.revision' <<<"$summary")

jq -e --arg version "$version" --arg revision "$revision" '
  (keys == ["agent", "builder", "centerImage", "revision", "schemaVersion", "sourceDateEpoch", "sourceUrl", "tag", "toolchainImages", "version"]) and
  .schemaVersion == 1 and
  .version == $version and .tag == ("v" + $version) and .revision == $revision and
  .sourceUrl == ("https://github.com/ipchronicle/ipchronicle/tree/v" + $version) and
  .sourceDateEpoch >= 1 and
  .centerImage == ("ghcr.io/ipchronicle/ipchronicle-center:v" + $version) and
  (.agent | keys == ["architectures", "cgoEnabled", "os"]) and
  .agent == {os: "linux", architectures: ["amd64", "arm64"], cgoEnabled: false} and
  (.builder | keys == ["buildxVersion", "dockerServerVersion"]) and
  ([.builder.buildxVersion, .builder.dockerServerVersion] | all(type == "string" and length > 0)) and
  (.toolchainImages | keys == ["dockerfileFrontend", "go", "node", "runtime", "syft"]) and
  ([.toolchainImages.dockerfileFrontend, .toolchainImages.go, .toolchainImages.node,
    .toolchainImages.runtime, .toolchainImages.syft] |
    all(type == "string" and contains("@sha256:")))
' "$directory/build-metadata.json" >/dev/null

compose_json=$(IPCHRONICLE_ADMIN_USERNAME=admin IPCHRONICLE_ADMIN_PASSWORD=admin \
  docker compose --env-file "$directory/.env.example" -f "$directory/compose.yaml" config --format json)
jq -e --arg image "ghcr.io/ipchronicle/ipchronicle-center:v$version" '
  .services.center.image == $image and
  .services.center.read_only == true and
  .services.center.user == "10001:10001" and
  .services.center.cap_drop == ["ALL"] and
  .services.center.security_opt == ["no-new-privileges:true"]
' <<<"$compose_json" >/dev/null

for architecture in amd64 arm64; do
  "$release_tool" verify-agent \
    --path "$directory/ipchronicle-agent-linux-$architecture" --arch "$architecture" >/dev/null

  "$release_tool" verify-oci \
      --path "$directory/ipchronicle-center-linux-$architecture.oci.tar" \
      --arch "$architecture" --version "$version" --revision "$revision" >/dev/null

  image_ref="ghcr.io/ipchronicle/ipchronicle-center:v$version"
  if docker image inspect "$image_ref" >/dev/null 2>&1; then
    echo "refusing to replace existing local image $image_ref during verification" >&2
    exit 1
  fi
  docker_archive="$scratch_directory/ipchronicle-center-linux-$architecture.docker.tar"
  docker run --rm --user "$container_user" -e HOME=/tmp \
    -v "$directory:/release:ro" -v "$scratch_directory:/scratch" \
    "$SKOPEO_IMAGE" copy --insecure-policy \
    "oci-archive:/release/ipchronicle-center-linux-$architecture.oci.tar" \
    "docker-archive:/scratch/ipchronicle-center-linux-$architecture.docker.tar:$image_ref" >/dev/null
  docker load --input "$docker_archive" >/dev/null
  rm -f "$docker_archive"
  loaded_image_refs+=("$image_ref")
  center_metadata=$(docker run --rm --platform "linux/$architecture" \
    --network none --read-only --cap-drop ALL --security-opt no-new-privileges \
    --user 10001:10001 --memory 64m --cpus 1 --pids-limit 64 \
    "$image_ref" version --json)
  jq -e --arg version "$version" --arg revision "$revision" --arg arch "$architecture" '
    .version == $version and .revision == $revision and .component == "center" and
    .os == "linux" and .arch == $arch
  ' <<<"$center_metadata" >/dev/null
  docker image rm "$image_ref" >/dev/null

  for runtime_name in alpine debian; do
    case "$runtime_name" in
      alpine) runtime_image=$ALPINE_IMAGE ;;
      debian) runtime_image=$DEBIAN_IMAGE ;;
    esac
    runtime_name_with_tag=${runtime_image%@*}
    runtime_source="${runtime_name_with_tag%:*}@${runtime_image##*@}"
    runtime_ref="ipchronicle-verify-$runtime_name-$architecture:v$version"
    if docker image inspect "$runtime_ref" >/dev/null 2>&1; then
      echo "refusing to replace existing local image $runtime_ref during verification" >&2
      exit 1
    fi
    runtime_archive="$scratch_directory/runtime-$runtime_name-$architecture.docker.tar"
    docker run --rm --user "$container_user" -e HOME=/tmp \
      -v "$scratch_directory:/scratch" "$SKOPEO_IMAGE" copy --insecure-policy \
      --override-os linux --override-arch "$architecture" \
      "docker://$runtime_source" \
      "docker-archive:/scratch/runtime-$runtime_name-$architecture.docker.tar:$runtime_ref" >/dev/null
    docker load --input "$runtime_archive" >/dev/null
    rm -f "$runtime_archive"
    loaded_image_refs+=("$runtime_ref")
    metadata=$(docker run --rm --platform "linux/$architecture" \
      --network none --read-only --cap-drop ALL --security-opt no-new-privileges \
      --user 65534:65534 --memory 64m --cpus 1 --pids-limit 64 \
      -v "$directory:/release:ro" --entrypoint "/release/ipchronicle-agent-linux-$architecture" \
      "$runtime_ref" version --json)
    jq -e --arg version "$version" --arg revision "$revision" --arg arch "$architecture" '
      .version == $version and .revision == $revision and .component == "agent" and
      .os == "linux" and .arch == $arch and .stateSchemaVersion >= 1 and
      (.capabilities | index("agent-update-v1") != null)
    ' <<<"$metadata" >/dev/null
    docker image rm "$runtime_ref" >/dev/null
  done

  jq -e --arg name "ipchronicle-agent-linux-$architecture" '
    .bomFormat == "CycloneDX" and .specVersion == "1.6" and .version == 1 and
    .metadata.component.name == $name and (.components | type == "array")
  ' "$directory/ipchronicle-agent-linux-$architecture.cdx.json" >/dev/null
  jq -e --arg name "ipchronicle-center-linux-$architecture" '
    .bomFormat == "CycloneDX" and .specVersion == "1.6" and .version == 1 and
    .metadata.component.name == $name and (.components | type == "array")
  ' "$directory/ipchronicle-center-linux-$architecture.cdx.json" >/dev/null
done

printf 'Verified IPChronicle v%s candidate at revision %s (%s artifacts).\n' \
  "$version" "$revision" "$(jq -er '.artifactCount' <<<"$summary")"
