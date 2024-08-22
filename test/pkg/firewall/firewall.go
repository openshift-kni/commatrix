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

	// save the nftabes on  /host/etc/nftables/firewall.nft
	_, err = runBashCommandOnPod(debugPod, fmt.Sprintf("echo '%s' > /host/etc/nftables/firewall.nft", string(NFTtable)), false, utilsHelpers)
	if err != nil {
		return fmt.Errorf("failed to save rule set to file on node %s: %w", nodeName, err)
	}

	// run apply nft command for the rules
	_, err = runBashCommandOnPod(debugPod, "nft -f /etc/nftables/firewall.nft", true, utilsHelpers)
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

	_, err = runBashCommandOnPod(debugPod, "/usr/sbin/nft list ruleset > /etc/nftables.conf", true, utilsHelpers)
	if err != nil {
		return fmt.Errorf("failed to save NFT ruleset to file on node %s: %w", nodeName, err)
	}

	_, err = runBashCommandOnPod(debugPod, "systemctl enable nftables", true, utilsHelpers)
	if err != nil {
		return fmt.Errorf("failed to enable nftables on node %s: %w", nodeName, err)
	}

	err = saveNFTablesRules("commatrix", "nftables-"+nodeName, string(output))
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

	err = saveNFTablesRules("commatrix-after-reboot", "nftables-"+nodeName, string(output))
	if err != nil {
		return nil, err
	}

	return output, nil
}

func runBashCommandOnPod(debugPod *v1.Pod, command string, chrootDir bool, utilsHelpers utils.UtilsInterface) ([]byte, error) {
	var fullCommand string
	if chrootDir {
		// Format the command for chroot
		fullCommand = fmt.Sprintf("chroot %s /bin/bash -c '%s'", "/host", command)
	} else {
		fullCommand = command
	}

	output, err := utilsHelpers.RunCommandOnPod(debugPod, []string{"bash", "-c", fullCommand})
	if err != nil {
		return nil, fmt.Errorf("failed to execute command '%s' on node %s: %w", command, debugPod.Spec.NodeName, err)
	}
	return output, nil
}

// Make sure that the icnlude of the new nftables is not there.
// Then if it not there just add that include.
func editNftablesConf(debugPod *v1.Pod, utilsHelpers utils.UtilsInterface) error {
	checkCommand := `grep -q 'include "/etc/nftables/firewall.nft"' /host/etc/sysconfig/nftables.conf`
	_, err := runBashCommandOnPod(debugPod, checkCommand, false, utilsHelpers)
	if err == nil {
		log.Println("Include statement already exists, no need to add it")
		return nil
	}

	addCommand := `echo 'include "/etc/nftables/firewall.nft"' >> /host/etc/sysconfig/nftables.conf`
	_, err = runBashCommandOnPod(debugPod, addCommand, false, utilsHelpers)
	if err != nil {
		return fmt.Errorf("failed to edit nftables.conf on debug pod: %w", err)
	}

	return nil
}

// For our test save the nftables to see before and after reboot.
func saveNFTablesRules(folderName, fileName, content string) error {
	artifactsDir, ok := os.LookupEnv("ARTIFACT_DIR")
	if !ok {
		log.Println("env var ARTIFACT_DIR is not set")
	}

	folderPath := filepath.Join(artifactsDir, folderName)
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
	output, err := runBashCommandOnPod(debugPod, command, true, utilsHelpers)
	if err != nil {
		return nil, fmt.Errorf("failed to list NFT ruleset on node %s: %w", debugPod.Spec.NodeName, err)
	}

	if len(output) == 0 {
		return nil, fmt.Errorf("no output from 'nft list ruleset' on node %s: ", debugPod.Spec.NodeName)
	}

	return output, nil
}
