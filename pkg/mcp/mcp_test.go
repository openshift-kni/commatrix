package mcp

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift-kni/commatrix/pkg/consts"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Test 1: Annotation-based resolution coverage.
var _ = Describe("ResolveNodeToPool", func() {
	It("derives pool name from node annotation and handles errors", func() {
		type scenario struct {
			desc      string
			nodes     []corev1.Node
			expectMap map[string]string
			expectErr string
		}

		scenarios := []scenario{
			{
				desc: "master rendered",
				nodes: []corev1.Node{
					{ObjectMeta: metav1.ObjectMeta{Name: "n1", Annotations: map[string]string{"machineconfiguration.openshift.io/currentConfig": "rendered-master-abc"}}},
				},
				expectMap: map[string]string{"n1": "master"},
			},
			{
				desc: "worker rendered",
				nodes: []corev1.Node{
					{ObjectMeta: metav1.ObjectMeta{Name: "n2", Annotations: map[string]string{"machineconfiguration.openshift.io/currentConfig": "rendered-worker-123"}}},
				},
				expectMap: map[string]string{"n2": "worker"},
			},
			{
				desc: "custom with dash in name",
				nodes: []corev1.Node{
					{ObjectMeta: metav1.ObjectMeta{Name: "n3", Annotations: map[string]string{"machineconfiguration.openshift.io/currentConfig": "rendered-custom-ws-999"}}},
				},
				expectMap: map[string]string{"n3": "custom-ws"},
			},
			{
				desc: "two nodes mixed",
				nodes: []corev1.Node{
					{ObjectMeta: metav1.ObjectMeta{Name: "a", Annotations: map[string]string{"machineconfiguration.openshift.io/currentConfig": "rendered-master-aaaa"}}},
					{ObjectMeta: metav1.ObjectMeta{Name: "b", Annotations: map[string]string{"machineconfiguration.openshift.io/currentConfig": "rendered-custom-1-zzzz"}}},
				},
				expectMap: map[string]string{"a": "master", "b": "custom-1"},
			},
			{
				desc: "missing annotation",
				nodes: []corev1.Node{
					{ObjectMeta: metav1.ObjectMeta{Name: "bad1"}},
				},
				expectErr: "missing annotation",
			},
			{
				desc: "malformed: no trailing hash",
				nodes: []corev1.Node{
					{ObjectMeta: metav1.ObjectMeta{Name: "bad2", Annotations: map[string]string{"machineconfiguration.openshift.io/currentConfig": "rendered-master"}}},
				},
				expectErr: "malformed currentConfig",
			},
			{
				desc: "malformed: wrong prefix",
				nodes: []corev1.Node{
					{ObjectMeta: metav1.ObjectMeta{Name: "bad3", Annotations: map[string]string{"machineconfiguration.openshift.io/currentConfig": "master-abc"}}},
				},
				expectErr: "malformed currentConfig",
			},
		}

		for _, sc := range scenarios {
			mapping, err := ResolveNodeToPool(sc.nodes)
			if sc.expectErr != "" {
				Expect(err).To(HaveOccurred(), sc.desc)
				Expect(err.Error()).To(ContainSubstring(sc.expectErr), sc.desc)
				continue
			}
			Expect(err).NotTo(HaveOccurred(), sc.desc)
			Expect(mapping).To(Equal(sc.expectMap), sc.desc)
		}
	})
})

// Test 2: Role derivation across a single pool.
var _ = Describe("GetPoolRolesForStaticEntriesExpansion", func() {
	It("derives master/worker roles per pool based on node labels", func() {
		n1 := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{consts.RoleLabel + "master": ""}}}
		n2 := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n2", Labels: map[string]string{consts.RoleLabel + "worker": ""}}}
		n3 := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n3", Labels: map[string]string{consts.RoleLabel + "control-plane": ""}}}

		nodes := []corev1.Node{n1, n2, n3}

		manualMap := map[string]string{"n1": "custom", "n2": "custom", "n3": "custom"}

		roles := GetPoolRolesForStaticEntriesExpansion(nodes, manualMap)
		Expect(roles["custom"]).To(ContainElements("master", "worker"))
	})
})
