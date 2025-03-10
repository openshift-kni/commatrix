package types

var GeneralStaticEntriesWorker = []ComDetails{
	{
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      22,
		NodeRole:  "worker",
		Service:   "sshd",
		Namespace: "Host system service",
		Pod:       "",
		Container: "",
		Optional:  true,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      9637,
		NodeRole:  "worker",
		Service:   "kube-rbac-proxy-crio",
		Namespace: "openshift-machine-config-operator",
		Pod:       "kube-rbac-proxy-crio",
		Container: "kube-rbac-proxy-crio",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      10250,
		NodeRole:  "worker",
		Service:   "kubelet",
		Namespace: "Host system service",
		Pod:       "",
		Container: "",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      9107,
		NodeRole:  "worker",
		Service:   "egressip-node-healthcheck",
		Namespace: "openshift-ovn-kubernetes",
		Pod:       "ovnkube-node",
		Container: "ovnkube-controller",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      111,
		NodeRole:  "worker",
		Service:   "rpcbind",
		Namespace: "Host system service",
		Pod:       "",
		Container: "",
		Optional:  true,
	}, {
		Direction: "Ingress",
		Protocol:  "UDP",
		Port:      111,
		NodeRole:  "worker",
		Service:   "rpcbind",
		Namespace: "Host system service",
		Pod:       "",
		Container: "",
		Optional:  true,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      10256,
		NodeRole:  "worker",
		Service:   "ovnkube",
		Namespace: "openshift-ovn-kubernetes",
		Pod:       "ovnkube",
		Container: "ovnkube-controller",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      9537,
		NodeRole:  "worker",
		Service:   "crio-metrics",
		Namespace: "Host system service",
		Pod:       "",
		Container: "",
		Optional:  false,
	},
}

var GeneralStaticEntriesMaster = []ComDetails{
	{
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      8080,
		NodeRole:  "master",
		Service:   "",
		Namespace: "openshift-network-operator",
		Pod:       "network-operator",
		Container: "network-operator",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      9637,
		NodeRole:  "master",
		Service:   "kube-rbac-proxy-crio",
		Namespace: "openshift-machine-config-operator",
		Pod:       "kube-rbac-proxy-crio",
		Container: "kube-rbac-proxy-crio",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      10256,
		NodeRole:  "master",
		Service:   "ovnkube",
		Namespace: "openshift-ovn-kubernetes",
		Pod:       "ovnkube",
		Container: "ovnkube-controller",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      9537,
		NodeRole:  "master",
		Service:   "crio-metrics",
		Namespace: "Host system service",
		Pod:       "",
		Container: "",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      10250,
		NodeRole:  "master",
		Service:   "kubelet",
		Namespace: "Host system service",
		Pod:       "",
		Container: "",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      9107,
		NodeRole:  "master",
		Service:   "egressip-node-healthcheck",
		Namespace: "openshift-ovn-kubernetes",
		Pod:       "ovnkube-node",
		Container: "ovnkube-controller",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      111,
		NodeRole:  "master",
		Service:   "rpcbind",
		Namespace: "Host system service",
		Pod:       "",
		Container: "",
		Optional:  true,
	}, {
		Direction: "Ingress",
		Protocol:  "UDP",
		Port:      111,
		NodeRole:  "master",
		Service:   "rpcbind",
		Namespace: "Host system service",
		Pod:       "",
		Container: "",
		Optional:  true,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      22,
		NodeRole:  "master",
		Service:   "sshd",
		Namespace: "Host system service",
		Pod:       "",
		Container: "",
		Optional:  true,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      9258,
		NodeRole:  "master",
		Service:   "machine-approver",
		Namespace: "openshift-cloud-controller-manager-operator",
		Pod:       "cluster-cloud-controller-manager",
		Container: "cluster-cloud-controller-manager",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      9980,
		NodeRole:  "master",
		Service:   "etcd",
		Namespace: "openshift-etcd",
		Pod:       "etcd",
		Container: "etcd",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      9978,
		NodeRole:  "master",
		Service:   "etcd",
		Namespace: "openshift-etcd",
		Pod:       "etcd",
		Container: "etcd-metrics",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      10357,
		NodeRole:  "master",
		Service:   "openshift-kube-apiserver-healthz",
		Namespace: "openshift-kube-apiserver",
		Pod:       "kube-apiserver",
		Container: "kube-apiserver-check-endpoints",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      17697,
		NodeRole:  "master",
		Service:   "openshift-kube-apiserver-healthz",
		Namespace: "openshift-kube-apiserver",
		Pod:       "kube-apiserver",
		Container: "kube-apiserver-check-endpoints",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      2380,
		NodeRole:  "master",
		Service:   "healthz",
		Namespace: "openshift-etcd",
		Pod:       "etcd",
		Container: "etcd",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      6080,
		NodeRole:  "master",
		Service:   "",
		Namespace: "openshift-kube-apiserver",
		Pod:       "kube-apiserver",
		Container: "kube-apiserver-insecure-readyz",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      22624,
		NodeRole:  "master",
		Service:   "machine-config-server",
		Namespace: "openshift-machine-config-operator",
		Pod:       "machine-config-server",
		Container: "machine-config-server",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      22623,
		NodeRole:  "master",
		Service:   "machine-config-server",
		Namespace: "openshift-machine-config-operator",
		Pod:       "machine-config-server",
		Container: "machine-config-server",
		Optional:  false,
	},
}

var BaremetalStaticEntriesWorker = []ComDetails{
	{
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      53,
		NodeRole:  "worker",
		Service:   "dns-default",
		Namespace: "openshift-dns",
		Pod:       "dnf-default",
		Container: "dns",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "UDP",
		Port:      53,
		NodeRole:  "worker",
		Service:   "dns-default",
		Namespace: "openshift-dns",
		Pod:       "dnf-default",
		Container: "dns",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      18080,
		NodeRole:  "worker",
		Service:   "",
		Namespace: "openshift-kni-infra",
		Pod:       "coredns",
		Container: "coredns",
		Optional:  false,
	},
}

var BaremetalStaticEntriesMaster = []ComDetails{
	{
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      53,
		NodeRole:  "master",
		Service:   "dns-default",
		Namespace: "openshift-dns",
		Pod:       "dnf-default",
		Container: "dns",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "UDP",
		Port:      53,
		NodeRole:  "master",
		Service:   "dns-default",
		Namespace: "openshift-dns",
		Pod:       "dnf-default",
		Container: "dns",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      5050,
		NodeRole:  "master",
		Service:   "",
		Namespace: "openshift-machine-api",
		Pod:       "ironic-proxy",
		Container: "ironic-proxy",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      9444,
		NodeRole:  "master",
		Service:   "",
		Namespace: "openshift-kni-infra",
		Pod:       "haproxy",
		Container: "haproxy",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      9445,
		NodeRole:  "master",
		Service:   "",
		Namespace: "openshift-kni-infra",
		Pod:       "haproxy",
		Container: "haproxy",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      6385,
		NodeRole:  "master",
		Service:   "",
		Namespace: "openshift-machine-api",
		Pod:       "ironic-proxy",
		Container: "ironic-proxy",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      18080,
		NodeRole:  "master",
		Service:   "",
		Namespace: "openshift-kni-infra",
		Pod:       "coredns",
		Container: "coredns",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      9447,
		NodeRole:  "master",
		Service:   "",
		Namespace: "openshift-machine-api",
		Pod:       "metal3-baremetal-operator",
		Container: "",
		Optional:  false,
	},
}

var CloudStaticEntriesWorker = []ComDetails{
	{
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      10300,
		NodeRole:  "worker",
		Service:   "csi-livenessprobe",
		Namespace: "openshift-cluster-csi-drivers",
		Pod:       "csi-driver-node",
		Container: "csi-driver",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      10309,
		NodeRole:  "worker",
		Service:   "csi-node-driver",
		Namespace: "openshift-cluster-csi-drivers",
		Pod:       "csi-driver-node",
		Container: "csi-node-driver-registrar",
		Optional:  false,
	},
}

var CloudStaticEntriesMaster = []ComDetails{
	{
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      10260,
		NodeRole:  "master",
		Service:   "cloud-controller",
		Namespace: "openshift-cloud-controller-manager-operator",
		Pod:       "cloud-controller-manager",
		Container: "cloud-controller-manager",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      10258,
		NodeRole:  "master",
		Service:   "cloud-controller",
		Namespace: "openshift-cloud-controller-manager-operator",
		Pod:       "cloud-controller-manager",
		Container: "cloud-controller-manager",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      10300,
		NodeRole:  "master",
		Service:   "csi-livenessprobe",
		Namespace: "openshift-cluster-csi-drivers",
		Pod:       "csi-driver-node",
		Container: "csi-driver",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      10309,
		NodeRole:  "master",
		Service:   "csi-node-driver",
		Namespace: "openshift-cluster-csi-drivers",
		Pod:       "csi-driver-node",
		Container: "csi-node-driver-registrar",
		Optional:  false,
	},
}

var StandardStaticEntries = []ComDetails{
	{
		Direction: "Ingress",
		Protocol:  "UDP",
		Port:      6081,
		NodeRole:  "worker",
		Service:   "ovn-kubernetes geneve",
		Namespace: "openshift-ovn-kubernetes",
		Pod:       "",
		Container: "",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  "UDP",
		Port:      6081,
		NodeRole:  "master",
		Service:   "ovn-kubernetes geneve",
		Namespace: "openshift-ovn-kubernetes",
		Pod:       "",
		Container: "",
		Optional:  false,
	},
}
