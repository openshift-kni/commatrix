package types

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/liornoy/node-comm-lib/internal/consts"
	"github.com/liornoy/node-comm-lib/internal/nftables"
)

type ComMatrix struct {
	Matrix []ComDetails
}

type ComDetails struct {
	Direction   string `json:"direction"`
	Protocol    string `json:"protocol"`
	Port        string `json:"port"`
	NodeRole    string `json:"nodeRole"`
	ServiceName string `json:"serviceName"`
	Required    bool   `json:"required"`
}

func (m *ComMatrix) ToCSV() ([]byte, error) {
	out := make([]byte, 0)
	w := bytes.NewBuffer(out)
	csvwriter := csv.NewWriter(w)

	for _, cd := range m.Matrix {
		record := strings.Split(cd.String(), ",")
		err := csvwriter.Write(record)
		if err != nil {
			return nil, fmt.Errorf("failed to convert to CSV foramt: %w", err)
		}
	}
	csvwriter.Flush()

	return w.Bytes(), nil
}

func (m *ComMatrix) ToJSON() ([]byte, error) {
	out, err := json.Marshal(m.Matrix)
	if err != nil {
		return nil, err
	}

	return out, nil
}

func (m *ComMatrix) ToNftables() ([]byte, error) {
	var res bytes.Buffer
	data := nftables.Data{
		AllowedTCPPorts: make([]string, 0),
		AllowedUDPPorts: make([]string, 0),
	}

	for _, cd := range m.Matrix {
		if cd.Protocol == "TCP" {
			data.AllowedTCPPorts = append(data.AllowedTCPPorts, cd.Port)
		}
		if cd.Protocol == "UDP" {
			data.AllowedUDPPorts = append(data.AllowedUDPPorts, cd.Port)
		}
	}

	tmpl, err := template.New("nftablesTemplate").Parse(nftables.Template)
	if err != nil {
		return nil, err
	}

	err = tmpl.Execute(&res, data)
	if err != nil {
		return nil, err
	}

	return res.Bytes(), nil
}

func (m *ComMatrix) String() string {
	var result strings.Builder
	for _, details := range m.Matrix {
		result.WriteString(details.String() + "\n")
	}

	return result.String()
}

func (cd ComDetails) String() string {
	return fmt.Sprintf("%s,%s,%s,%s,%s,%v", cd.Direction, cd.Protocol, cd.Port, cd.NodeRole, cd.ServiceName, cd.Required)
}

func (cd ComDetails) ToEndpointSlice(endpointSliceName string, namespace string, nodeName string, labels map[string]string, port int) discoveryv1.EndpointSlice {
	endpointSlice := discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      endpointSliceName,
			Namespace: namespace,
			Labels:    labels,
		},
		Ports: []discoveryv1.EndpointPort{
			{
				Port:     ptr.To[int32](int32(port)),
				Protocol: (*corev1.Protocol)(&cd.Protocol),
			},
		},
		Endpoints: []discoveryv1.Endpoint{
			{
				NodeName:  ptr.To(nodeName),
				Addresses: []string{consts.PlaceHolderIPAddress},
			},
		},
		AddressType: consts.DefaultAddressType,
	}

	return endpointSlice
}
