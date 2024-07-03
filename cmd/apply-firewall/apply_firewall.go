package applyFirewall

import (
	"fmt"

	clientutil "github.com/openshift-kni/commatrix/client"
	"github.com/openshift-kni/commatrix/commatrix"
	"github.com/openshift-kni/commatrix/consts"
	"github.com/openshift-kni/commatrix/debug"
)

func ApplyFirewallRules(kubeconfig, destDir string, env commatrix.Env, deployment commatrix.Deployment) {
	cs, err := clientutil.New(kubeconfig)
	if err != nil {
		panic(err)
	}

	mat, err := commatrix.New(kubeconfig, "", "", env, deployment)
	if err != nil {
		panic(fmt.Sprintf("failed to create the communication matrix: %s", err))
	}
	err = debug.CreateNamespace(cs, consts.DefaultDebugNamespace)
	if err != nil {
		panic(err)
	}
	defer func() {
		err := debug.DeleteNamespace(cs, consts.DefaultDebugNamespace)
		if err != nil {
			panic(err)
		}
	}()
	err = commatrix.ApplyFireWallRules(cs, mat, "master")
	if err != nil {
		panic(err)
	}
	if deployment == commatrix.MNO {
		err = commatrix.ApplyFireWallRules(cs, mat, "worker")
		if err != nil {
			panic(err)
		}
	}
}
