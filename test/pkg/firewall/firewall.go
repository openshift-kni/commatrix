package firewall

import (
	"fmt"
	"path/filepath"

	"github.com/openshift-kni/commatrix/pkg/client"
	"github.com/openshift-kni/commatrix/pkg/firewall"
	"github.com/openshift-kni/commatrix/pkg/utils"
)

func CreateMachineConfig(c *client.ClientSet, NFTtable []byte, artifactsDir, nodePool string,
	utilsHelpers utils.UtilsInterface) (machineConfig []byte, err error) {
	machineConfig, err = firewall.NFTablesToMachineConfig(NFTtable, nodePool)
	if err != nil {
		return nil, fmt.Errorf("failed to convert nftables to MachineConfig: %v", err)
	}

	fileName := fmt.Sprintf("mc-%s.yaml", nodePool)
	filePath := filepath.Join(artifactsDir, fileName)
	err = utilsHelpers.WriteFile(filePath, machineConfig)
	if err != nil {
		return nil, err
	}

	return machineConfig, nil
}
