#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root_dir="$(cd "$script_dir/.." && pwd)"
# shellcheck disable=SC1091
source "$script_dir/release-images.env"

version=${1:-}
semver_number='(0|[1-9][0-9]*)'
if [[ ! $version =~ ^${semver_number}\.${semver_number}\.${semver_number}(-rc\.${semver_number})?$ ]]; then
  echo "usage: $0 VERSION" >&2
  exit 2
fi

for command_name in docker git jq; do
  command -v "$command_name" >/dev/null 2>&1 || {
    echo "$command_name is required to build a release candidate" >&2
    exit 1
  }
done

cd "$root_dir"
revision=$(git rev-parse --verify HEAD)
if [[ ! $revision =~ ^[0-9a-f]{40}$ ]]; then
  echo "HEAD is not a full lowercase Git revision" >&2
  exit 1
fi
if [ "${IPCHRONICLE_ALLOW_DIRTY:-0}" != "1" ] && [ -n "$(git status --porcelain --untracked-files=all)" ]; then
  echo "release candidates must be built from a clean worktree" >&2
  exit 1
fi
source_date_epoch=$(git show -s --format=%ct "$revision")
if [[ ! $source_date_epoch =~ ^[0-9]+$ ]]; then
  echo "source commit timestamp is invalid" >&2
  exit 1
fi
docker_server_version=$(docker version --format '{{.Server.Version}}')
buildx_version=$(docker buildx version)

for pinned_image in "$GO_IMAGE" "$NODE_IMAGE" "$DEBIAN_IMAGE" "$DOCKERFILE_IMAGE"; do
  if ! grep -Fq "$pinned_image" "$root_dir/Dockerfile"; then
    echo "Dockerfile does not use expected pinned image $pinned_image" >&2
    exit 1
  fi
done

output_root=${IPCHRONICLE_RELEASE_OUTPUT_ROOT:-$root_dir/dist/release}
mkdir -p "$output_root"
output_root=$(cd "$output_root" && pwd)
final_directory="$output_root/$version"
if [ -e "$final_directory" ]; then
  echo "release output already exists: $final_directory" >&2
  exit 1
fi

staging_root=$(mktemp -d "$output_root/.candidate-$version.XXXXXX")
payload_directory="$staging_root/payload"
scratch_directory="$staging_root/scratch"
mkdir -p "$payload_directory" "$scratch_directory"
cleanup() {
  rm -rf "$staging_root"
}
trap cleanup EXIT HUP INT TERM

container_user="$(id -u):$(id -g)"
ldflags="-s -w -buildid= -extldflags=-Wl,--build-id=none -X github.com/ipchronicle/ipchronicle/internal/version.Value=$version -X github.com/ipchronicle/ipchronicle/internal/version.Revision=$revision"
docker run --rm --user "$container_user" \
  -e HOME=/tmp -e GOCACHE=/tmp/go-build -e GOMODCACHE=/tmp/go-mod \
  -e VERSION="$version" -e REVISION="$revision" -e LDFLAGS="$ldflags" \
  -v "$root_dir:/src:ro" -v "$payload_directory:/release" -v "$scratch_directory:/scratch" \
  -w /src "$GO_IMAGE" \
  sh -ceu '
    go mod download
    for arch in amd64 arm64; do
      CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -buildvcs=false \
        -ldflags "$LDFLAGS" -o "/release/ipchronicle-agent-linux-$arch" ./cmd/ipchronicle-agent
    done
    CGO_ENABLED=0 go build -trimpath -buildvcs=false -o /scratch/ipchronicle-release-tool \
      ./cmd/ipchronicle-release-tool
  '
chmod 0755 "$payload_directory"/ipchronicle-agent-linux-*

image_reference="ghcr.io/ipchronicle/ipchronicle-center:v$version"
source_url="https://github.com/ipchronicle/ipchronicle/tree/v$version"
for architecture in amd64 arm64; do
  docker buildx build \
    --platform "linux/$architecture" \
    --file "$root_dir/Dockerfile" \
    --build-arg "VERSION=$version" \
    --build-arg "REVISION=$revision" \
    --build-arg "SOURCE_DATE_EPOCH=$source_date_epoch" \
    --build-arg "SOURCE_URL=$source_url" \
    --tag "$image_reference" \
    --provenance=false \
    --sbom=false \
    --output "type=oci,dest=$payload_directory/ipchronicle-center-linux-$architecture.oci.tar,rewrite-timestamp=true" \
    "$root_dir"
done

cp "$root_dir/LICENSE" "$payload_directory/LICENSE"
cp "$root_dir/THIRD_PARTY_NOTICES.md" "$payload_directory/THIRD_PARTY_NOTICES.md"
cp "$root_dir/.env.example" "$payload_directory/.env.example"
cp "$root_dir/scripts/install-agent.sh" "$payload_directory/install-agent.sh"
chmod 0755 "$payload_directory/install-agent.sh"
sed "s|IPCHRONICLE_VERSION_PLACEHOLDER|v$version|g" \
  "$root_dir/deploy/compose.release.yaml" > "$payload_directory/compose.yaml"
if grep -q 'IPCHRONICLE_VERSION_PLACEHOLDER' "$payload_directory/compose.yaml"; then
  echo "release Compose template still contains its version placeholder" >&2
  exit 1
fi

jq -n -S \
  --arg version "$version" \
  --arg revision "$revision" \
  --arg sourceUrl "$source_url" \
  --arg image "$image_reference" \
  --argjson sourceDateEpoch "$source_date_epoch" \
  --arg goImage "$GO_IMAGE" \
  --arg nodeImage "$NODE_IMAGE" \
  --arg runtimeImage "$DEBIAN_IMAGE" \
  --arg dockerfileImage "$DOCKERFILE_IMAGE" \
  --arg syftImage "$SYFT_IMAGE" \
  --arg dockerServerVersion "$docker_server_version" \
  --arg buildxVersion "$buildx_version" \
  '{
    schemaVersion: 1,
    version: $version,
    tag: ("v" + $version),
    revision: $revision,
    sourceUrl: $sourceUrl,
    sourceDateEpoch: $sourceDateEpoch,
    centerImage: $image,
    agent: {os: "linux", architectures: ["amd64", "arm64"], cgoEnabled: false},
    builder: {dockerServerVersion: $dockerServerVersion, buildxVersion: $buildxVersion},
    toolchainImages: {
      go: $goImage,
      node: $nodeImage,
      runtime: $runtimeImage,
      dockerfileFrontend: $dockerfileImage,
      syft: $syftImage
    }
  }' > "$payload_directory/build-metadata.json"

generate_sbom() {
  local source=$1
  local output=$2
  local component_name=$3
  local raw="$scratch_directory/sbom.raw.json"
  docker run --rm -v "$payload_directory:/release:ro" "$SYFT_IMAGE" \
    scan "$source" -o cyclonedx-json@1.6 > "$raw"
  jq -S --arg name "$component_name" --arg version "$version" '
    del(.serialNumber, .metadata.timestamp) |
    .metadata.component.name = $name |
    .metadata.component.version = $version
  ' "$raw" > "$output"
  rm -f "$raw"
}

for architecture in amd64 arm64; do
  generate_sbom \
    "file:/release/ipchronicle-agent-linux-$architecture" \
    "$payload_directory/ipchronicle-agent-linux-$architecture.cdx.json" \
    "ipchronicle-agent-linux-$architecture"
  generate_sbom \
    "oci-archive:/release/ipchronicle-center-linux-$architecture.oci.tar" \
    "$payload_directory/ipchronicle-center-linux-$architecture.cdx.json" \
    "ipchronicle-center-linux-$architecture"
done

"$scratch_directory/ipchronicle-release-tool" create \
  --directory "$payload_directory" --version "$version" --revision "$revision"

mv "$payload_directory" "$final_directory"
trap - EXIT HUP INT TERM
rm -rf "$staging_root"
printf 'Release candidate created at %s\n' "$final_directory"
