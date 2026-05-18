package endpointslices

import (
	"maps"
	"slices"

	corev1 "k8s.io/api/core/v1"
	nodeaffinity "k8s.io/component-helpers/scheduling/corev1/nodeaffinity"
)

// filterNonStaticPods returns only the scheduler-managed pods, filtering out
// static (mirror) pods. Static pods are managed directly by the kubelet and
// identified by having a Node as their owner reference. They cannot be
// rescheduled, so the EndpointSlice snapshot is authoritative for them and
// scheduling expansion should only apply to the non-static pods.
func filterNonStaticPods(pods []corev1.Pod) []corev1.Pod {
	var nonStatic []corev1.Pod
	for i := range pods {
		if !isStaticPod(&pods[i]) {
			nonStatic = append(nonStatic, pods[i])
		}
	}
	return nonStatic
}

func isStaticPod(pod *corev1.Pod) bool {
	for _, ref := range pod.OwnerReferences {
		if ref.Kind == "Node" {
			return true
		}
	}
	return false
}

// getEligiblePoolsForPods returns the union of all pools where any of the
// given pods could potentially be scheduled, based on nodeSelector and
// required nodeAffinity constraints. Checking every pod (rather than only
// the first) handles the case where a single Service selects pods from
// different controllers with different scheduling constraints.
//
// PreferredDuringSchedulingIgnoredDuringExecution is intentionally ignored:
// pods with only preferred affinity can still run on any node, so they are
// treated the same as pods without selectors.
// Tolerations/taints are also not checked: being slightly over-permissive
// (opening a port on a node where a taint would prevent scheduling) is
// benign, while being under-permissive (missing a firewall rule) causes
// outages.
func getEligiblePoolsForPods(pods []corev1.Pod, nodes []corev1.Node, nodeToGroup map[string]string) []string {
	poolsMap := make(map[string]struct{})
	for _, pod := range pods {
		req := nodeaffinity.GetRequiredNodeAffinity(&pod)
		for i := range nodes {
			if match, err := req.Match(&nodes[i]); err != nil || !match {
				continue
			}
			if pool := nodeToGroup[nodes[i].Name]; pool != "" {
				poolsMap[pool] = struct{}{}
			}
		}
	}

	return slices.Sorted(maps.Keys(poolsMap))
}
