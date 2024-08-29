package firewall

import (
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"

	"github.com/openshift-kni/commatrix/pkg/consts"
	"github.com/openshift-kni/commatrix/pkg/utils"
)

// Apply the firewall rules on the node.
func ApplyRulesToNode(NFTtable []byte, nodeName, artifactsDir string, utilsHelpers utils.UtilsInterface) error {
	debugPod, err := utilsHelpers.CreatePodOnNode(nodeName, consts.DefaultDebugNamespace, consts.DefaultDebugPodImage)
	if err != nil {
		return fmt.Errorf("failed to create debug pod on node %s: %w", nodeName, err)
	}

	defer func() {
		err := utilsHelpers.DeletePod(debugPod)
		if err != nil {
			log.Printf("failed cleaning debug pod %s: %v", debugPod, err)
		}
	}()

	// save the nftables on  /host/etc/nftables/firewall.nft
	_, err = RunRootCommandOnPod(debugPod, fmt.Sprintf("echo '%s' > /host/etc/nftables/firewall.nft", string(NFTtable)), false, utilsHelpers)
	if err != nil {
		return fmt.Errorf("failed to save rule set to file on node %s: %w", nodeName, err)
	}

	// run apply nft command for the rules
	_, err = RunRootCommandOnPod(debugPod, "nft -f /etc/nftables/firewall.nft", true, utilsHelpers)
	if err != nil {
		return fmt.Errorf("failed to apply rule set on node %s: %w", nodeName, err)
	}

	_, err = NftListAndWriteToFile(debugPod, utilsHelpers, artifactsDir, "nftables-"+nodeName)
	if err != nil {
		return err
	}

	// edit the sys conf to make sure it will be on the list after reboot
	err = editNftablesConf(debugPod, utilsHelpers)
	if err != nil {
		return err
	}

	_, err = RunRootCommandOnPod(debugPod, "/usr/sbin/nft list ruleset > /etc/nftables.conf", true, utilsHelpers)
	if err != nil {
		return fmt.Errorf("failed to save NFT ruleset to file on node %s: %w", nodeName, err)
	}

	_, err = RunRootCommandOnPod(debugPod, "systemctl enable nftables", true, utilsHelpers)
	if err != nil {
		return fmt.Errorf("failed to enable nftables on node %s: %w", nodeName, err)
	}

	return nil
}

func RunRootCommandOnPod(debugPod *v1.Pod, command string, chrootDir bool, utilsHelpers utils.UtilsInterface) ([]byte, error) {
	if chrootDir {
		// Format the command for chroot
		command = fmt.Sprintf("chroot /host /bin/bash -c '%s'", command)
	}

	output, err := utilsHelpers.RunCommandOnPod(debugPod, []string{"bash", "-c", command})
	if err != nil {
		return nil, fmt.Errorf("failed to execute command '%s' on node %s: %w", command, debugPod.Spec.NodeName, err)
	}
	return output, nil
}

// Checks if the include statement for the new nftables configuration,
// Is present in the nftables.conf file. If the include statement is not present,
// Add it the statement to the file.
func editNftablesConf(debugPod *v1.Pod, utilsHelpers utils.UtilsInterface) error {
	checkCommand := `grep 'include "/etc/nftables/firewall.nft"' /host/etc/sysconfig/nftables.conf | wc -l`
	output, err := RunRootCommandOnPod(debugPod, checkCommand, false, utilsHelpers)
	if err != nil {
		return fmt.Errorf("failed to check nftables.conf on debug pod: %w", err)
	}

	// Convert output to an integer and check if the include statement exists
	includeCount, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return fmt.Errorf("failed to parse grep output: %w", err)
	}

	if includeCount > 0 {
		log.Println("Include statement already exists, no need to add it")
		return nil
	}

	addCommand := `echo 'include "/etc/nftables/firewall.nft"' >> /host/etc/sysconfig/nftables.conf`
	_, err = RunRootCommandOnPod(debugPod, addCommand, false, utilsHelpers)
	if err != nil {
		return fmt.Errorf("failed to edit nftables.conf on debug pod: %w", err)
	}

	return nil
}

func NftListAndWriteToFile(debugPod *v1.Pod, utilsHelpers utils.UtilsInterface, artifactsDir, fileName string) ([]byte, error) {
	command := "nft list ruleset"
	output, err := RunRootCommandOnPod(debugPod, command, true, utilsHelpers)
	if err != nil {
		return nil, fmt.Errorf("failed to list NFT ruleset on node %s: %w", debugPod.Spec.NodeName, err)
	}

	if len(output) == 0 {
		return nil, fmt.Errorf("no nft rules on node %s: ", debugPod.Spec.NodeName)
	}

	err = utilsHelpers.WriteFile(filepath.Join(artifactsDir, fileName), output)
	if err != nil {
		return nil, err
	}

	return output, nil
}
