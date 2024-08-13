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

type Firewall struct {
	ns           string
	utilsHelpers utils.UtilsInterface
}

func New(ns string, utilsHelpers utils.UtilsInterface) *Firewall {
	return &Firewall{
		ns:           ns,
		utilsHelpers: utilsHelpers,
	}
}

func (firewall *Firewall) runBashCommandOnPod(debugPod *v1.Pod, command string, chrootDir string) ([]byte, error) {
	var fullCommand string
	if chrootDir != "" {
		// Format the command for chroot
		fullCommand = fmt.Sprintf("chroot %s /bin/bash -c '%s'", chrootDir, command)
	} else {
		fullCommand = command
	}

	output, err := firewall.utilsHelpers.RunCommandOnPod(debugPod, []string{"bash", "-c", fullCommand})
	if err != nil {
		return nil, fmt.Errorf("failed to execute command '%s' on node %s: %w", command, debugPod.Spec.NodeName, err)
	}
	return output, nil
}

func (firewall *Firewall) ApplyRulesToNode(NFTtable []byte, nodeName string) error {
	debugPod, err := firewall.utilsHelpers.CreatePodOnNode(nodeName, firewall.ns, consts.DefaultDebugPodImage)
	if err != nil {
		return fmt.Errorf("failed to create debug pod on node %s: %w", nodeName, err)
	}

	defer func() {
		err := firewall.utilsHelpers.DeletePod(debugPod)
		if err != nil {
			log.Printf("failed cleaning debug pod %s: %v", debugPod, err)
		}
	}()

	_, err = firewall.runBashCommandOnPod(debugPod, fmt.Sprintf("echo '%s' > /host/etc/nftables/firewall.nft", string(NFTtable)), "")
	if err != nil {
		return fmt.Errorf("failed to save rule set to file on node %s: %w", nodeName, err)
	}

	_, err = firewall.runBashCommandOnPod(debugPod, "nft -f /etc/nftables/firewall.nft", "/host")
	if err != nil {
		return fmt.Errorf("failed to apply rule set on node %s: %w", nodeName, err)
	}

	output, err := firewall.nftList(debugPod)
	if err != nil {
		return err
	}

	err = firewall.editNftablesConf(debugPod)
	if err != nil {
		return err
	}

	_, err = firewall.runBashCommandOnPod(debugPod, "/usr/sbin/nft list ruleset > /etc/nftables.conf", "/host")
	if err != nil {
		return fmt.Errorf("failed to save NFT ruleset to file on node %s: %w", nodeName, err)
	}

	_, err = firewall.runBashCommandOnPod(debugPod, "systemctl enable nftables", "/host")
	if err != nil {
		return fmt.Errorf("failed to enable nftables on node %s: %w", nodeName, err)
	}

	err = saveNFTablesRules("commatrix", "nftables-"+nodeName, string(output))
	if err != nil {
		return err
	}

	return nil
}

func (firewall *Firewall) RulesList(nodeName string) ([]byte, error) {
	debugPod, err := firewall.utilsHelpers.CreatePodOnNode(nodeName, firewall.ns, consts.DefaultDebugPodImage)
	if err != nil {
		return nil, fmt.Errorf("failed to create debug pod on node %s: %w", nodeName, err)
	}

	defer func() {
		err := firewall.utilsHelpers.DeletePod(debugPod)
		if err != nil {
			log.Printf("failed cleaning debug pod %s: %v", debugPod, err)
		}
	}()

	output, err := firewall.nftList(debugPod)
	if err != nil {
		return nil, err
	}

	err = saveNFTablesRules("commatrix-after-reboot", "nftables-"+nodeName, string(output))
	if err != nil {
		return nil, err
	}

	return output, nil
}

func (firewall *Firewall) editNftablesConf(debugPod *v1.Pod) error {
	checkCommand := `grep -q 'include "/etc/nftables/firewall.nft"' /host/etc/sysconfig/nftables.conf`
	_, err := firewall.runBashCommandOnPod(debugPod, checkCommand, "")
	if err == nil {
		log.Println("Include statement already exists, no need to add it")
		return nil
	}

	addCommand := `echo 'include "/etc/nftables/firewall.nft"' >> /host/etc/sysconfig/nftables.conf`
	_, err = firewall.runBashCommandOnPod(debugPod, addCommand, "")
	if err != nil {
		return fmt.Errorf("failed to edit nftables.conf on debug pod: %w", err)
	}

	return nil
}

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

func (firewall *Firewall) nftList(debugPod *v1.Pod) ([]byte, error) {
	command := "nft list ruleset"
	output, err := firewall.runBashCommandOnPod(debugPod, command, "/host")
	if err != nil {
		return nil, fmt.Errorf("failed to list NFT ruleset on node %s: %w", debugPod.Spec.NodeName, err)
	}

	if len(output) == 0 {
		return nil, fmt.Errorf("no output from 'nft list ruleset' on node %s: ", debugPod.Spec.NodeName)
	}

	return output, nil
}
