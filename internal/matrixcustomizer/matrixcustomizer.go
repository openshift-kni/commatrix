package matrixcustomizer

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/liornoy/node-comm-lib/internal/types"
)

func AddFromFile(fp string) ([]types.ComDetails, error) {
	var res []types.ComDetails
	f, err := os.Open(filepath.Clean(fp))
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %v", fp, err)
	}
	defer f.Close()
	raw, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %v", fp, err)
	}

	err = json.Unmarshal(raw, &res)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal custom entries file: %v", err)
	}

	return res, nil
}

func GetStaticCustomEntries() ([]types.ComDetails, error) {
	var res []types.ComDetails

	err := json.Unmarshal([]byte(staticCustomEntries), &res)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal static custom entries bytes: %v", err)
	}

	return res, nil
}

var staticCustomEntries = `
[
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "18080",
    "nodeRole": "worker",
    "serviceName": "openshift-kni-infra-coredns",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "53",
    "nodeRole": "worker",
    "serviceName": "openshift-kni-infra-coredns",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "34087",
    "nodeRole": "worker",
    "serviceName": "crio",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "443",
    "nodeRole": "worker",
    "serviceName": "openshift-ingress-router-default-59884fcc7b-tn4rk-router",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "22",
    "nodeRole": "worker",
    "serviceName": "sshd",
    "required": false
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "80",
    "nodeRole": "worker",
    "serviceName": "openshift-ingress-router-default",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "9642",
    "nodeRole": "worker",
    "serviceName": "openshift-ovn-kubernetes-ovnkube-sbdb",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "9641",
    "nodeRole": "worker",
    "serviceName": "openshift-ovn-kubernetes-ovnkube-nbdb",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "9100",
    "nodeRole": "worker",
    "serviceName": "openshift-monitoring-node-exporter-kube-rbac-proxy",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "10250",
    "nodeRole": "worker",
    "serviceName": "kubelet",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "9107",
    "nodeRole": "worker",
    "serviceName": "openshift-ovn-kubernetes-ovnkube-controller",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "111",
    "nodeRole": "worker",
    "serviceName": "rpcbind",
    "required": false
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "1936",
    "nodeRole": "worker",
    "serviceName": "openshift-ingress-router-default-router",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "10256",
    "nodeRole": "worker",
    "serviceName": "openshift-ovn-kubernetes-ovnkube-ovnkube-controller",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "9105",
    "nodeRole": "worker",
    "serviceName": "openshift-ovn-kubernetes-ovnkube-kube-rbac-proxy-ovn-metrics",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "9103",
    "nodeRole": "worker",
    "serviceName": "openshift-ovn-kubernetes-ovnkube-kube-rbac-proxy-node",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "9001",
    "nodeRole": "worker",
    "serviceName": "openshift-machine-config-operator-machine-config-daemon-kube-rbac-proxy",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "9537",
    "nodeRole": "worker",
    "serviceName": "crio",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "53",
    "nodeRole": "master",
    "serviceName": "openshift-kni-infra-coredns",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "18080",
    "nodeRole": "master",
    "serviceName": "openshift-kni-infra-coredns",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "10250",
    "nodeRole": "master",
    "serviceName": "kubelet",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "9641",
    "nodeRole": "master",
    "serviceName": "openshift-ovn-kubernetes-ovnkube",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "9642",
    "nodeRole": "master",
    "serviceName": "openshift-ovn-kubernetes-ovnkube",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "9100",
    "nodeRole": "master",
    "serviceName": "openshift-monitoring-node-exporter-kube-rbac-proxy",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "9107",
    "nodeRole": "master",
    "serviceName": "openshift-ovn-kubernetes-ovnkube-controller",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "111",
    "nodeRole": "master",
    "serviceName": "rpcbind",
    "required": false
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "22",
    "nodeRole": "master",
    "serviceName": "sshd",
    "required": false
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "9001",
    "nodeRole": "master",
    "serviceName": "openshift-machine-config-operator-machine-config-daemon-kube-rbac-proxy",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "9191",
    "nodeRole": "master",
    "serviceName": "openshift-cluster-machine-approver-machine-approver-machine-approver-controller",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "9192",
    "nodeRole": "master",
    "serviceName": "openshift-cluster-machine-approver-machine-approver-kube-rbac-proxy",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "9108",
    "nodeRole": "master",
    "serviceName": "openshift-ovn-kubernetes-ovnkube-control-plane-kube-rbac-proxy",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "9105",
    "nodeRole": "master",
    "serviceName": "openshift-ovn-kubernetes-ovnkube-kube-rbac-proxy-ovn-metrics",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "9103",
    "nodeRole": "master",
    "serviceName": "openshift-ovn-kubernetes-ovnkube-kube-rbac-proxy-node",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "9099",
    "nodeRole": "master",
    "serviceName": "openshift-cluster-version-cluster-version-operator-cluster-version-operator",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "9980",
    "nodeRole": "master",
    "serviceName": "openshift-etcd-etcd-readyz",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "9979",
    "nodeRole": "master",
    "serviceName": "openshift-etcd-etcd-metrics",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "9978",
    "nodeRole": "master",
    "serviceName": "openshift-etcd-etcd-etcd",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "9445",
    "nodeRole": "master",
    "serviceName": "openshift-kni-infra-haproxy-haproxy",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "9444",
    "nodeRole": "master",
    "serviceName": "openshift-kni-infra-haproxy-haproxy",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "9537",
    "nodeRole": "master",
    "serviceName": "crio",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "10357",
    "nodeRole": "master",
    "serviceName": "openshift-kube-controller-manager-kube-controller-manager-cluster-policy-controller",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "10257",
    "nodeRole": "master",
    "serviceName": "openshift-kube-controller-manager-kube-controller-manager-kube-controller-manager",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "10256",
    "nodeRole": "master",
    "serviceName": "openshift-ovn-kubernetes-ovnkube-ovnkube-controller",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "10259",
    "nodeRole": "master",
    "serviceName": "openshift-kube-scheduler-openshift-kube-scheduler-kube-scheduler",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "17697",
    "nodeRole": "master",
    "serviceName": "openshift-kube-apiserver-kube-apiserve\u05e8-kube-apiserver-check-endpoints",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "2380",
    "nodeRole": "master",
    "serviceName": "openshift-etcd-etcd-etcd",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "2379",
    "nodeRole": "master",
    "serviceName": "openshift-etcd-etcd-etcd",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "5050",
    "nodeRole": "master",
    "serviceName": "openshift-machine-api-ironic-proxy-6gn2n-ironic-proxy",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "6080",
    "nodeRole": "master",
    "serviceName": "openshift-kube-apiserver-kube-apiserver-kube-apiserver-insecure-readyz",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "22624",
    "nodeRole": "master",
    "serviceName": "openshift-machine-config-operator-machine-config-server-machine-config-server",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "22623",
    "nodeRole": "master",
    "serviceName": "openshift-machine-config-operator-machine-config-server-machine-config-server",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "29445",
    "nodeRole": "master",
    "serviceName": "openshift-kni-infra-haproxy-haproxy",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "6385",
    "nodeRole": "master",
    "serviceName": "openshift-machine-api-ironic-proxy--ironic-proxy",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": "6443",
    "nodeRole": "master",
    "serviceName": "openshift-kube-apiserver-kube-apiserver-kube-apiserver",
    "required": true
  }
]
`
