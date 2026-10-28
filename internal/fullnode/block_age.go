package fullnode

import (
	"fmt"
	"time"

	cosmosv1 "github.com/bryanlabs/cosmos-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
)

// maxBlockAgeAnnotation opts a CosmosFullNode into upgrade-aware readiness. Its value is a Go duration such as "60s".
//
// A node halted at an upgrade height still reports catching_up=false, so the default healthcheck keeps it ready
// while it serves a frozen height. With this annotation set:
//   - the healthcheck sidecar reports not ready once the latest block is older than the duration
//   - pods halted at an upgrade height are replaced together instead of one at a time under maxUnavailable
//   - p2p Services publish not-ready addresses, so a network-wide halt does not drop peers
const maxBlockAgeAnnotation = "cosmos.bryanlabs.net/max-block-age"

// maxBlockAge returns the parsed annotation, or zero when it is absent, malformed, or not positive.
func maxBlockAge(crd *cosmosv1.CosmosFullNode) time.Duration {
	v, ok := crd.Annotations[maxBlockAgeAnnotation]
	if !ok {
		return 0
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0
	}
	return d
}

// healthCheckCmd is the healthcheck sidecar command, with the block age limit when the CRD opts in.
func healthCheckCmd(crd *cosmosv1.CosmosFullNode) []string {
	cmd := []string{"/manager", "healthcheck", "--rpc-host", fmt.Sprintf("http://localhost:%d", crd.Spec.ChainSpec.Comet.RPCPort())}
	if d := maxBlockAge(crd); d > 0 {
		cmd = append(cmd, "--max-block-age", d.String())
	}
	return cmd
}

// haltedForUpgrade reports whether the pod runs a chain image from an earlier entry in spec.chain.versions than its
// height calls for. Such a node has stopped at the upgrade height and can neither serve current state nor advance
// until it restarts on the new image, so holding it back under maxUnavailable only lengthens the outage.
func haltedForUpgrade(crd *cosmosv1.CosmosFullNode, pod *corev1.Pod) bool {
	versions := crd.Spec.ChainSpec.Versions
	if len(versions) == 0 || len(pod.Spec.Containers) == 0 {
		return false
	}
	if o, ok := crd.Spec.InstanceOverrides[pod.Name]; ok && o.Image != "" {
		return false
	}
	height, ok := crd.Status.Height[pod.Name]
	if !ok {
		return false
	}

	current := -1
	for i, v := range versions {
		if height < v.UpgradeHeight {
			break
		}
		current = i
	}
	if current <= 0 {
		return false
	}

	image := pod.Spec.Containers[0].Image
	if image == versions[current].Image {
		return false
	}
	for _, v := range versions[:current] {
		if v.Image == image {
			return true
		}
	}
	return false
}
