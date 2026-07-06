package utils

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openshift-kni/commatrix/pkg/client"
)

var testScheme *runtime.Scheme

var _ = BeforeSuite(func() {
	testScheme = runtime.NewScheme()
	Expect(corev1.AddToScheme(testScheme)).To(Succeed())
	Expect(configv1.AddToScheme(testScheme)).To(Succeed())
})

func newFakeUtils(objs ...runtime.Object) *utils {
	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithRuntimeObjects(objs...).
		Build()

	return &utils{&client.ClientSet{Client: fakeClient}}
}

var _ = DescribeTable("parseImageStreamTagString",
	func(input, expectedNs, expectedName, expectedTag string) {
		ns, name, tag := parseImageStreamTagString(input)
		Expect(ns).To(Equal(expectedNs))
		Expect(name).To(Equal(expectedName))
		Expect(tag).To(Equal(expectedTag))
	},
	Entry("parses a fully qualified namespace/name:tag", "openshift/cli:latest", "openshift", "cli", "latest"),
	Entry("returns empty namespace when no slash is present", "cli:latest", "", "cli", "latest"),
	Entry("defaults tag to 'latest' when no tag is present", "openshift/cli", "openshift", "cli", "latest"),
	Entry("defaults tag to 'latest' for empty string", "", "", "", "latest"),
	Entry("treats everything after the first slash as name:tag", "openshift/tools/cli:v1", "openshift", "tools/cli", "v1"),
)

var _ = Describe("getPodDefinition", func() {
	It("creates a pod with the correct spec fields", func() {
		pod := getPodDefinition("node-1", "test-ns", "registry.example.com/image:v1", []string{"echo", "hello"})
		Expect(pod.Namespace).To(Equal("test-ns"))
		Expect(pod.Spec.NodeName).To(Equal("node-1"))
		Expect(pod.Spec.Containers).To(HaveLen(1))
		Expect(pod.Spec.Containers[0].Image).To(Equal("registry.example.com/image:v1"))
		Expect(pod.Spec.Containers[0].Command).To(Equal([]string{"echo", "hello"}))
		Expect(pod.Spec.Containers[0].SecurityContext.Privileged).To(Equal(ptr.To(true)))
		Expect(pod.Spec.Containers[0].SecurityContext.RunAsUser).To(Equal(ptr.To(int64(0))))
		Expect(pod.Spec.HostNetwork).To(BeTrue())
		Expect(pod.Spec.HostPID).To(BeTrue())
		Expect(pod.Spec.Containers[0].VolumeMounts).To(HaveLen(1))
		Expect(pod.Spec.Containers[0].VolumeMounts[0].MountPath).To(Equal("/host"))
		Expect(pod.Spec.Volumes).To(HaveLen(1))
		Expect(pod.Spec.Volumes[0].HostPath.Path).To(Equal("/"))
	})

	It("uses a default sleep command when command is empty", func() {
		pod := getPodDefinition("node-1", "test-ns", "image:latest", nil)
		Expect(pod.Spec.Containers[0].Command).To(Equal([]string{"/bin/sh", "-c", "sleep INF"}))
	})
})

var _ = Describe("getNamespaceDefinition", func() {
	It("creates a namespace with pod security labels", func() {
		ns := getNamespaceDefinition("my-debug")
		Expect(ns.Name).To(Equal("my-debug"))
		Expect(ns.Labels).To(HaveKeyWithValue("pod-security.kubernetes.io/enforce", "privileged"))
		Expect(ns.Labels).To(HaveKeyWithValue("pod-security.kubernetes.io/audit", "privileged"))
		Expect(ns.Labels).To(HaveKeyWithValue("pod-security.kubernetes.io/warn", "privileged"))
	})
})

var _ = Describe("ListNodes", func() {
	It("returns an empty list when no nodes exist", func() {
		u := newFakeUtils()
		nodes, err := u.ListNodes()
		Expect(err).ToNot(HaveOccurred())
		Expect(nodes).To(BeEmpty())
	})

	It("returns all nodes in the cluster", func() {
		node1 := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}}
		node2 := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-2"}}
		u := newFakeUtils(node1, node2)

		nodes, err := u.ListNodes()
		Expect(err).ToNot(HaveOccurred())
		Expect(nodes).To(HaveLen(2))
	})
})

var _ = Describe("WriteFile", func() {
	It("writes data to a file", func() {
		dir := GinkgoT().TempDir()
		path := filepath.Join(dir, "test.txt")

		u := &utils{}
		err := u.WriteFile(path, []byte("hello"))
		Expect(err).ToNot(HaveOccurred())

		data, err := os.ReadFile(path)
		Expect(err).ToNot(HaveOccurred())
		Expect(string(data)).To(Equal("hello"))
	})

	It("returns an error for a nonexistent parent directory", func() {
		u := &utils{}
		err := u.WriteFile("/nonexistent/dir/file.txt", []byte("data"))
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("GetPlatformType", func() {
	It("returns the platform type from the Infrastructure CR", func() {
		infra := &configv1.Infrastructure{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
			Status: configv1.InfrastructureStatus{
				PlatformStatus: &configv1.PlatformStatus{
					Type: configv1.AWSPlatformType,
				},
			},
		}
		u := newFakeUtils(infra)

		pt, err := u.GetPlatformType()
		Expect(err).ToNot(HaveOccurred())
		Expect(pt).To(Equal(configv1.AWSPlatformType))
	})

	It("returns an error when Infrastructure CR is missing", func() {
		u := newFakeUtils()
		_, err := u.GetPlatformType()
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("GetControlPlaneTopology", func() {
	It("returns the topology from the Infrastructure CR", func() {
		infra := &configv1.Infrastructure{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
			Status: configv1.InfrastructureStatus{
				ControlPlaneTopology: configv1.HighlyAvailableTopologyMode,
				PlatformStatus:       &configv1.PlatformStatus{Type: configv1.AWSPlatformType},
			},
		}
		u := newFakeUtils(infra)

		topo, err := u.GetControlPlaneTopology()
		Expect(err).ToNot(HaveOccurred())
		Expect(topo).To(Equal(configv1.HighlyAvailableTopologyMode))
	})

	It("returns an error when Infrastructure CR is missing", func() {
		u := newFakeUtils()
		_, err := u.GetControlPlaneTopology()
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("GetClusterVersion", func() {
	It("returns major.minor from a full version string", func() {
		cv := &configv1.ClusterVersion{
			ObjectMeta: metav1.ObjectMeta{Name: "version"},
			Status: configv1.ClusterVersionStatus{
				Desired: configv1.Release{Version: "4.17.3"},
			},
		}
		u := newFakeUtils(cv)

		version, err := u.GetClusterVersion()
		Expect(err).ToNot(HaveOccurred())
		Expect(version).To(Equal("4.17"))
	})

	It("returns major.minor when only two parts exist", func() {
		cv := &configv1.ClusterVersion{
			ObjectMeta: metav1.ObjectMeta{Name: "version"},
			Status: configv1.ClusterVersionStatus{
				Desired: configv1.Release{Version: "4.16"},
			},
		}
		u := newFakeUtils(cv)

		version, err := u.GetClusterVersion()
		Expect(err).ToNot(HaveOccurred())
		Expect(version).To(Equal("4.16"))
	})

	It("returns an error for a single-part version", func() {
		cv := &configv1.ClusterVersion{
			ObjectMeta: metav1.ObjectMeta{Name: "version"},
			Status: configv1.ClusterVersionStatus{
				Desired: configv1.Release{Version: "4"},
			},
		}
		u := newFakeUtils(cv)

		_, err := u.GetClusterVersion()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unexpected cluster version format"))
	})

	It("returns an error when ClusterVersion CR is missing", func() {
		u := newFakeUtils()
		_, err := u.GetClusterVersion()
		Expect(err).To(HaveOccurred())
	})
})

var _ = DescribeTable("IsIPv6Enabled",
	func(cidrs []string, expectedIPv6 bool, expectErr bool) {
		var u *utils
		if cidrs == nil {
			// No Network CR at all
			u = newFakeUtils()
		} else {
			entries := make([]configv1.ClusterNetworkEntry, len(cidrs))
			for i, cidr := range cidrs {
				entries[i] = configv1.ClusterNetworkEntry{CIDR: cidr}
			}
			network := &configv1.Network{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.NetworkSpec{
					ClusterNetwork: entries,
				},
			}
			u = newFakeUtils(network)
		}

		ipv6, err := u.IsIPv6Enabled()
		if expectErr {
			Expect(err).To(HaveOccurred())
		} else {
			Expect(err).ToNot(HaveOccurred())
			Expect(ipv6).To(Equal(expectedIPv6))
		}
	},
	Entry("returns false for IPv4-only cluster", []string{"10.128.0.0/14"}, false, false),
	Entry("returns true for dual-stack cluster with IPv6", []string{"10.128.0.0/14", "fd01::/48"}, true, false),
	Entry("returns true for IPv6-only cluster", []string{"fd01::/48"}, true, false),
	Entry("returns false when no cluster networks are configured", []string{}, false, false),
	Entry("returns an error when Network CR is missing", nil, false, true),
)

var _ = Describe("DeletePod", func() {
	It("deletes an existing pod", func() {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
		}
		u := newFakeUtils(pod)

		err := u.DeletePod(pod)
		Expect(err).ToNot(HaveOccurred())
	})

	It("returns an error when the pod does not exist", func() {
		u := newFakeUtils()
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "nonexistent", Namespace: "default"},
		}

		err := u.DeletePod(pod)
		Expect(err).To(HaveOccurred())
	})
})
