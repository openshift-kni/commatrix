package mcp

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift-kni/commatrix/pkg/client"
	"github.com/openshift-kni/commatrix/pkg/consts"

	machineconfigurationv1 "github.com/openshift/api/machineconfiguration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// Test 1: Annotation-based resolution coverage.
var _ = Describe("ResolveNodeToPool", func() {
	It("derives pool name from node annotation and handles errors", func() {
		type scenario struct {
			desc      string
			objects   []crclient.Object
			expectMap map[string]string
			expectErr string
		}
		createScheme := func() *runtime.Scheme {
			sch := runtime.NewScheme()
			Expect(corev1.AddToScheme(sch)).To(Succeed())
			Expect(machineconfigurationv1.AddToScheme(sch)).To(Succeed())
			return sch
		}

		scenarios := []scenario{
			{
				desc: "master rendered",
				objects: []crclient.Object{
					&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1", Annotations: map[string]string{"machineconfiguration.openshift.io/currentConfig": "rendered-master-abc"}}},
				},
				expectMap: map[string]string{"n1": "master"},
			},
			{
				desc: "worker rendered",
				objects: []crclient.Object{
					&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n2", Annotations: map[string]string{"machineconfiguration.openshift.io/currentConfig": "rendered-worker-123"}}},
				},
				expectMap: map[string]string{"n2": "worker"},
			},
			{
				desc: "custom with dash in name",
				objects: []crclient.Object{
					&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n3", Annotations: map[string]string{"machineconfiguration.openshift.io/currentConfig": "rendered-custom-ws-999"}}},
				},
				expectMap: map[string]string{"n3": "custom-ws"},
			},
			{
				desc: "two nodes mixed",
				objects: []crclient.Object{
					&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "a", Annotations: map[string]string{"machineconfiguration.openshift.io/currentConfig": "rendered-master-aaaa"}}},
					&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "b", Annotations: map[string]string{"machineconfiguration.openshift.io/currentConfig": "rendered-custom-1-zzzz"}}},
				},
				expectMap: map[string]string{"a": "master", "b": "custom-1"},
			},
			{
				desc: "missing annotation",
				objects: []crclient.Object{
					&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "bad1"}},
				},
				expectErr: "missing annotation",
			},
			{
				desc: "malformed: no trailing hash",
				objects: []crclient.Object{
					&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "bad2", Annotations: map[string]string{"machineconfiguration.openshift.io/currentConfig": "rendered-master"}}},
				},
				expectErr: "malformed currentConfig",
			},
			{
				desc: "malformed: wrong prefix",
				objects: []crclient.Object{
					&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "bad3", Annotations: map[string]string{"machineconfiguration.openshift.io/currentConfig": "master-abc"}}},
				},
				expectErr: "malformed currentConfig",
			},
		}

		for _, sc := range scenarios {
			sch := createScheme()
			fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(sc.objects...).Build()
			cs := &client.ClientSet{Client: fakeClient}
			mapping, err := ResolveNodeToPool(cs)
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
		sch := runtime.NewScheme()
		Expect(corev1.AddToScheme(sch)).To(Succeed())
		Expect(machineconfigurationv1.AddToScheme(sch)).To(Succeed())

		n1 := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{consts.RoleLabel + "master": ""}}}
		n2 := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n2", Labels: map[string]string{consts.RoleLabel + "worker": ""}}}
		n3 := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n3", Labels: map[string]string{consts.RoleLabel + "control-plane": ""}}}

		pm := &machineconfigurationv1.MachineConfigPool{ObjectMeta: metav1.ObjectMeta{Name: "master"}, Spec: machineconfigurationv1.MachineConfigPoolSpec{NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{consts.RoleLabel + "master": ""}}}, Status: machineconfigurationv1.MachineConfigPoolStatus{MachineCount: 1}}
		pw := &machineconfigurationv1.MachineConfigPool{ObjectMeta: metav1.ObjectMeta{Name: "worker"}, Spec: machineconfigurationv1.MachineConfigPoolSpec{NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{consts.RoleLabel + "worker": ""}}}, Status: machineconfigurationv1.MachineConfigPoolStatus{MachineCount: 1}}

		fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(n1, n2, n3, pm, pw).Build()
		cs := &client.ClientSet{Client: fakeClient}

		// We supply a single pool name to collect both roles
		manualMap := map[string]string{"n1": "custom", "n2": "custom", "n3": "custom"}

		roles, err := GetPoolRolesForStaticEntriesExpansion(cs, manualMap)
		Expect(err).NotTo(HaveOccurred())
		Expect(roles["custom"]).To(ContainElements("master", "worker"))
	})
})
