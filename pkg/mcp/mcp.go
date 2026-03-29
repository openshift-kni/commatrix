package mcp

import (
	"fmt"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/openshift-kni/commatrix/pkg/consts"
)

// ResolveNodeToPool builds a mapping from node name to its MachineConfigPool.
// It derives the pool from the node annotation "machineconfiguration.openshift.io/currentConfig",
// expected in the form: "rendered-<pool>-<hash>". The pool name is obtained by removing the
// "rendered-" prefix and trimming the trailing "-<hash>".
// Returns an error if the annotation is missing or malformed.
func ResolveNodeToPool(nodes []corev1.Node) (map[string]string, error) {
	nodeToPool := make(map[string]string, len(nodes))
	for _, node := range nodes {
		current, exist := node.GetAnnotations()["machineconfiguration.openshift.io/currentConfig"]
		if !exist {
			return nil, fmt.Errorf("node %s missing annotation machineconfiguration.openshift.io/currentConfig", node.Name)
		}
		pool, ok := poolNameFromRenderedConfig(current)
		if !ok || pool == "" {
			return nil, fmt.Errorf("node %s has malformed currentConfig %q", node.Name, current)
		}
		nodeToPool[node.Name] = pool
	}

	return nodeToPool, nil
}

func poolNameFromRenderedConfig(currentConfig string) (string, bool) {
	if !strings.HasPrefix(currentConfig, "rendered-") {
		return "", false
	}
	trimmed := strings.TrimPrefix(currentConfig, "rendered-")
	lastDash := strings.LastIndex(trimmed, "-")
	if lastDash <= 0 {
		return "", false
	}
	return trimmed[:lastDash], true
}

// GetPoolRolesForStaticEntriesExpansion derives, per pool, which of [master, worker]
// Are present on its nodes; used to expand role-scoped static entries across pools.
func GetPoolRolesForStaticEntriesExpansion(nodes []corev1.Node, nodeToPool map[string]string) (map[string][]string, error) {
	observedRoles := make(map[string][]string)
	for _, node := range nodes {
		_, hasmaster := node.Labels[consts.RoleLabel+"master"]
		_, hascontrolplane := node.Labels[consts.RoleLabel+"control-plane"]
		_, hasworker := node.Labels[consts.RoleLabel+"worker"]
		pool := nodeToPool[node.Name]
		if (hasmaster || hascontrolplane) && !slices.Contains(observedRoles[pool], "master") {
			observedRoles[pool] = append(observedRoles[pool], "master")
		}
		if hasworker && !slices.Contains(observedRoles[pool], "worker") {
			observedRoles[pool] = append(observedRoles[pool], "worker")
		}
	}

	return observedRoles, nil
}
