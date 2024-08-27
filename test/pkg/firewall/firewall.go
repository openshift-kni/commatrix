package firewall

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	v1 "k8s.io/api/core/v1"

	"github.com/openshift-kni/commatrix/pkg/consts"
	"github.com/openshift-kni/commatrix/pkg/utils"
)

// Apply the firewall rules on the node.
func ApplyRulesToNode(NFTtable []byte, nodeName string, utilsHelpers utils.UtilsInterface) error {
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
	_, err = RunCommandInPod(debugPod, fmt.Sprintf("echo '%s' > /host/etc/nftables/firewall.nft", string(NFTtable)), false, utilsHelpers)
	if err != nil {
		return fmt.Errorf("failed to save rule set to file on node %s: %w", nodeName, err)
	}

	// run apply nft command for the rules
	_, err = RunCommandInPod(debugPod, "nft -f /etc/nftables/firewall.nft", true, utilsHelpers)
	if err != nil {
		return fmt.Errorf("failed to apply rule set on node %s: %w", nodeName, err)
	}

	output, err := nftList(debugPod, utilsHelpers)
	if err != nil {
		return err
	}

	// edit the sys conf to make sure it will be on the list after reboot
	err = editNftablesConf(debugPod, utilsHelpers)
	if err != nil {
		return err
	}

	_, err = RunCommandInPod(debugPod, "/usr/sbin/nft list ruleset > /etc/nftables.conf", true, utilsHelpers)
	if err != nil {
		return fmt.Errorf("failed to save NFT ruleset to file on node %s: %w", nodeName, err)
	}

	_, err = RunCommandInPod(debugPod, "systemctl enable nftables", true, utilsHelpers)
	if err != nil {
		return fmt.Errorf("failed to enable nftables on node %s: %w", nodeName, err)
	}

	err = saveNFTablesRules("nftables-"+nodeName, string(output))
	if err != nil {
		return err
	}

	return nil
}

func RulesList(nodeName string, utilsHelpers utils.UtilsInterface) ([]byte, error) {
	debugPod, err := utilsHelpers.CreatePodOnNode(nodeName, consts.DefaultDebugNamespace, consts.DefaultDebugPodImage)
	if err != nil {
		return nil, fmt.Errorf("failed to create debug pod on node %s: %w", nodeName, err)
	}

	defer func() {
		err := utilsHelpers.DeletePod(debugPod)
		if err != nil {
			log.Printf("failed cleaning debug pod %s: %v", debugPod, err)
		}
	}()

	output, err := nftList(debugPod, utilsHelpers)
	if err != nil {
		return nil, err
	}

	err = saveNFTablesRules("nftables-after-reboot"+nodeName, string(output))
	if err != nil {
		return nil, err
	}

	return output, nil
}

func RunCommandInPod(debugPod *v1.Pod, command string, chrootDir bool, utilsHelpers utils.UtilsInterface) ([]byte, error) {
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
	checkCommand := `grep -q 'include "/etc/nftables/firewall.nft"' /host/etc/sysconfig/nftables.conf`
	_, err := RunCommandInPod(debugPod, checkCommand, false, utilsHelpers)
	if err == nil {
		log.Println("Include statement already exists, no need to add it")
		return nil
	}

	addCommand := `echo 'include "/etc/nftables/firewall.nft"' >> /host/etc/sysconfig/nftables.conf`
	_, err = RunCommandInPod(debugPod, addCommand, false, utilsHelpers)
	if err != nil {
		return fmt.Errorf("failed to edit nftables.conf on debug pod: %w", err)
	}

	return nil
}

// For our test save the nftables to see before and after reboot.
func saveNFTablesRules(fileName, content string) error {
	artifactsDir, ok := os.LookupEnv("ARTIFACT_DIR")
	if !ok {
		log.Println("env var ARTIFACT_DIR is not set")
	}

	folderPath := filepath.Join(artifactsDir, "commatrix")
	err := os.MkdirAll(folderPath, 0755)
	if err != nil {
		return fmt.Errorf("failed to create folder %s: %w", folderPath, err)
	}

	filePath := filepath.Join(folderPath, fileName)
	err = os.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		return fmt.Errorf("failed to save file %s: %w", filePath, err)
	}

	return nil
}

func nftList(debugPod *v1.Pod, utilsHelpers utils.UtilsInterface) ([]byte, error) {
	command := "nft list ruleset"
	output, err := RunCommandInPod(debugPod, command, true, utilsHelpers)
	if err != nil {
		return nil, fmt.Errorf("failed to list NFT ruleset on node %s: %w", debugPod.Spec.NodeName, err)
	}

	if len(output) == 0 {
		return nil, fmt.Errorf("no nft rules on node %s: ", debugPod.Spec.NodeName)
	}

	return output, nil
}
