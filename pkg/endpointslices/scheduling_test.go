package endpointslices

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetEligiblePoolsForPod_NoConstraints(t *testing.T) {
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "master-0", Labels: map[string]string{"node-role.kubernetes.io/master": ""}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "worker-0", Labels: map[string]string{"node-role.kubernetes.io/worker": ""}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "worker-1", Labels: map[string]string{"node-role.kubernetes.io/worker": ""}}},
	}
	nodeToGroup := map[string]string{
		"master-0": "master",
		"worker-0": "standard",
		"worker-1": "customcnf",
	}

	pod := corev1.Pod{Spec: corev1.PodSpec{}}

	pools := getEligiblePoolsForPod(pod, nodes, nodeToGroup)
	if len(pools) != 3 {
		t.Fatalf("expected 3 pools, got %d: %v", len(pools), pools)
	}
}

func TestGetEligiblePoolsForPod_WorkerNodeSelector(t *testing.T) {
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "master-0", Labels: map[string]string{"node-role.kubernetes.io/master": ""}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "worker-0", Labels: map[string]string{"node-role.kubernetes.io/worker": ""}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "worker-1", Labels: map[string]string{"node-role.kubernetes.io/worker": ""}}},
	}
	nodeToGroup := map[string]string{
		"master-0": "master",
		"worker-0": "standard",
		"worker-1": "customcnf",
	}

	pod := corev1.Pod{
		Spec: corev1.PodSpec{
			NodeSelector: map[string]string{"node-role.kubernetes.io/worker": ""},
		},
	}

	pools := getEligiblePoolsForPod(pod, nodes, nodeToGroup)
	if len(pools) != 2 {
		t.Fatalf("expected 2 worker pools, got %d: %v", len(pools), pools)
	}
	if pools[0] != "customcnf" || pools[1] != "standard" {
		t.Fatalf("expected [customcnf standard], got %v", pools)
	}
}

func TestGetEligiblePoolsForPod_MasterNodeSelector(t *testing.T) {
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "master-0", Labels: map[string]string{"node-role.kubernetes.io/master": ""}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "worker-0", Labels: map[string]string{"node-role.kubernetes.io/worker": ""}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "worker-1", Labels: map[string]string{"node-role.kubernetes.io/worker": ""}}},
	}
	nodeToGroup := map[string]string{
		"master-0": "master",
		"worker-0": "standard",
		"worker-1": "customcnf",
	}

	pod := corev1.Pod{
		Spec: corev1.PodSpec{
			NodeSelector: map[string]string{"node-role.kubernetes.io/master": ""},
		},
	}

	pools := getEligiblePoolsForPod(pod, nodes, nodeToGroup)
	if len(pools) != 1 || pools[0] != "master" {
		t.Fatalf("expected [master], got %v", pools)
	}
}

func TestGetEligiblePoolsForPod_CustomLabelNodeSelector(t *testing.T) {
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "master-0", Labels: map[string]string{"node-role.kubernetes.io/master": ""}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "worker-0", Labels: map[string]string{
			"node-role.kubernetes.io/worker": "",
			"feature.node.kubernetes.io/ptp": "true",
		}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "worker-1", Labels: map[string]string{
			"node-role.kubernetes.io/worker": "",
		}}},
	}
	nodeToGroup := map[string]string{
		"master-0": "master",
		"worker-0": "standard",
		"worker-1": "customcnf",
	}

	pod := corev1.Pod{
		Spec: corev1.PodSpec{
			NodeSelector: map[string]string{
				"node-role.kubernetes.io/worker": "",
				"feature.node.kubernetes.io/ptp": "true",
			},
		},
	}

	pools := getEligiblePoolsForPod(pod, nodes, nodeToGroup)
	if len(pools) != 1 || pools[0] != "standard" {
		t.Fatalf("expected [standard], got %v", pools)
	}
}

func TestGetEligiblePoolsForPod_RequiredNodeAffinity(t *testing.T) {
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "master-0", Labels: map[string]string{"node-role.kubernetes.io/master": ""}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "worker-0", Labels: map[string]string{"node-role.kubernetes.io/worker": ""}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "worker-1", Labels: map[string]string{"node-role.kubernetes.io/worker": ""}}},
	}
	nodeToGroup := map[string]string{
		"master-0": "master",
		"worker-0": "standard",
		"worker-1": "customcnf",
	}

	pod := corev1.Pod{
		Spec: corev1.PodSpec{
			Affinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "node-role.kubernetes.io/worker",
										Operator: corev1.NodeSelectorOpExists,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	pools := getEligiblePoolsForPod(pod, nodes, nodeToGroup)
	if len(pools) != 2 {
		t.Fatalf("expected 2 worker pools, got %d: %v", len(pools), pools)
	}
	if pools[0] != "customcnf" || pools[1] != "standard" {
		t.Fatalf("expected [customcnf standard], got %v", pools)
	}
}

func TestGetEligiblePoolsForPod_NodeAffinityIn(t *testing.T) {
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "worker-0", Labels: map[string]string{
			"node-role.kubernetes.io/worker": "",
			"zone":                           "us-east-1a",
		}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "worker-1", Labels: map[string]string{
			"node-role.kubernetes.io/worker": "",
			"zone":                           "us-west-2a",
		}}},
	}
	nodeToGroup := map[string]string{
		"worker-0": "pool-east",
		"worker-1": "pool-west",
	}

	pod := corev1.Pod{
		Spec: corev1.PodSpec{
			Affinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "zone",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"us-east-1a"},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	pools := getEligiblePoolsForPod(pod, nodes, nodeToGroup)
	if len(pools) != 1 || pools[0] != "pool-east" {
		t.Fatalf("expected [pool-east], got %v", pools)
	}
}

func TestGetEligiblePoolsForPod_NodeAffinityNotIn(t *testing.T) {
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "worker-0", Labels: map[string]string{
			"node-role.kubernetes.io/worker": "",
			"zone":                           "us-east-1a",
		}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "worker-1", Labels: map[string]string{
			"node-role.kubernetes.io/worker": "",
			"zone":                           "us-west-2a",
		}}},
	}
	nodeToGroup := map[string]string{
		"worker-0": "pool-east",
		"worker-1": "pool-west",
	}

	pod := corev1.Pod{
		Spec: corev1.PodSpec{
			Affinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "zone",
										Operator: corev1.NodeSelectorOpNotIn,
										Values:   []string{"us-east-1a"},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	pools := getEligiblePoolsForPod(pod, nodes, nodeToGroup)
	if len(pools) != 1 || pools[0] != "pool-west" {
		t.Fatalf("expected [pool-west], got %v", pools)
	}
}

func TestGetEligiblePoolsForPod_NodeAffinityDoesNotExist(t *testing.T) {
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "worker-0", Labels: map[string]string{
			"node-role.kubernetes.io/worker": "",
			"gpu":                            "true",
		}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "worker-1", Labels: map[string]string{
			"node-role.kubernetes.io/worker": "",
		}}},
	}
	nodeToGroup := map[string]string{
		"worker-0": "gpu-pool",
		"worker-1": "standard",
	}

	pod := corev1.Pod{
		Spec: corev1.PodSpec{
			Affinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "gpu",
										Operator: corev1.NodeSelectorOpDoesNotExist,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	pools := getEligiblePoolsForPod(pod, nodes, nodeToGroup)
	if len(pools) != 1 || pools[0] != "standard" {
		t.Fatalf("expected [standard], got %v", pools)
	}
}

func TestNodeMatchesPodConstraints_BothNodeSelectorAndAffinity(t *testing.T) {
	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"node-role.kubernetes.io/worker": "",
				"zone":                           "us-east-1a",
			},
		},
	}

	pod := corev1.Pod{
		Spec: corev1.PodSpec{
			NodeSelector: map[string]string{
				"node-role.kubernetes.io/worker": "",
			},
			Affinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "zone",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"us-west-2a"},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if nodeMatchesPodConstraints(pod, &node) {
		t.Fatal("expected node NOT to match (nodeSelector matches but nodeAffinity doesn't)")
	}
}

func TestIsStaticPod(t *testing.T) {
	staticPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-apiserver-master-0",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "Node", Name: "master-0"},
			},
		},
	}
	if !isStaticPod(staticPod) {
		t.Fatal("expected pod with Node owner to be detected as static")
	}

	regularPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "router-default-abc",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "DaemonSet", Name: "router-default"},
			},
		},
	}
	if isStaticPod(regularPod) {
		t.Fatal("expected DaemonSet-owned pod NOT to be detected as static")
	}

	noOwnerPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "standalone"},
	}
	if isStaticPod(noOwnerPod) {
		t.Fatal("expected pod with no owner NOT to be detected as static")
	}
}

func TestNodeMatchesPodConstraints_ORSelectorTerms(t *testing.T) {
	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"zone": "us-west-2a",
			},
		},
	}

	pod := corev1.Pod{
		Spec: corev1.PodSpec{
			Affinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{Key: "zone", Operator: corev1.NodeSelectorOpIn, Values: []string{"us-east-1a"}},
								},
							},
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{Key: "zone", Operator: corev1.NodeSelectorOpIn, Values: []string{"us-west-2a"}},
								},
							},
						},
					},
				},
			},
		},
	}

	if !nodeMatchesPodConstraints(pod, &node) {
		t.Fatal("expected node to match (second OR term matches)")
	}
}
