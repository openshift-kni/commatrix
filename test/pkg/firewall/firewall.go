package firewall

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"

	butaneConfig "github.com/coreos/butane/config"
	"github.com/coreos/butane/config/common"
	"github.com/openshift-kni/commatrix/pkg/consts"
	"github.com/openshift-kni/commatrix/pkg/utils"
)

// Apply the firewall rules on the node.
func ApplyRulesToNode(NFTtable []byte, nodeName, namespace, artifactsDir string, utilsHelpers utils.UtilsInterface) error {
	debugPod, err := utilsHelpers.CreatePodOnNode(nodeName, namespace, consts.DefaultDebugPodImage)
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

func MachineconfigWay(NFTtable []byte, artifactsDir, nodeRolde string, utilsHelpers utils.UtilsInterface) error {
	fmt.Println("MachineConfiguration way!")

	output, err := createButaneConfig(string(NFTtable), nodeRolde)
	if err != nil {
		return fmt.Errorf("failed to create Butane Config on nodeRole: %s %s: ", nodeRolde, err)
	}
	fileName := fmt.Sprintf("bu-%s.bu", nodeRolde)
	fmt.Printf("fileName the bu MachineConfiguration %s", fileName)

	filePath := filepath.Join(artifactsDir, fileName)

	fmt.Printf("write the bu MachineConfiguration %s", filePath)

	err = utilsHelpers.WriteFile(filePath, output)
	if err != nil {
		return err
	}
	fmt.Println("suc write the bu MachineConfiguration ")

	output, err = ConvertButaneToYAML(output)
	if err != nil {
		return fmt.Errorf("failed to list NFT ruleset on node %s: ", err)
	}
	fmt.Println("convert the bu MachineConfiguration ")
	fileName = fmt.Sprintf("bu-%s.yaml", nodeRolde)
	filePath = filepath.Join(artifactsDir, fileName)
	err = utilsHelpers.WriteFile(filePath, output)
	if err != nil {
		return err
	}
	fmt.Println("write the yaml MachineConfiguration ")

	versionMajorMinor, err := utilsHelpers.GetClusterVersiona()
	if err != nil {
		return err
	}

	if isVersionGreaterThan(versionMajorMinor, "4.16") {
		if err = updateMachineConfiguration(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return err
		}
	}
	fmt.Println("apply the yaml MachineConfiguration ")

	if err = applyYAMLWithOC(filePath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return err
	}

	return nil
}

// isVersionGreaterThan compares two version strings (major.minor) and returns true if v1 > v2.
func isVersionGreaterThan(v1, v2 string) bool {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	// Convert string parts to integers for comparison
	for i := 0; i < 2; i++ {
		if len(parts1) > i && len(parts2) > i {
			if parts1[i] > parts2[i] {
				return true
			} else if parts1[i] < parts2[i] {
				return false
			}
		} else if len(parts1) > i {
			// If v2 has no more parts, v1 is greater
			return true
		}
	}

	// If they are equal or v1 is less than v2
	return false
}

func createButaneConfig(nftablesRules, nodeRole string) ([]byte, error) {
	lines := strings.Split(nftablesRules, "\n")
	nftablesRulesWithoutFirstLine := ""
	if len(lines) > 1 {
		nftablesRulesWithoutFirstLine = strings.Join(lines[1:], "\n")
	}
	//nftablesRulesWithoutFirstLine = strings.ReplaceAll(nftablesRulesWithoutFirstLine, "\t", "  ")

	butaneConfig := fmt.Sprintf(`variant: openshift
version: 4.16.0
metadata:
  name: 98-nftables-cnf-%s
  labels:
    machineconfiguration.openshift.io/role: %s
systemd:
  units:
    - name: "nftables.service"
      enabled: true
      contents: |
        [Unit]
        Description=Netfilter Tables
        Documentation=man:nft(8)
        Wants=network-pre.target
        Before=network-pre.target
        [Service]
        Type=oneshot
        ProtectSystem=full
        ProtectHome=true
        ExecStart=/sbin/nft -f /etc/sysconfig/nftables.conf
        ExecReload=/sbin/nft -f /etc/sysconfig/nftables.conf
        ExecStop=/sbin/nft 'add table inet openshift_filter; delete table inet openshift_filter'
        RemainAfterExit=yes
        [Install]
        WantedBy=multi-user.target
storage:
  files:
    - path: /etc/sysconfig/nftables.conf
      mode: 0600
      overwrite: true
      contents:
        inline: |
          table inet openshift_filter
          delete table inet openshift_filter
		%s
        `, nodeRole, nodeRole, nftablesRulesWithoutFirstLine)
	butaneConfig = strings.ReplaceAll(butaneConfig, "\t", "  ")
	return []byte(butaneConfig), nil
}

// Function to convert Butane config to YAML using API
/*func convertButaneToYAML(butaneContent []byte) ([]byte, error) {
	// Example API endpoint (replace with actual Butane conversion API)
	apiURL := "https://butane-api.example.com/convert"

	// Create the POST request
	resp, err := http.Post(apiURL, "application/x-yaml", bytes.NewBuffer(butaneContent))
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body
	yamlContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: %s", string(yamlContent))
	}

	return yamlContent, nil
}*/

func fail(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
	os.Exit(1)
}

func ConvertButaneToYAML(butaneContent []byte) ([]byte, error) {
	options := common.TranslateBytesOptions{}

	dataOut, _, err := butaneConfig.TranslateBytes(butaneContent, options)
	if err != nil {
		fail("failed to  : %v\n", err)
	}

	return dataOut, nil
}

func applyYAMLWithOC(filePath string) error {
	// Construct the oc apply command
	cmd := exec.Command("/usr/local/bin/oc", "apply", "-f", filePath)

	// Set the command's output to be the same as the program's output
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run the oc apply command
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to apply YAML file using oc: %w", err)
	}

	fmt.Println("YAML file applied successfully with oc!")
	return nil
}

func updateMachineConfiguration() error {
	// Define the command to edit the MachineConfiguration
	cmd := exec.Command("/usr/local/bin/oc", "patch", "MachineConfiguration", "cluster", "-n", "openshift-machine-config-operator", "--type", "merge", "-p", `
spec:
  logLevel: Normal
  managementState: Managed
  operatorLogLevel: Normal
  nodeDisruptionPolicy:
    files:
    - actions:
      - restart:
          serviceName: nftables.service
        type: Restart
      path: /etc/sysconfig/nftables.conf
    units:
    - actions:
      - type: DaemonReload
      - type: Reload
        reload:
          serviceName: nftables.service
      name: nftables.service
`)

	// Set up buffers to capture standard output and standard error
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute the command
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to update MachineConfiguration: %v, stderr: %s", err, stderr.String())
	}

	fmt.Println("MachineConfiguration updated successfully!")
	return nil
}
