package nodes

import (
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/liornoy/node-comm-lib/internal/consts"
)

func GetRoles(node *corev1.Node) string {
	res := make([]string, 0)
	validRoles := map[string]bool{"worker": true, "master": true}

	for label := range node.Labels {
		if after, found := strings.CutPrefix(label, consts.RoleLabel); found {
			if !validRoles[after] {
				continue
			}

			res = append(res, after)
		}
	}

	return strings.Join(res, ",")
}
