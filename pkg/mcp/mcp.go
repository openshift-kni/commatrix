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
// If multiple MCPs match, we choose deterministically by sorted name order.
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
		bestName := ""
		bestScore := -1
		for _, pool := range mcps.Items {
			if pool.Status.MachineCount == 0 {
				continue
			}
			selector, err := metav1.LabelSelectorAsSelector(pool.Spec.NodeSelector)
			if err != nil || selector == nil {
				continue
			}
			if selector.Matches(labels.Set(node.Labels)) {
				score := 0
				if pool.Spec.NodeSelector != nil {
					score += len(pool.Spec.NodeSelector.MatchLabels)
					score += len(pool.Spec.NodeSelector.MatchExpressions)
				}
				if score > bestScore || (score == bestScore && pool.Name > bestName) {
					bestName = pool.Name
					bestScore = score
				}
			}
		}
		if bestName == "" {
			return nil, fmt.Errorf("no MachineConfigPool matched node %s", node.Name)
		}
		nodeToPool[node.Name] = bestName
	}

	return nodeToPool, nil
}

// MapPoolToRoles  maps each MachineConfigPool to the roles present on its nodes.
// Only "master" and "worker" roles are considered.
func MapPoolToRoles(cs *client.ClientSet, nodeToPool map[string]string) (map[string][]string, error) {
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
