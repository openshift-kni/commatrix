## commatrix

This project allows to automatically generate an accurate and up-to-date communication  
flows matrix that can be delivered to customers as part of product documentation for all  
ingress flows of OpenShift (multi-node and single-node deployments).

### Usage of the EndpointSlice Resource

This library leverages the EndpointSlice resource to identify the ports the  
cluster uses for ingress traffic. Relevant EndpointSlices include those  
referencing host-networked pods, Node Port services, and LoadBalancer services.

### Creating Custom ComDetails with ss Command

The `ss` command, a Linux utility, lists listening ports on  
the host with `ss -anplt` for TCP or `ss -anplu` for UDP.
For example, consider the following ss entry:
```
LISTEN 0      4096    127.0.0.1:10248 0.0.0.0:* users:(("kubelet",pid=6187,fd=20))
```

The `ss` package provides the `CreateComDetailsFromNode` function that runs
the `ss` command on each node, and converts the output into a corresponding ComDetails list.  

### Communication Matrix Creation Guide

Use the `generate` Makefile target to create the matrix.
Add additional entires to the matrix via a custom file, using
the variables `CUSTOM_ENTRIES_PATH` and `CUSTOM_ENTRIES_FORMAT`.
Examples are available in the `example-custom-entries` files.

The following environment variables are used to configure:
```
FORMAT (csv/json/yaml)
CLUSTER_ENV (baremetal/aws)
DEST_DIR (path to the directory containing the artifacts)
DEPLOYMENT (mno/sno)
CUSTOM_ENTRIES_PATH (path to the file containing custom entries to add to the matrix)
CUSTOM_ENTRIES_FORMAT (the format of the custom entries file (json,yaml,csv))
```

The generated artifcats are:
```
communication-matrix - The generated communication matrix.
ss-generated-matrix - The communication matrix that generated by the `ss` command.
matrix-diff-ss - Shows the variance between two matrices. Entries present in the communication matrix but absent in the ss matrix are marked with '+', while entries present in the ss matrix but not in the communication matrix are marked with '-'.
raw-ss-tcp - The raw `ss` output for TCP.
raw-ss-udp - The raw `ss` output for UDP.
nft-file-worker - worker NFTtable
nft-file-master - master NFTtable
```

Each record describes a flow with the following information:
```
direction      Data flow direction (currently ingress only)
protocol       IP protocol (TCP/UDP/SCTP/etc)
port           Flow port number
namespace      EndpointSlice Namespace
service        EndpointSlice owner Service name
pod            EndpointSlice target Pod name
container      Port owner Container name
nodeRole       Service node host role (master/worker/master&worker[for SNO])
optional       Optional or mandatory flow for OpenShift
```
