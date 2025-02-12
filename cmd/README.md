# `oc commatrix` Plugin
---

## Overview

The `oc commatrix generate` generates an up-to-date communication flows matrix for all ingress flows of OpenShift (multi-node and single-node deployments) and Operators.

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
$ oc commatrix generate
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
      --destDir string               Output files dir (default communication-matrix)
      --format string                Desired format (json,yaml,csv,nft) (default "csv")
      --host-open-ports              Generate communication matrix, host open port matrix, and their difference.
  ```


## Example Output

Once you run the `oc commatrix generate` command, the plugin will
generate a communication matrix based on the ingress flows in your
OpenShift cluster. The output will be saved to a file (destDir) in the chosen format,
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

`host-open-ports example command`
```sh
$ oc commatrix generate --host-open-ports --format csv
```

the command will generate the follwing paths:

`communication-matrix path`

```sh
Direction,Protocol,Port,Namespace,Service,Pod,Container,Node Role,Optional
Ingress,TCP,22,Host system service,sshd,,,master,true
Ingress,TCP,80,openshift-ingress,router-internal-default,router-default,router,master,false
Ingress,UDP,59975,,rpc.statd,,,master,false
```

`ss-generated-matrix path`

```sh
Direction,Protocol,Port,Namespace,Service,Pod,Container,Node Role,Optional
Ingress,TCP,22,,sshd,,,master,false
Ingress,TCP,80,,haproxy,,router,master,false
Ingress,TCP,111,Host system service,rpcbind,,,master,true
```

`matrix-diff-ss path`

```sh
Direction,Protocol,Port,Namespace,Service,Pod,Container,Node Role,Optional
Ingress,TCP,22,Host system service,sshd,,,master,true
Ingress,TCP,80,openshift-ingress,router-internal-default,router-default,router,master,false
- Ingress,TCP,111,Host system service,rpcbind,,,master,true
+ Ingress,UDP,59975,,rpc.statd,,,master,false
```

`raw-ss-tcp path`

```sh
node: clus0-0
LISTEN 0      4096    127.0.0.1:29103 0.0.0.0:* users:(("ovnkube",pid=10913,fd=8))
LISTEN 0      4096    127.0.0.1:29108 0.0.0.0:* users:(("ovnkube",pid=9764,fd=3))
LISTEN 0      4096    127.0.0.1:29105 0.0.0.0:* users:(("ovnkube",pid=10913,fd=7))
```

`raw-ss-udp path`

```sh
node: clus0-0
UNCONN 0      0      0.0.0.0:111   0.0.0.0:* users:(("rpcbind",pid=3919,fd=5),("systemd",pid=1,fd=169))
UNCONN 0      0      127.0.0.1:323   0.0.0.0:* users:(("chronyd",pid=2805,fd=5))
UNCONN 0      0      127.0.0.1:708   0.0.0.0:* users:(("rpc.statd",pid=3922,fd=8))
```

`customEntriesFormat and customEntriesPath example command`
```sh
$ oc commatrix generate --format csv --customEntriesFormat csv --customEntriesPath "communication-matrix/customEntriesPath"
```

`contents of communication-matrix/customEntriesPath`

```
Direction,Protocol,Port,Namespace,Service,Pod,Container,Node Role,Optional
ingress,TCP,9050,example-namespace,example-service,example-pod,example-container,master,false
ingress,UDP,9051,example-namespace2,example-service2,example-pod2,example-container2,worker,false
```

The command will generate the communication matrix, including the custom entries.
The output would look like this:

```
Direction,Protocol,Port,Namespace,Service,Pod,Container,Node Role,Optional
Ingress,TCP,22,Host system service,sshd,,,master,true
Ingress,TCP,53,openshift-dns,dns-default,dnf-default,dns,master,false
ingress,TCP,9050,example-namespace,example-service,example-pod,example-container,master,false
ingress,UDP,9051,example-namespace2,example-service2,example-pod2,example-container2,worker,false
```