package firewall

// NodeDisruptionPolicyPatch is a YAML patch for the MachineConfiguration CR
// ("cluster") that adds a nodeDisruptionPolicy. With this policy, updating the
// nftables rules file or the nftables.service unit will reload/restart the
// service instead of requiring a full node reboot.
//
// Apply with:
//
//	oc patch machineconfiguration cluster --type=merge --patch-file=node-disruption-policy.yaml
const NodeDisruptionPolicyPatch = `apiVersion: operator.openshift.io/v1
kind: MachineConfiguration
metadata:
  name: cluster
spec:
  nodeDisruptionPolicy:
    units:
      - name: nftables.service
        actions:
          - type: Reload
            reload:
              serviceName: nftables.service
    files:
      - path: /etc/sysconfig/nftables.conf
        actions:
          - type: Restart
            restart:
              serviceName: nftables.service
`
