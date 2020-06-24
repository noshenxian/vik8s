package glusterfs

import (
	"fmt"
	"github.com/ihaiker/vik8s/install/hosts"
	"github.com/ihaiker/vik8s/install/k8s"
	"github.com/ihaiker/vik8s/libs/ssh"
	"github.com/ihaiker/vik8s/libs/utils"
	"github.com/spf13/cobra"
	"os"
	"strings"
)

func glusterVolumes(master *ssh.Node) (volumes []string) {
	volumeStr := master.MustCmd2String(gluster+" volume list", true)
	if volumeStr != "No volumes present in cluster" {
		volumes = strings.Split(master.MustCmd2String(gluster+" volume list", true), "\n")
	}
	return volumes
}

func glusterMountPath(master *ssh.Node) string {
	return master.MustCmd2String(`kubectl -n glusterfs get daemonsets.apps glusterfs \
				-o go-template='{{range .spec.template.spec.volumes}}{{if eq .name "vik8s-glusterfs-volumes"}}{{.hostPath.path}}{{end}}{{end}}'`)
}

var volumeCmd = &cobra.Command{
	Use: "volume", Short: "create volume",
	Example: "vik8s storage glusterfs volume volumeName " +
		"[stripe <COUNT>] [[replica <COUNT> [arbiter <COUNT>]]|[replica 2 thin-arbiter 1]] [disperse [<COUNT>]] [disperse-data <COUNT>] [redundancy <COUNT>]",
	Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.MinimumNArgs(1)(cmd, args); err != nil {
			return err
		}
		return cobra.OnlyValidArgs(cmd, args[1:])
	},
	ValidArgs: []string{
		"stripe", "replica", "arbiter", "thin-arbiter",
		"disperse", "disperse-data", "redundancy",
		"2", "3", "4", "5",
	},
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		master := k8s.Config.Master()
		volumes := glusterVolumes(master)

		if utils.Search(volumes, name) >= 0 {
			fmt.Println("the volume already exists")
			return
		}

		podNodes := strings.Split(master.MustCmd2String("kubectl get pod -l glusterfs=pod -n glusterfs -o jsonpath={.items[*].spec.nodeName}", true), " ")
		volumePath := glusterMountPath(master)
		for _, nodeName := range podNodes {
			hosts.Get(nodeName).MustCmdStd(fmt.Sprintf("mkdir -p %s/%s", volumePath, name), os.Stdout, false)
		}

		options := strings.Join(args[1:], " ")

		brick := ""
		for _, node := range podNodes {
			brick += fmt.Sprintf(" %s:/data/%s", node, name)
		}
		master.MustCmdStd(fmt.Sprintf("%s volume create %s %s transport tcp %s force", gluster, name, options, brick), os.Stdout)
		master.MustCmdStd(fmt.Sprintf("%s volume start %s", gluster, name), os.Stdout)

		fmt.Printf(`
# example 
---
apiVersion: v1
kind: PersistentVolume
metadata:
  name: gluster-%s-pv
spec:
  capacity:
    storage: 8Gi
  accessModes:
    - ReadWriteMany
  glusterfs:
    endpoints: "glusterfs-endpoints"
    endpointsNamespace: glusterfs
    path: "%s"
    readOnly: false
---
kind: PersistentVolumeClaim
apiVersion: v1
metadata:
  name: gluster-%s-pvc
spec:
  accessModes:
    - ReadWriteMany
  resources:
    requests:
      storage: 8Gi
`, name, name, name)

		fmt.Printf(`
## mount it
yum install -y glusterfs
mkdir -p /mnt/%s
cluster=$(kubectl -n glusterfs get service glusterfs-endpoints -o jsonpath={.spec.clusterIP})
mount -t glusterfs $cluster:/%s /mnt/%s
`, name, name, name)
	},
}
