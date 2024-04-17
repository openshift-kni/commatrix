package nftables

import (
	"bytes"
	"html/template"

	"github.com/liornoy/node-comm-lib/pkg/commatrix"
)

type NftablesData struct {
	AllowedTCPPorts []string
	AllowedUDPPorts []string
}

const nftablesTemplate = `#!/usr/sbin/nft -f

table ip my_filter {
    chain input {
        type filter hook input priority 0; policy drop;

        iifname "lo" accept;

        # Hard-coded rule to allow SSH traffic for safety
        tcp dport 22 accept;
		
		{{if (len .AllowedTCPPorts) gt 0}}
        tcp dport { {{range .AllowedTCPPorts}}{{.}}, {{end}} } accept;
		{{end}}

		{{if (len .AllowedUDPPorts) gt 0}}
        udp dport { {{range .AllowedUDPPorts}}{{.}}, {{end}} } accept;
		{{end}}
    }
}
`

func GetRulesFromCommDetails(cds []commatrix.ComDetails) (string, error) {
	var (
		nftablesContent bytes.Buffer
		data            = NftablesData{
			AllowedTCPPorts: make([]string, 0),
			AllowedUDPPorts: make([]string, 0),
		}
	)

	for _, cd := range cds {
		if cd.Protocol == "TCP" {
			data.AllowedTCPPorts = append(data.AllowedTCPPorts, cd.Port)
		}
		if cd.Protocol == "UDP" {
			data.AllowedUDPPorts = append(data.AllowedUDPPorts, cd.Port)
		}
	}

	tmpl, err := template.New("nftablesTemplate").Parse(nftablesTemplate)
	if err != nil {
		return "", err
	}

	err = tmpl.Execute(&nftablesContent, data)
	if err != nil {
		return "", err
	}

	return nftablesContent.String(), nil
}
