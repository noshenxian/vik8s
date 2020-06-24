package glusterfs

import (
	"fmt"
	"github.com/ihaiker/vik8s/install/hosts"
	"github.com/ihaiker/vik8s/install/k8s"
	"github.com/ihaiker/vik8s/install/tools"
	"github.com/ihaiker/vik8s/libs/ssh"
	"github.com/ihaiker/vik8s/libs/utils"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"strings"
)

func labels(master *ssh.Node) map[string]string {
	labelsString := master.MustCmd2String("kubectl -n glusterfs get daemonsets.apps glusterfs" +
		" -o go-template='{{range $k,$v := .spec.template.spec.nodeSelector}},{{$k}}={{$v}}{{end}}'")
	labels := make(map[string]string)
	for _, labelAndValue := range strings.Split(labelsString[1:], ",") {
		label, value := utils.Split2(labelAndValue, "=")
		labels[label] = value
	}
	return labels
}

/*
### 新接待加入时使用hostname问题
在新加入节点使用hostname方式会出现一些问题，提示无法连接peer，
这是因为在gluster完成第一次安装后后面又有新机器添加，如果使用hostname方式的话需要添加/etc/hosts文件内容，之前初始化的pod里面并不包含这些新添加的机器

解决方式：
	- 更新pod，同时会自动更新/etc/hosts文件(会导致peer断联几秒钟)
	- 直接更新host文件. `cat /etc/hosts >> /var/lib/kubelet/pods/$(PODID)/etc-hosts`
*/
func updatePodHosts(master *ssh.Node) {
	utils.Line("update gluster pod /etc/hosts")
	nodeNameAndUIDs := master.MustCmd2String("kubectl get pods -n glusterfs -l glusterfs=pod -l glusterfs-node=pod " +
		"-o jsonpath='{range .items[*]},{.spec.nodeName}:{.metadata.uid}{end}'")

	for _, nodeNameAndUID := range strings.Split(nodeNameAndUIDs[1:], ",") {
		nodeName, uid := utils.Split2(nodeNameAndUID, ":")
		hosts.Get(nodeName).MustCmd(fmt.Sprintf("cat /etc/hosts >> /var/lib/kubelet/pods/%s/etc-hosts", uid))
	}
}

var peersCmd = &cobra.Command{
	Use: "peer", Short: "add peer",
	Example: "vik8s storage glusterfs peer [--volume=vol2 --volume=vol1] <nodeHostname/nodeIP> <nodeHostname/nodeIP> ...",
	Args:    cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		master := k8s.Config.Master()
		labels := labels(master)
		glusterNodes := tools.SearchLabelNode(master, labels)
		for _, node := range glusterNodes {
			utils.Assert(utils.Search(args, node) == -1, "gluster node (%s) already exists ", node)
		}
		updatePodHosts(master)

		utils.Line("add gluster node")
		for _, peer := range args {
			node := hosts.Get(peer)
			preInstall(node)
			master.MustCmd(fmt.Sprintf("kubectl label node %s %s ", node.Hostname, utils.Join(labels, " ", "=")))
		}

		wait(master, "Waiting New Node",
			"kubectl get ds glusterfs -n glusterfs -o jsonpath='{.status.numberAvailable}'", len(glusterNodes)+len(args))

		//这里不能用户 用变量gluster提供的命令，存在BUG，1、第0个正好是本身就报错了，2、已经创建了volume
		cli := master.MustCmd2String(fmt.Sprintf("kubectl get pods -n glusterfs -l glusterfs=pod "+
			"-o go-template='{{range .items}}{{if eq .spec.nodeName \"%s\"}}{{.metadata.name}}{{end}}{{end}}'", glusterNodes[0]))

		cli = fmt.Sprintf("kubectl exec -n glusterfs %s -- gluster", cli)
		for _, peer := range args {
			master.MustCmdStd(fmt.Sprintf("%s peer probe %s", cli, hosts.Get(peer).Hostname), os.Stdout)
		}

		master.MustCmdStd(fmt.Sprintf("%s peer status", gluster), os.Stdout)

		volumeBricks, _ := cmd.Flags().GetStringSlice("volume")
		if len(volumeBricks) > 0 {
			utils.Line("gluster volume add-brick")
			volumes := glusterVolumes(master)
			path := glusterMountPath(master)
			for _, brick := range volumeBricks {
				if utils.Search(volumes, brick) == -1 {
					fmt.Printf("the volume %s not exists !", brick)
					continue
				}
				addBrick := ""
				for _, arg := range args {
					node := hosts.Get(arg)
					node.MustCmd(fmt.Sprintf("mkdir -p %s", filepath.Join(path, brick)))
					addBrick += fmt.Sprintf(" %s:/data/%s", node.Hostname, brick)
				}
				master.MustCmdStd(fmt.Sprintf("%s volume add-brick %s %s",
					cli, brick, addBrick), os.Stdout)
			}
		}
	},
}

func init() {
	peersCmd.Flags().StringSlice("volume", []string{}, "volume to add brick")
}
