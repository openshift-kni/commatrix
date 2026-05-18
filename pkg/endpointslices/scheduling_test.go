package endpointslices

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetEligiblePoolsForPods(t *testing.T) {
	defaultNodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "master-0", Labels: map[string]string{"node-role.kubernetes.io/master": ""}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "worker-0", Labels: map[string]string{
			"node-role.kubernetes.io/worker": "",
			"feature.node.kubernetes.io/ptp": "true",
		}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "worker-1", Labels: map[string]string{"node-role.kubernetes.io/worker": ""}}},
	}
	defaultNodeToGroup := map[string]string{
		"master-0": "master",
		"worker-0": "standard",
		"worker-1": "customcnf",
	}

	tests := []struct {
		name        string
		pods        []corev1.Pod
		nodes       []corev1.Node
		nodeToGroup map[string]string
		wantPools   []string
	}{
		{
			name:        "no constraints matches all pools",
			pods:        []corev1.Pod{{Spec: corev1.PodSpec{}}},
			nodes:       defaultNodes,
			nodeToGroup: defaultNodeToGroup,
			wantPools:   []string{"customcnf", "master", "standard"},
		},
		{
			name: "worker nodeSelector matches worker pools only",
			pods: []corev1.Pod{{
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{"node-role.kubernetes.io/worker": ""},
				},
			}},
			nodes:       defaultNodes,
			nodeToGroup: defaultNodeToGroup,
			wantPools:   []string{"customcnf", "standard"},
		},
		{
			name: "master nodeSelector matches master pool only",
			pods: []corev1.Pod{{
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{"node-role.kubernetes.io/master": ""},
				},
			}},
			nodes:       defaultNodes,
			nodeToGroup: defaultNodeToGroup,
			wantPools:   []string{"master"},
		},
		{
			name: "custom label narrows to a single pool",
			pods: []corev1.Pod{{
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						"node-role.kubernetes.io/worker": "",
						"feature.node.kubernetes.io/ptp": "true",
					},
				},
			}},
			nodes:       defaultNodes,
			nodeToGroup: defaultNodeToGroup,
			wantPools:   []string{"standard"},
		},
		{
			name: "required nodeAffinity Exists matches worker pools",
			pods: []corev1.Pod{{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{{
									MatchExpressions: []corev1.NodeSelectorRequirement{{
										Key:      "node-role.kubernetes.io/worker",
										Operator: corev1.NodeSelectorOpExists,
									}},
								}},
							},
						},
					},
				},
			}},
			nodes:       defaultNodes,
			nodeToGroup: defaultNodeToGroup,
			wantPools:   []string{"customcnf", "standard"},
		},
		{
			name: "nodeAffinity In operator",
			pods: []corev1.Pod{{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{{
									MatchExpressions: []corev1.NodeSelectorRequirement{{
										Key:      "zone",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"us-east-1a"},
									}},
								}},
							},
						},
					},
				},
			}},
			nodes: []corev1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "worker-0", Labels: map[string]string{"zone": "us-east-1a"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "worker-1", Labels: map[string]string{"zone": "us-west-2a"}}},
			},
			nodeToGroup: map[string]string{"worker-0": "pool-east", "worker-1": "pool-west"},
			wantPools:   []string{"pool-east"},
		},
		{
			name: "nodeAffinity NotIn operator",
			pods: []corev1.Pod{{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{{
									MatchExpressions: []corev1.NodeSelectorRequirement{{
										Key:      "zone",
										Operator: corev1.NodeSelectorOpNotIn,
										Values:   []string{"us-east-1a"},
									}},
								}},
							},
						},
					},
				},
			}},
			nodes: []corev1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "worker-0", Labels: map[string]string{"zone": "us-east-1a"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "worker-1", Labels: map[string]string{"zone": "us-west-2a"}}},
			},
			nodeToGroup: map[string]string{"worker-0": "pool-east", "worker-1": "pool-west"},
			wantPools:   []string{"pool-west"},
		},
		{
			name: "nodeAffinity DoesNotExist operator",
			pods: []corev1.Pod{{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{{
									MatchExpressions: []corev1.NodeSelectorRequirement{{
										Key:      "gpu",
										Operator: corev1.NodeSelectorOpDoesNotExist,
									}},
								}},
							},
						},
					},
				},
			}},
			nodes: []corev1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "worker-0", Labels: map[string]string{"gpu": "true"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "worker-1", Labels: map[string]string{}}},
			},
			nodeToGroup: map[string]string{"worker-0": "gpu-pool", "worker-1": "standard"},
			wantPools:   []string{"standard"},
		},
		{
			name: "both nodeSelector and nodeAffinity must match",
			pods: []corev1.Pod{{
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{"node-role.kubernetes.io/worker": ""},
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{{
									MatchExpressions: []corev1.NodeSelectorRequirement{{
										Key:      "zone",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"us-west-2a"},
									}},
								}},
							},
						},
					},
				},
			}},
			nodes: []corev1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "worker-0", Labels: map[string]string{
					"node-role.kubernetes.io/worker": "", "zone": "us-east-1a",
				}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "worker-1", Labels: map[string]string{
					"node-role.kubernetes.io/worker": "", "zone": "us-west-2a",
				}}},
			},
			nodeToGroup: map[string]string{"worker-0": "pool-east", "worker-1": "pool-west"},
			wantPools:   []string{"pool-west"},
		},
		{
			name: "OR selector terms match multiple pools",
			pods: []corev1.Pod{{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{MatchExpressions: []corev1.NodeSelectorRequirement{
										{Key: "zone", Operator: corev1.NodeSelectorOpIn, Values: []string{"us-east-1a"}},
									}},
									{MatchExpressions: []corev1.NodeSelectorRequirement{
										{Key: "zone", Operator: corev1.NodeSelectorOpIn, Values: []string{"us-west-2a"}},
									}},
								},
							},
						},
					},
				},
			}},
			nodes: []corev1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "worker-0", Labels: map[string]string{"zone": "us-east-1a"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "worker-1", Labels: map[string]string{"zone": "us-west-2a"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "worker-2", Labels: map[string]string{"zone": "eu-central-1a"}}},
			},
			nodeToGroup: map[string]string{"worker-0": "pool-east", "worker-1": "pool-west", "worker-2": "pool-eu"},
			wantPools:   []string{"pool-east", "pool-west"},
		},
		{
			name: "preferred affinity only matches all pools",
			pods: []corev1.Pod{{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{{
								Weight: 100,
								Preference: corev1.NodeSelectorTerm{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{Key: "node-role.kubernetes.io/worker", Operator: corev1.NodeSelectorOpExists},
									},
								},
							}},
						},
					},
				},
			}},
			nodes:       defaultNodes,
			nodeToGroup: defaultNodeToGroup,
			wantPools:   []string{"customcnf", "master", "standard"},
		},
		{
			name: "multiple pods with different selectors returns union",
			pods: []corev1.Pod{
				{Spec: corev1.PodSpec{NodeSelector: map[string]string{"node-role.kubernetes.io/master": ""}}},
				{Spec: corev1.PodSpec{NodeSelector: map[string]string{"custom-group": "special"}}},
			},
			nodes: []corev1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "master-0", Labels: map[string]string{"node-role.kubernetes.io/master": ""}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "worker-0", Labels: map[string]string{"node-role.kubernetes.io/worker": ""}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "worker-1", Labels: map[string]string{"node-role.kubernetes.io/worker": "", "custom-group": "special"}}},
			},
			nodeToGroup: defaultNodeToGroup,
			wantPools:   []string{"customcnf", "master"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getEligiblePoolsForPods(tt.pods, tt.nodes, tt.nodeToGroup)
			if len(got) != len(tt.wantPools) {
				t.Fatalf("expected %d pools %v, got %d: %v", len(tt.wantPools), tt.wantPools, len(got), got)
			}
			for i := range tt.wantPools {
				if got[i] != tt.wantPools[i] {
					t.Fatalf("expected %v, got %v", tt.wantPools, got)
				}
			}
		})
	}
}

func TestFilterNonStaticPods(t *testing.T) {
	staticPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "kube-apiserver-master-0",
			OwnerReferences: []metav1.OwnerReference{{Kind: "Node", Name: "master-0"}},
		},
	}
	regularPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "router-default-abc",
			OwnerReferences: []metav1.OwnerReference{{Kind: "DaemonSet", Name: "router-default"}},
		},
	}
	noOwnerPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "standalone"},
	}

	tests := []struct {
		name          string
		pods          []corev1.Pod
		wantNonStatic int
	}{
		{"static pod only", []corev1.Pod{staticPod}, 0},
		{"DaemonSet-owned pod only", []corev1.Pod{regularPod}, 1},
		{"pod with no owner", []corev1.Pod{noOwnerPod}, 1},
		{"mixed slice returns only non-static pods", []corev1.Pod{regularPod, staticPod, noOwnerPod}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nonStatic := filterNonStaticPods(tt.pods)
			if len(nonStatic) != tt.wantNonStatic {
				t.Fatalf("len(nonStatic) = %d, want %d", len(nonStatic), tt.wantNonStatic)
			}
		})
	}
}
