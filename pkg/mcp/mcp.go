package mcp

import (
	"context"
	"fmt"
	"slices"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/openshift-kni/commatrix/pkg/client"
	"github.com/openshift-kni/commatrix/pkg/consts"
	machineconfigv1 "github.com/openshift/api/machineconfiguration/v1"
	rtclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ResolveNodeToPool builds a mapping from node name to its MachineConfigPool.
// Selection follows OpenShift MCO precedence rules (https://access.redhat.com/solutions/7004527):
// 1) If >1 custom pool matches, error
// 2) If exactly 1 custom pool and master match, error
// 3) If exactly 1 custom pool matches, select it
// 4) Else if master matches, select master
// 5) Else if worker matches, select worker
// Returns an error if a node does not match any MCP.
func ResolveNodeToPool(cs *client.ClientSet) (map[string]string, error) {
	// List nodes
	nodes := &corev1.NodeList{}
	if err := cs.List(context.TODO(), nodes, &rtclient.ListOptions{}); err != nil {
		return nil, err
	}

	// List MCPs
	mcps := &machineconfigv1.MachineConfigPoolList{}
	if err := cs.List(context.TODO(), mcps, &rtclient.ListOptions{}); err != nil {
		return nil, err
	}

	nodeToPool := make(map[string]string, len(nodes.Items))
	for _, node := range nodes.Items {
		matchedMaster := false
		matchedWorker := false
		customMatches := make([]string, 0, 2)

		for _, pool := range mcps.Items {
			if pool.Status.MachineCount == 0 {
				continue
			}
			selector, err := metav1.LabelSelectorAsSelector(pool.Spec.NodeSelector)
			if err != nil || selector == nil {
				continue
			}
			if !selector.Matches(labels.Set(node.Labels)) {
				continue
			}

			// Classify matches into master, worker, or custom
			switch pool.Name {
			case "master":
				matchedMaster = true
			case "worker":
				matchedWorker = true
			default:
				customMatches = append(customMatches, pool.Name)
			}
		}

		// Apply precedence rules
		if len(customMatches) > 1 {
			return nil, fmt.Errorf("node %s matched multiple custom MachineConfigPools: %v", node.Name, customMatches)
		}
		if len(customMatches) == 1 {
			if matchedMaster {
				return nil, fmt.Errorf("node %s matched both custom pool %q and master pool", node.Name, customMatches[0])
			}
			nodeToPool[node.Name] = customMatches[0]
			continue
		}
		if matchedMaster {
			nodeToPool[node.Name] = "master"
			continue
		}
		if matchedWorker {
			nodeToPool[node.Name] = "worker"
			continue
		}

		return nil, fmt.Errorf("no MachineConfigPool matched node %s", node.Name)
	}

	return nodeToPool, nil
}

// GetPoolRolesForStaticEntriesExpansion derives, per pool, which of [master, worker]
// Are present on its nodes; used to expand role-scoped static entries across pools.
func GetPoolRolesForStaticEntriesExpansion(cs *client.ClientSet, nodeToPool map[string]string) (map[string][]string, error) {
	// List nodes to inspect their labels
	nodes := &corev1.NodeList{}
	if err := cs.List(context.TODO(), nodes, &rtclient.ListOptions{}); err != nil {
		return nil, err
	}

	observedRoles := make(map[string][]string)
	for _, node := range nodes.Items {
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
