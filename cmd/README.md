# `oc commatrix` Plugin
---

## Overview

The `oc commatrix` plugin including the `generate` command, which generates an up-to-date communication flows matrix for all ingress flows of OpenShift (multi-node and single-node deployments) and Operators.

By using the `generate` command, you can get the matrix of host open ports and the EndpointSlice matrix, as well as the difference between the two.

For additional details, please refer to the [commatrix documentation](https://github.com/openshift-kni/commatrix/blob/main/README.md)


## Installation

### Prerequisites

- OpenShift CLI (`oc`) installed and configured to access your cluster.
- Go installed for building the plugin, or download a pre-built binary (if available).

---

## Running
build the code locally and install it in `/usr/local/bin`:
```sh
$ make build
$ sudo make install

# you can now begin using this plugin as:
$ oc commatrix generate --host-open-ports
```

---

## Usage
```
Usage:
  oc commatrix generate [flags]

Flags:
      --customEntriesFormat string   Set the format of the custom entries file (json,yaml,csv)
      --customEntriesPath string     Add custom entries from a file to the matrix
      --debug                        Debug logs (default is false)
      --destDir string               Output files dir 
      --format string                Desired format (json,yaml,csv,nft) (default "csv")
      --host-open-ports              Generate communication matrices: EndpointSlice matrix, SS matrix, and their difference.
  ```


## Example Output

Once you run the `oc commatrix generate` command, the plugin will
generate a communication matrix based on the ingress flows in your
Kubernetes cluster. The output will be saved to a file in the chosen format,
similar to the following:

`csv example`
```sh
$ oc commatrix generate --format csv
Direction,Protocol,Port,Namespace,Service,Pod,Container,Node Role,Optional
Ingress,TCP,22,Host system service,sshd,,,master,true
Ingress,TCP,53,openshift-dns,dns-default,dnf-default,dns,master,false
Ingress,TCP,80,openshift-ingress,router-internal-default,router-default,router,master,false
Ingress,TCP,111,Host system service,rpcbind,,,master,true
```

`json example`
```sh
$ oc commatrix generate --format json
[
    {
        "direction": "Ingress",
        "protocol": "TCP",
        "port": 22,
        "namespace": "Host system service",
        "service": "sshd",
        "pod": "",
        "container": "",
        "nodeRole": "master",
        "optional": true
    },
    {
        "direction": "Ingress",
        "protocol": "TCP",
        "port": 53,
        "namespace": "openshift-dns",
        "service": "dns-default",
        "pod": "dnf-default",
        "container": "dns",
        "nodeRole": "master",
        "optional": false
    }
]
```