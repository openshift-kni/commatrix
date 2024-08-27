package node

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/onsi/gomega"

	"github.com/openshift-kni/commatrix/pkg/client"
	"github.com/openshift-kni/commatrix/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SoftRebootNodeAndWaitForDisconnect soft reboots given node and wait for node to be unreachable.
func SoftRebootNodeAndWaitForDisconnect(debugPod *v1.Pod, cs *client.ClientSet) error {
	utilsHelpers := utils.New(cs)
	nodeName := debugPod.Spec.NodeName
	rebootcmd := []string{"chroot", "/host", "reboot"}

	_, err := utilsHelpers.RunCommandOnPod(debugPod, rebootcmd)
	if err != nil {
		return fmt.Errorf("failed to reboot node: %s with error %w", nodeName, err)
	}

	WaitForNodeNotReady(nodeName, cs)
	return nil
}

// WaitForNodeNotReady waits for the node to be in the NotReady state in Kubernetes.
func WaitForNodeNotReady(nodeName string, cs *client.ClientSet) {
	log.Printf("Waiting for node %s to be in NotReady state", nodeName)

	timeout := 5 * time.Minute
	gomega.Eventually(func() bool {
		node, err := cs.CoreV1Interface.Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
		if err != nil {
			log.Printf("Error getting node %s: %v", node.Name, err)
			return false
		}

		for _, condition := range node.Status.Conditions {
			if condition.Type == v1.NodeReady && condition.Status != v1.ConditionTrue {
				return true
			}
		}
		return false
	}, timeout, 3*time.Second).Should(
		gomega.BeTrue(),
		fmt.Sprintf("Node %s is still ready after %s", nodeName, timeout.String()),
	)

	log.Printf("Node %s is NotReady\n", nodeName)
}

// WaitForNodeReady waits for the node to be in the Ready state in Kubernetes.
func WaitForNodeReady(nodeName string, cs *client.ClientSet) {
	log.Printf("Waiting for node %s to be in Ready state", nodeName)

	timeout := 15 * time.Minute
	gomega.Eventually(func() bool {
		node, err := cs.CoreV1Interface.Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
		if err != nil {
			log.Printf("Error getting node %s: %v", nodeName, err)
			return false
		}

		for _, condition := range node.Status.Conditions {
			if condition.Type == v1.NodeReady && condition.Status == v1.ConditionTrue {
				return true
			}
		}
		return false
	}, timeout, 3*time.Second).Should(
		gomega.BeTrue(),
		fmt.Sprintf("Node %s is still not ready after %s", nodeName, timeout.String()),
	)

	log.Printf("Node %s is Ready\n", nodeName)
}
