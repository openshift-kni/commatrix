package endpointslices

import (
	"slices"

	corev1 "k8s.io/api/core/v1"
)

// isStaticPod returns true if the pod is a static (mirror) pod.
// Static pods are managed directly by the kubelet from manifest files on disk,
// not by the kube-scheduler. They are identified by having a Node as their
// owner reference. Since static pods cannot be rescheduled to other nodes,
// the point-in-time EndpointSlice snapshot is correct for them and scheduling
// expansion should be skipped.
func isStaticPod(pod corev1.Pod) bool {
	for _, ref := range pod.OwnerReferences {
		if ref.Kind == "Node" {
			return true
		}
	}
	return false
}

// getEligiblePoolsForPod returns all pools where the pod could potentially be
// scheduled, based on its nodeSelector and required nodeAffinity constraints.
// Unlike getEndpointSliceGroups which only returns pools where endpoints
// currently exist, this function returns all pools that have at least one
// node matching the pod's scheduling constraints. This ensures that firewall
// rules are generated for every pool a workload could land on after
// rescheduling (e.g., after a node reboot).
func getEligiblePoolsForPod(pod corev1.Pod, nodes []corev1.Node, nodeToGroup map[string]string) []string {
	poolsMap := make(map[string]bool)
	for i := range nodes {
		if !nodeMatchesPodConstraints(pod, &nodes[i]) {
			continue
		}
		pool := nodeToGroup[nodes[i].Name]
		if pool != "" {
			poolsMap[pool] = true
		}
	}

	pools := make([]string, 0, len(poolsMap))
	for k := range poolsMap {
		pools = append(pools, k)
	}
	slices.Sort(pools)
	return pools
}

// nodeMatchesPodConstraints checks whether a node satisfies the pod's
// nodeSelector and required nodeAffinity. Tolerations/taints are intentionally
// not checked: being slightly over-permissive (opening a port on a node where
// a taint would prevent scheduling) is benign, while being under-permissive
// (missing a firewall rule) causes outages.
func nodeMatchesPodConstraints(pod corev1.Pod, node *corev1.Node) bool {
	for key, val := range pod.Spec.NodeSelector {
		nodeVal, ok := node.Labels[key]
		if !ok || nodeVal != val {
			return false
		}
	}

	if pod.Spec.Affinity == nil ||
		pod.Spec.Affinity.NodeAffinity == nil ||
		pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		return true
	}

	return nodeMatchesNodeSelector(
		pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution,
		node.Labels,
	)
}

// nodeMatchesNodeSelector evaluates a corev1.NodeSelector against a node's
// labels. NodeSelectorTerms are ORed: a node matches if ANY term matches.
func nodeMatchesNodeSelector(sel *corev1.NodeSelector, nodeLabels map[string]string) bool {
	for _, term := range sel.NodeSelectorTerms {
		if nodeMatchesNodeSelectorTerm(term, nodeLabels) {
			return true
		}
	}
	return false
}

// nodeMatchesNodeSelectorTerm checks a single NodeSelectorTerm.
// MatchExpressions are ANDed: all must match for the term to match.
// MatchFields are ignored (they match on node fields, not labels).
func nodeMatchesNodeSelectorTerm(term corev1.NodeSelectorTerm, nodeLabels map[string]string) bool {
	for _, expr := range term.MatchExpressions {
		if !matchNodeSelectorExpression(expr, nodeLabels) {
			return false
		}
	}
	return true
}

func matchNodeSelectorExpression(expr corev1.NodeSelectorRequirement, nodeLabels map[string]string) bool {
	val, exists := nodeLabels[expr.Key]
	switch expr.Operator {
	case corev1.NodeSelectorOpIn:
		return exists && slices.Contains(expr.Values, val)
	case corev1.NodeSelectorOpNotIn:
		return !exists || !slices.Contains(expr.Values, val)
	case corev1.NodeSelectorOpExists:
		return exists
	case corev1.NodeSelectorOpDoesNotExist:
		return !exists
	default:
		// Gt, Lt are rare; be permissive to avoid blocking traffic.
		return true
	}
}
