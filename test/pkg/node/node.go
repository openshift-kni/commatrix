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

const (
	timeout  = 20 * time.Minute
	interval = 3 * time.Second
)

// SoftRebootNodeAndWaitForDisconnect soft reboots given node and wait for node to be unreachable.
func SoftRebootNodeAndWaitForDisconnect(utilsHelpers utils.UtilsInterface, cs *client.ClientSet, nodeName, ns string) error {
	debugPod, err := utilsHelpers.CreatePodOnNode(nodeName, ns, consts.DefaultDebugPodImage, []string{})
	if err != nil {
		return fmt.Errorf("failed to create debug pod on node %s: %w", nodeName, err)
	}

	defer func() {
		err := utilsHelpers.DeletePod(debugPod)
		if err != nil {
			log.Printf("failed cleaning debug pod %s: %v", debugPod, err)
		}
	}()

	err = utilsHelpers.WaitForPodStatus(ns, debugPod, v1.PodRunning)
	if err != nil {
		return err
	}

	rebootCmd := []string{"chroot", "/host", "reboot"}
	_, err = utilsHelpers.RunCommandOnPod(debugPod, rebootCmd)
	if err != nil {
		return fmt.Errorf("failed to reboot node: %s with error %w", nodeName, err)
	}

	WaitForNodeNotReady(nodeName, cs)
	return nil
}

// WaitForNodeNotReady waits for the node to be in the NotReady state.
func WaitForNodeNotReady(nodeName string, cs *client.ClientSet) {
	log.Printf("Waiting for node %s to be in NotReady state", nodeName)

	gomega.Eventually(func() bool {
		node, err := cs.Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
		if err != nil {
			log.Printf("Error getting node %s: %v", nodeName, err)
			return false
		}

		for _, condition := range node.Status.Conditions {
			if condition.Type == v1.NodeReady && condition.Status != v1.ConditionTrue {
				return true
			}
		}
		return false
	}, timeout, interval).Should(
		gomega.BeTrue(),
		fmt.Sprintf("Node %s is still ready after %s", nodeName, timeout.String()),
	)

	log.Printf("Node %s is NotReady\n", nodeName)
}

// WaitForNodeReady waits for the node to be in the Ready state.
func WaitForNodeReady(nodeName string, cs *client.ClientSet) {
	log.Printf("Waiting for node %s to be in Ready state", nodeName)

	gomega.Eventually(func() bool {
		node, err := cs.Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
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
	}, timeout, interval).Should(
		gomega.BeTrue(),
		fmt.Sprintf("Node %s is still not ready after %s", nodeName, timeout.String()),
	)

	log.Printf("Node %s is Ready\n", nodeName)
}
