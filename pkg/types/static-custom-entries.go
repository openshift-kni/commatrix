package types

import (
	"fmt"

	configv1 "github.com/openshift/api/config/v1"

	"github.com/openshift-kni/commatrix/pkg/consts"
)

var GeneralStaticEntriesWorker = []ComDetails{
	{
		Direction: "Ingress",
		Protocol:  consts.ProtocolTCP,
		Port:      22,
		NodeGroup: "worker",
		Service:   "sshd",
		Namespace: "Host system service",
		Pod:       "",
		Container: "",
		Optional:  true,
	}, {
		Direction: "Ingress",
		Protocol:  consts.ProtocolTCP,
		Port:      10250,
		NodeGroup: "worker",
		Service:   "kubelet",
		Namespace: "Host system service",
		Pod:       "",
		Container: "",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  consts.ProtocolTCP,
		Port:      9107,
		NodeGroup: "worker",
		Service:   "egressip-node-healthcheck",
		Namespace: "openshift-ovn-kubernetes",
		Pod:       "ovnkube-node",
		Container: "ovnkube-controller",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  consts.ProtocolTCP,
		Port:      111,
		NodeGroup: "worker",
		Service:   "rpcbind",
		Namespace: "Host system service",
		Pod:       "",
		Container: "",
		Optional:  true,
	}, {
		Direction: "Ingress",
		Protocol:  consts.ProtocolUDP,
		Port:      111,
		NodeGroup: "worker",
		Service:   "rpcbind",
		Namespace: "Host system service",
		Pod:       "",
		Container: "",
		Optional:  true,
	}, {
		Direction: "Ingress",
		Protocol:  consts.ProtocolTCP,
		Port:      10256,
		NodeGroup: "worker",
		Service:   "ovnkube",
		Namespace: "openshift-ovn-kubernetes",
		Pod:       "ovnkube",
		Container: "ovnkube-controller",
		Optional:  false,
	},
}

var GeneralStaticEntriesMaster = []ComDetails{
	{
		Direction: "Ingress",
		Protocol:  consts.ProtocolTCP,
		Port:      10256,
		NodeGroup: "master",
		Service:   "ovnkube",
		Namespace: "openshift-ovn-kubernetes",
		Pod:       "ovnkube",
		Container: "ovnkube-controller",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  consts.ProtocolTCP,
		Port:      10250,
		NodeGroup: "master",
		Service:   "kubelet",
		Namespace: "Host system service",
		Pod:       "",
		Container: "",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  consts.ProtocolTCP,
		Port:      9107,
		NodeGroup: "master",
		Service:   "egressip-node-healthcheck",
		Namespace: "openshift-ovn-kubernetes",
		Pod:       "ovnkube-node",
		Container: "ovnkube-controller",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  consts.ProtocolTCP,
		Port:      111,
		NodeGroup: "master",
		Service:   "rpcbind",
		Namespace: "Host system service",
		Pod:       "",
		Container: "",
		Optional:  true,
	}, {
		Direction: "Ingress",
		Protocol:  consts.ProtocolUDP,
		Port:      111,
		NodeGroup: "master",
		Service:   "rpcbind",
		Namespace: "Host system service",
		Pod:       "",
		Container: "",
		Optional:  true,
	}, {
		Direction: "Ingress",
		Protocol:  consts.ProtocolTCP,
		Port:      22,
		NodeGroup: "master",
		Service:   "sshd",
		Namespace: "Host system service",
		Pod:       "",
		Container: "",
		Optional:  true,
	},
}

var BaremetalStaticEntriesWorker = []ComDetails{
	{
		Direction: "Ingress",
		Protocol:  consts.ProtocolTCP,
		Port:      53,
		NodeGroup: "worker",
		Service:   "dns-default",
		Namespace: "openshift-dns",
		Pod:       "dns-default",
		Container: "dns",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  consts.ProtocolUDP,
		Port:      53,
		NodeGroup: "worker",
		Service:   "dns-default",
		Namespace: "openshift-dns",
		Pod:       "dns-default",
		Container: "dns",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  consts.ProtocolTCP,
		Port:      18080,
		NodeGroup: "worker",
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
		Protocol:  consts.ProtocolTCP,
		Port:      53,
		NodeGroup: "master",
		Service:   "dns-default",
		Namespace: "openshift-dns",
		Pod:       "dns-default",
		Container: "dns",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  consts.ProtocolUDP,
		Port:      53,
		NodeGroup: "master",
		Service:   "dns-default",
		Namespace: "openshift-dns",
		Pod:       "dns-default",
		Container: "dns",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  consts.ProtocolTCP,
		Port:      9444,
		NodeGroup: "master",
		Service:   "",
		Namespace: "openshift-kni-infra",
		Pod:       "haproxy",
		Container: "haproxy",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  consts.ProtocolTCP,
		Port:      9445,
		NodeGroup: "master",
		Service:   "",
		Namespace: "openshift-kni-infra",
		Pod:       "haproxy",
		Container: "haproxy",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  consts.ProtocolTCP,
		Port:      9454,
		NodeGroup: "master",
		Service:   "",
		Namespace: "openshift-kni-infra",
		Pod:       "haproxy",
		Container: "haproxy",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  consts.ProtocolTCP,
		Port:      18080,
		NodeGroup: "master",
		Service:   "",
		Namespace: "openshift-kni-infra",
		Pod:       "coredns",
		Container: "coredns",
		Optional:  false,
	},
}

var NoneStaticEntriesWorker = []ComDetails{
	{
		Direction: "Ingress",
		Protocol:  consts.ProtocolTCP,
		Port:      53,
		NodeGroup: "worker",
		Service:   "dns-default",
		Namespace: "openshift-dns",
		Pod:       "dns-default",
		Container: "dns",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  consts.ProtocolUDP,
		Port:      53,
		NodeGroup: "worker",
		Service:   "dns-default",
		Namespace: "openshift-dns",
		Pod:       "dns-default",
		Container: "dns",
		Optional:  false,
	},
}

var NoneStaticEntriesMaster = []ComDetails{
	{
		Direction: "Ingress",
		Protocol:  consts.ProtocolTCP,
		Port:      53,
		NodeGroup: "master",
		Service:   "dns-default",
		Namespace: "openshift-dns",
		Pod:       "dns-default",
		Container: "dns",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  consts.ProtocolUDP,
		Port:      53,
		NodeGroup: "master",
		Service:   "dns-default",
		Namespace: "openshift-dns",
		Pod:       "dns-default",
		Container: "dns",
		Optional:  false,
	},
}

var StandardStaticEntries = []ComDetails{
	{
		Direction: "Ingress",
		Protocol:  consts.ProtocolUDP,
		Port:      6081,
		NodeGroup: "worker",
		Service:   "ovn-kubernetes geneve",
		Namespace: "openshift-ovn-kubernetes",
		Pod:       "",
		Container: "",
		Optional:  false,
	}, {
		Direction: "Ingress",
		Protocol:  consts.ProtocolUDP,
		Port:      6081,
		NodeGroup: "master",
		Service:   "ovn-kubernetes geneve",
		Namespace: "openshift-ovn-kubernetes",
		Pod:       "",
		Container: "",
		Optional:  false,
	},
}

// General IPv6-only static entries that should be applied when the cluster supports IPv6.
var GeneralIPv6StaticEntriesWorker = []ComDetails{
	{
		Direction: "Ingress",
		Protocol:  consts.ProtocolUDP,
		Port:      546,
		NodeGroup: "worker",
		Service:   "NetworkManager",
		Namespace: "",
		Pod:       "",
		Container: "",
		Optional:  false,
	},
}

var GeneralIPv6StaticEntriesMaster = []ComDetails{
	{
		Direction: "Ingress",
		Protocol:  consts.ProtocolUDP,
		Port:      546,
		NodeGroup: "master",
		Service:   "NetworkManager",
		Namespace: "",
		Pod:       "",
		Container: "",
		Optional:  false,
	},
}

// DHCP static entries that should be applied when the host uses DHCP for network configuration.
var GeneralDHCPStaticEntriesWorker = []ComDetails{
	{
		Direction: "Ingress",
		Protocol:  consts.ProtocolUDP,
		Port:      68,
		NodeGroup: "worker",
		Service:   "NetworkManager",
		Namespace: "",
		Pod:       "",
		Container: "",
		Optional:  false,
	},
}

var GeneralDHCPStaticEntriesMaster = []ComDetails{
	{
		Direction: "Ingress",
		Protocol:  consts.ProtocolUDP,
		Port:      68,
		NodeGroup: "master",
		Service:   "NetworkManager",
		Namespace: "",
		Pod:       "",
		Container: "",
		Optional:  false,
	},
}

var KubeletNodePortDefaultDynamicRange = DynamicRangeList{
	{
		Direction:   "Ingress",
		Protocol:    consts.ProtocolTCP,
		MinPort:     30000,
		MaxPort:     32767,
		Description: "Kubelet node ports",
		Optional:    true,
	},
	{
		Direction:   "Ingress",
		Protocol:    consts.ProtocolUDP,
		MinPort:     30000,
		MaxPort:     32767,
		Description: "Kubelet node ports",
		Optional:    true,
	},
}

// GetStaticEntries returns the static entries for the given platform, topology,
// IPv6 and DHCP configuration.
func GetStaticEntries(platformType configv1.PlatformType, topology configv1.TopologyMode, ipv6Enabled, dhcpEnabled bool) ([]ComDetails, error) {
	var comDetails []ComDetails

	switch platformType {
	case configv1.BareMetalPlatformType:
		comDetails = append(comDetails, BaremetalStaticEntriesMaster...)
		if topology != configv1.SingleReplicaTopologyMode {
			comDetails = append(comDetails, BaremetalStaticEntriesWorker...)
		}
	case configv1.NonePlatformType:
		comDetails = append(comDetails, NoneStaticEntriesMaster...)
		if topology != configv1.SingleReplicaTopologyMode {
			comDetails = append(comDetails, NoneStaticEntriesWorker...)
		}
	case configv1.AWSPlatformType:
		// No cloud-specific static entries
	default:
		return nil, fmt.Errorf("invalid value for cluster environment: %v", platformType)
	}

	comDetails = append(comDetails, GeneralStaticEntriesMaster...)
	if ipv6Enabled {
		comDetails = append(comDetails, GeneralIPv6StaticEntriesMaster...)
	}
	if dhcpEnabled {
		comDetails = append(comDetails, GeneralDHCPStaticEntriesMaster...)
	}
	if topology == configv1.SingleReplicaTopologyMode {
		return comDetails, nil
	}

	comDetails = append(comDetails, StandardStaticEntries...)
	comDetails = append(comDetails, GeneralStaticEntriesWorker...)
	if ipv6Enabled {
		comDetails = append(comDetails, GeneralIPv6StaticEntriesWorker...)
	}
	if dhcpEnabled {
		comDetails = append(comDetails, GeneralDHCPStaticEntriesWorker...)
	}

	return comDetails, nil
}
