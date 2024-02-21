The custom Communcation Matrix entries should be in the following JSON foramt:
```
[
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": 18080,
    "nodeRole": "worker",
    "serviceName": "openshift-kni-infra-coredns",
    "required": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": 53,
    "nodeRole": "worker",
    "serviceName": "openshift-kni-infra-coredns",
    "required": true
  }
]
```
