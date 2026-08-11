#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root_dir="$(cd "$script_dir/.." && pwd)"
# shellcheck disable=SC1091
source "$script_dir/release-images.env"

command -v docker >/dev/null 2>&1 || {
  echo "docker is required for the release failure gate" >&2
  exit 1
}

test_pattern='TestProbeUploaderSurvivesCenterOutageAndRetransmitsWithoutExecuting|TestAddressTransitionsAndBoundedGapSurviveRestart|TestProbeRunReconcilesWithoutRerunningAfterRestart|TestProbeQueueEvictsOldestPerEgressAndKeepsGap|TestCheckerRejectsUnavailableSelector|TestManagerStagesValidatedAgentAndStartsSupervisor|TestSupervisorRollsBackAfterHealthTimeoutAndPreservesState|TestSupervisorRollsBackWhenNewAgentDoesNotStart|TestVersionFiveNetworkEgressesMigrateToCurrentVersion|TestVersionElevenProbeTasksMigrateToSharedAgentTaskSlot|TestDeletedHistoryAdvancesGeneration|TestProbeHistoryResetInvalidatesOldGenerationAndAdvancesConfiguration|TestNotificationQueueOverflowIsTerminal'
container_user="$(id -u):$(id -g)"
docker run --rm --user "$container_user" \
  -e HOME=/tmp -e GOCACHE=/tmp/go-build -e GOMODCACHE=/tmp/go-mod \
  -e TEST_PATTERN="$test_pattern" -v "$root_dir:/workspace" -w /workspace "$GO_IMAGE" \
  sh -ceu 'go test -count=1 -run "$TEST_PATTERN" \
    ./internal/agent ./internal/agent/observation ./internal/agent/state ./internal/agent/update \
    ./internal/center/database ./internal/center/nodes ./internal/center/notifications'

if [ "$(id -u)" -eq 0 ]; then
  "$script_dir/test-install-agent.sh"
elif command -v sudo >/dev/null 2>&1 && sudo -n true; then
  sudo -n "$script_dir/test-install-agent.sh"
else
  echo "root or passwordless sudo is required for the Agent installer lifecycle gate" >&2
  exit 1
fi
printf '%s\n' 'Release failure gate passed.'
