package node

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/onsi/gomega"

	"github.com/openshift-kni/commatrix/pkg/client"
	"github.com/openshift-kni/commatrix/pkg/consts"
	"github.com/openshift-kni/commatrix/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NodeWrapper struct {
	Node *v1.Node
	cs   *client.ClientSet
}

func New(node *v1.Node, cs *client.ClientSet) *NodeWrapper {
	return &NodeWrapper{
		Node: node,
		cs:   cs,
	}
}

// SoftRebootNodeAndWaitForDisconnect soft reboots given node and wait for node to be unreachable.
func (node *NodeWrapper) SoftRebootNodeAndWaitForDisconnect(ns string) error {
	utilsHelpers := utils.New(node.cs)

	debugPod, err := utilsHelpers.CreatePodOnNode(node.Node.Name, ns, consts.DefaultDebugPodImage)
	if err != nil {
		return fmt.Errorf("failed to create debug pod on node %s: %w", node.Node.Name, err)
	}

	defer func() {
		err := utilsHelpers.DeletePod(debugPod)
		if err != nil {
			fmt.Printf("failed cleaning debug pod %s: %v", debugPod, err)
		}
	}()

	rebootcmd := []string{"chroot", "/host", "reboot"}

	_, err = utilsHelpers.RunCommandOnPod(debugPod, rebootcmd)
	if err != nil {
		return fmt.Errorf("failed to install nftables on  debugpod on node%s: %w", node.Node.Name, err)
	}

	node.WaitForNodeNotReady()
	return nil
}

// WaitForNodeNotReady waits for the node to be in the NotReady state in Kubernetes.
func (node *NodeWrapper) WaitForNodeNotReady() {
	log.Printf("Waiting for node %s to be in NotReady state", node.Node.Name)

	timeout := 5 * time.Minute
	gomega.Eventually(func() bool {
		updatedNode, err := node.cs.Nodes().Get(context.TODO(), node.Node.Name, metav1.GetOptions{})
		if err != nil {
			log.Printf("Error getting node %s: %v", node.Node.Name, err)
			return false
		}

		for _, condition := range updatedNode.Status.Conditions {
			if condition.Type == v1.NodeReady && condition.Status != v1.ConditionTrue {
				return true
			}
		}
		return false
	}, timeout, 3*time.Second).Should(
		gomega.BeTrue(),
		fmt.Sprintf("Node %s is still ready after %s", node.Node.Name, timeout.String()),
	)

	log.Printf("Node %s is NotReady\n", node.Node.Name)
}

// WaitForNodeReady waits for the node to be in the Ready state in Kubernetes.
func (node *NodeWrapper) WaitForNodeReady() {
	log.Printf("Waiting for node %s to be in Ready state", node.Node.Name)

	timeout := 15 * time.Minute
	gomega.Eventually(func() bool {
		updatedNode, err := node.cs.Nodes().Get(context.TODO(), node.Node.Name, metav1.GetOptions{})
		if err != nil {
			log.Printf("Error getting node %s: %v", node.Node.Name, err)
			return false
		}

		for _, condition := range updatedNode.Status.Conditions {
			if condition.Type == v1.NodeReady && condition.Status == v1.ConditionTrue {
				return true
			}
		}
		return false
	}, timeout, 3*time.Second).Should(
		gomega.BeTrue(),
		fmt.Sprintf("Node %s is still not ready after %s", node.Node.Name, timeout.String()),
	)

	log.Printf("Node %s is Ready\n", node.Node.Name)
}
