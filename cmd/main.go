package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/openshift-kni/commatrix/cmd/applyFirewall"
	"github.com/openshift-kni/commatrix/cmd/generateMatrix"
	"github.com/openshift-kni/commatrix/commatrix"
	"github.com/openshift-kni/commatrix/types"
)

var (
	destDir             string
	format              string
	envStr              string
	deploymentStr       string
	customEntriesPath   string
	customEntriesFormat string
	applyFirewallFlag   bool
	printFn             func(m types.ComMatrix) ([]byte, error)
)

func init() {
	flag.StringVar(&destDir, "destDir", "communication-matrix", "Output files dir")
	flag.StringVar(&format, "format", "csv", "Desired format (json,yaml,csv)")
	flag.StringVar(&envStr, "env", "baremetal", "Cluster environment (baremetal/aws)")
	flag.StringVar(&deploymentStr, "deployment", "mno", "Deployment type (mno/sno)")
	flag.StringVar(&customEntriesPath, "customEntriesPath", "", "Add custom entries from a file to the matrix")
	flag.StringVar(&customEntriesFormat, "customEntriesFormat", "", "Set the format of the custom entries file (json,yaml,csv)")
	flag.BoolVar(&applyFirewallFlag, "applyFirewall", false, "Apply firewall rules")

	flag.Parse()

	switch format {
	case "json":
		printFn = types.ToJSON
	case "csv":
		printFn = types.ToCSV
	case "yaml":
		printFn = types.ToYAML
	default:
		panic(fmt.Sprintf("invalid format: %s. Please specify json, csv, or yaml.", format))
	}
}

func main() {
	kubeconfig, ok := os.LookupEnv("KUBECONFIG")
	if !ok {
		panic("must set the KUBECONFIG environment variable")
	}

	var env commatrix.Env
	switch envStr {
	case "baremetal":
		env = commatrix.Baremetal
	case "aws":
		env = commatrix.AWS
	default:
		panic(fmt.Sprintf("invalid cluster environment: %s", envStr))
	}

	var deployment commatrix.Deployment
	switch deploymentStr {
	case "mno":
		deployment = commatrix.MNO
	case "sno":
		deployment = commatrix.SNO
	default:
		panic(fmt.Sprintf("invalid deployment type: %s", deploymentStr))
	}

	if customEntriesPath != "" && customEntriesFormat == "" {
		panic("error, variable customEntriesFormat is not set")
	}

	if applyFirewallFlag {
		applyFirewall.ApplyFirewallRules(kubeconfig, destDir, env, deployment)
	} else {
		generateMatrix.GeneratCommatrix(kubeconfig, customEntriesPath, customEntriesFormat, format, env, deployment, printFn, destDir)
	}
}
