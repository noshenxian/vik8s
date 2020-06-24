package glusterfs

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/cheggaaa/pb"
	"github.com/ihaiker/vik8s/install/hosts"
	"github.com/ihaiker/vik8s/install/k8s"
	"github.com/ihaiker/vik8s/install/repo"
	"github.com/ihaiker/vik8s/install/tools"
	"github.com/ihaiker/vik8s/libs/flags"
	"github.com/ihaiker/vik8s/libs/ssh"
	"github.com/ihaiker/vik8s/libs/utils"
	"github.com/spf13/cobra"
	"gopkg.in/fatih/color.v1"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	labelKey       = "sc.vik8s.io/glusterfs"
	heketiLabelKey = "sc.vik8s.io/heketi"
	labelValue     = "vik8s"

	gluster   = "kubectl exec -n glusterfs `kubectl get pods -n glusterfs -l glusterfs=pod  -o jsonpath={.items[0].metadata.name}` -- gluster"
	heketiCli = "kubectl exec -n glusterfs `kubectl get pods -n glusterfs -l heketi=pod -o jsonpath={.items[0].metadata.name}` -- heketi-cli"

	heketiDeployPod = "kubectl get pods -n glusterfs -l deploy-heketi=pod -o jsonpath={.items[0].metadata.name}"
	heketiPod       = "kubectl get pods -n glusterfs -l heketi=pod -o jsonpath={.items[0].metadata.name}"
	heketiDeployCli = "kubectl exec -n glusterfs `" + heketiDeployPod + "` -- heketi-cli"
)

type (
	nodeAndDevices struct {
		Node    *ssh.Node
		Devices []string
	}

	GlusterFS struct {
		PrintOnly bool `help:"Just print the final installation details" def:"false"`

		Repo         string            `help:"Choose a container registry to pull control plane images from."`
		NodeSelector map[string]string `help:"the glusterfs daemonset pod nodeselector" def:""`
		Nodes        []string          `help:"set of node names where glusterfs daemonset is deployed" def:""`

		Device struct {
			Exclude []string `help:"raw block device name exclude filter (regexp)" def:""`
			Include []string `help:"raw block device name include filter (regexp)" def:"ha[a-d],sd[a-p],vd[a-p],hd[a-p]"`
		}

		VolumesDir string `help:"Create custom 'glusterfs volume' directory, these volumes are not managed by heketi." def:"/data/glusterfs/volumes"`

		VIP string `flag:"-"`

		Heketi struct {
			Name   string `help:"storageclass name" def:"gluster-heketi"`
			Enable bool   `help:"enable glusterfs heketi and storageclass" def:"true"`

			Replicas     int               `help:"" def:"1"`
			Nodes        []string          `help:"the heketi node" def:""`
			NodeSelector map[string]string `help:"the heketi nodeselector" def:""`

			AdminKey string `help:"heketi server adminkey" def:"vik8s"`
			UserKey  string `help:"heketi server userkey" def:"vik8s"`

			ConfigJSON   string `flag:"-"`
			TopologyJson string `flag:"-"`
			VIP          string `flag:"-"`
		}

		Deploys []*nodeAndDevices `flag:"-"`
	}
)

func (gfs *GlusterFS) Name() string {
	return "glusterfs"
}

func (gfs *GlusterFS) Description() string {
	return tools.Template(`GlusterFS Native Storage Service

Gluster Requirements:
Each node that will be running gluster needs at least one raw block device. This block device will be made into an LVM PV and fully managed by Heketi.
For typical installs at least three nodes will need to be provisioned. Extra small clusters can be configured with just one node, but additional steps will need to be taken to always volumes with no durability.

Parameter Description:
--nodes:
Choose a set of nodes and block device, if this parameter is set, the --nodeselector parameter will be modified to '{{.Label}}={{.Value}}', 
and this label will be automatically added to these nodes.

rules: [ip1-ipN]:device1,device2 or [hostname]:device1,device2

for example: 
	--nodes=10.24.0.10:sdb,sdc --nodes=vm10:sdb --nodes=10.24.0.10-10.24.0.12:sdb
    --nodes=10.24.0.10 --nodes=10.24.0.13-10.24.0.15 --nodes=hostname1 --nodes=hostname2

If the --node and --nodeselector parameters are not set, 
the system will automatically find the worker node 
and label {{.Label}}={{.Value}} and modify --nodeselector parameter.

OpenEBS Documentation:
	https://github.com/gluster/gluster-kubernetes
	https://github.com/gluster/gluster
`, tools.Json{"Label": labelKey, "Value": labelValue}).String()
}

func (gfs *GlusterFS) Flags(cmd *cobra.Command) {
	flags.Flags(cmd.Flags(), gfs, "")
	cmd.AddCommand(volumeCmd, peersCmd)
}

func (gfs *GlusterFS) Apply() {
	gfs.Repo = repo.Suffix(gfs.Repo)
	gfs.Heketi.ConfigJSON = base64.StdEncoding.EncodeToString(
		tools.MustAssert("yaml/storage/glusterfs/heketi.json", gfs))
	gfs.Heketi.VIP = tools.GetVip(k8s.Config.Kubernetes.SvcCIDR, tools.GlusterGSHeketiService)
	gfs.VIP = tools.GetVip(k8s.Config.Kubernetes.SvcCIDR, tools.GlusterGSEndpointService)

	nodes := hosts.Gets(k8s.Config.Masters, k8s.Config.Nodes)

	//选择需要部署的node
	gfs.choseDeployNodes(nodes)
	if gfs.PrintOnly {
		return
	}

	//安装必须软件
	for _, d := range gfs.Deploys {
		preInstall(d.Node)
	}

	//heketi topology.json 创建
	gfs.makeTopologyJson()

	//部署
	tools.MustScpAndApplyAssert(k8s.Config.Master(), "yaml/storage/glusterfs/glusterfs.yaml", gfs)

	//等待domanset准备就绪
	gfs.waitGlusterfsDaemonSet(k8s.Config.Master())

	//gluster节点准备添加
	gfs.addAlias(k8s.Config.Master())

	//启用heketi
	gfs.enableHeketi(k8s.Config.Master())

	fmt.Println(" ====== SUCCESS ===== ")
}

func preInstall(node *ssh.Node) {
	_ = node.MustCmd2String("modprobe dm_snapshot && modprobe dm_mirror modprobe dm_thin_pool")
	tools.Install("glusterfs-fuse", "", node)
}

func (gfs *GlusterFS) choseDeployNodes(nodes ssh.Nodes) {
	gfs.Deploys = make([]*nodeAndDevices, 0)

	if len(gfs.Nodes) != 0 {
		for _, n := range gfs.Nodes {
			nodeAndDevice := strings.Split(n, ":")
			selectNodes := tools.SelectNodeNames(nodes, []string{nodeAndDevice[0]})
			var assignDevices []string
			if len(nodeAndDevice) == 2 {
				assignDevices = strings.Split(nodeAndDevice[1], ",")
			}
			for _, selectNode := range selectNodes {
				one := &nodeAndDevices{Node: nodes.Get(selectNode)}
				if gfs.Heketi.Enable {
					if len(assignDevices) == 0 {
						gfs.findValidDevices(one, true)
					} else {
						gfs.findValidDevices(one, false)
						notMatch := utils.SelectNotMatch(assignDevices, one.Devices...)
						utils.Assert(len(notMatch) == 0, "%s not found disk [%s]", selectNode, strings.Join(notMatch, ","))
						one.Devices = assignDevices
					}
				}
				gfs.Deploys = append(gfs.Deploys, one)
			}
		}
		gfs.NodeSelector = map[string]string{labelKey: labelValue}

	} else if len(gfs.NodeSelector) != 0 {
		storageNodes := tools.SearchLabelNode(k8s.Config.Master(), gfs.NodeSelector)
		fmt.Println("find label nodes: ", strings.Join(storageNodes, ","))

		for _, storageNode := range storageNodes {
			one := &nodeAndDevices{Node: nodes.Get(storageNode)}
			if gfs.Heketi.Enable {
				gfs.findValidDevices(one, true)
			}
			gfs.Deploys = append(gfs.Deploys, one)
		}
	} else {
		color.Blue("[warning] the --nodes and --nodeselector parameters are not specified, the worker node is automatically obtained")
		workers := tools.SearchLabelNode(k8s.Config.Master(), map[string]string{"node-role.kubernetes.io/master!": ""})
		fmt.Println("find worker nodes: ", strings.Join(workers, ","))

		for _, worker := range workers {
			one := &nodeAndDevices{Node: nodes.Get(worker)}
			if gfs.Heketi.Enable {
				gfs.findValidDevices(one, true)
				if len(one.Devices) > 0 {
					gfs.Deploys = append(gfs.Deploys, one)
				}
			} else {
				gfs.Deploys = append(gfs.Deploys, one)
			}
		}
		gfs.NodeSelector = map[string]string{labelKey: labelValue}
	}

	utils.Assert(len(gfs.Deploys) > 0, "no nodes found for deploy!")
	if gfs.Heketi.Enable {
		for _, deploy := range gfs.Deploys {
			utils.Assert(len(deploy.Devices) > 0, "%s no disks found as valid", deploy.Node.Hostname)
			fmt.Printf("%s devices: [ %s ]\n", deploy.Node.Hostname, strings.Join(deploy.Devices, ","))
		}
	}

	if gfs.PrintOnly {
		return
	}

	selectNodes := make([]string, 0)
	for _, deploy := range gfs.Deploys {
		selectNodes = append(selectNodes, deploy.Node.Hostname)
	}
	tools.AutoLabelNodes(k8s.Config.Master(), gfs.NodeSelector, selectNodes...)

	//为注解点打标签
	if gfs.Heketi.Enable && len(gfs.Heketi.Nodes) != 0 {
		gfs.Heketi.NodeSelector = map[string]string{heketiLabelKey: labelValue}
		tools.AutoLabelNodes(k8s.Config.Master(), gfs.Heketi.NodeSelector, gfs.Heketi.Nodes...)
	}
}

func (gfs *GlusterFS) makeTopologyJson() {
	if !gfs.Heketi.Enable {
		return
	}
	nodes := make([]tools.Json, 0)
	for _, deploy := range gfs.Deploys {
		devices := make([]tools.Json, 0)
		for _, device := range deploy.Devices {
			devices = append(devices, tools.Json{
				"name":        "/dev/" + device,
				"destroydata": false,
			})
		}
		node := tools.Json{
			"node": tools.Json{
				"hostnames": tools.Json{
					"manage":  []string{deploy.Node.Hostname},
					"storage": []string{deploy.Node.Host},
				},
				"zone": 1,
			},
			"devices": devices,
		}
		nodes = append(nodes, node)
	}
	data := tools.Json{"clusters": []tools.Json{{"nodes": nodes}}}
	bs, _ := json.MarshalIndent(data, "    ", "    ")
	gfs.Heketi.TopologyJson = string(bs)
}

func (gfs *GlusterFS) findValidDevices(n *nodeAndDevices, filter bool) {
	devices := strings.Split(n.Node.MustCmd2String("lsblk  -d -o NAME,TYPE | grep disk | awk '{print $1}'", false), "\n")
	if filter {
		devices = utils.SelectMatch(devices, gfs.Device.Include...)
		devices = utils.SelectNotMatch(devices, gfs.Device.Exclude...)
	}
	n.Devices = devices
}

func (gfs *GlusterFS) waitGlusterfsDaemonSet(master *ssh.Node) {
	wait(master, "Waiting all GlusterFS Daemanset available: ",
		"kubectl get ds glusterfs -n glusterfs -o jsonpath='{.status.numberAvailable}'", len(gfs.Deploys))
	if gfs.Heketi.Enable {
		wait(master, "Waiting GlusterFS deploy heketi service available: ",
			"kubectl get deployments.apps -n glusterfs deploy-heketi -o jsonpath='{.status.availableReplicas}'", 1)
	}
}

func wait(master *ssh.Node, info, cmd string, size int) {
	bar := pb.New(size)
	bar.SetTemplateString(info + " : {{counters . }}")
	defer bar.Finish()
	bar.SetWriter(os.Stdout).SetRefreshRate(time.Second)
	bar.Set(pb.Bytes, false).Set(pb.Terminal, true)
	bar.Start()

	for {
		out := master.MustCmd2String(cmd, true)
		current, _ := strconv.Atoi(out)
		bar.SetCurrent(int64(current))
		if current == size {
			break
		}
		time.Sleep(time.Second)
	}
}

func (gfs *GlusterFS) addAlias(master *ssh.Node) {
	_ = master.ScpContent(
		[]byte(fmt.Sprintf("alias gluster='%s'\nalias heketi-cli='%s'", gluster, gfs.adminCli(heketiCli))),
		"/etc/profile.d/glusterfs.sh")
	_ = master.CmdStd(fmt.Sprintf("%s peer status", gluster), os.Stdout)
	return
}

func (gfs *GlusterFS) enableHeketi(master *ssh.Node) {
	if !gfs.Heketi.Enable {
		for _, nn := range gfs.Deploys { //peer probe
			master.MustCmd2String(fmt.Sprintf("%s peer probe %s", gluster, nn.Node.Hostname))
		}
		return
	}
	deployCli := gfs.adminCli(heketiDeployCli)
	master.MustCmdStd(deployCli+" topology load --json=/data/topology.json", os.Stdout)
	master.MustCmdStd(deployCli+" setup-heketi-db-storage", os.Stdout)

	//copy heketi.db
	heketiDB := master.Vik8s("heketi.db")
	_, _ = master.Cmd(fmt.Sprintf("kubectl cp glusterfs/`%s`:/var/lib/heketi/heketi.db %s", heketiDeployPod, heketiDB))

	//restart pod
	_ = master.MustCmd2String("kubectl delete pod -n glusterfs -l heketi=pod")
	_ = master.MustCmd2String(fmt.Sprintf("kubectl cp %s glusterfs/`%s`:/var/lib/heketi/heketi.db", heketiDB, heketiPod))

	master.MustCmdStd("kubectl delete -n glusterfs all,service,jobs,deployment,secret --selector='deploy-heketi'", os.Stdout)
}

func (gfs *GlusterFS) adminCli(cli string) string {
	return fmt.Sprintf("%s --user admin --secret %s", cli, gfs.Heketi.AdminKey)
}

func (gfs *GlusterFS) createVolume(master *ssh.Node, name string, nodes []string) {
	found := master.MustCmd2String(fmt.Sprintf("%s volume list | grep %s || echo 'NOT_FOUND'", gluster, name))
	if found != name {
		brick := ""
		for _, node := range nodes {
			brick += fmt.Sprintf(" %s:/data/%s", node, name)
		}
		master.MustCmdStd(fmt.Sprintf("%s volume create %s transport tcp %s force", gluster, name, brick), os.Stdout)
	}
	status := master.MustCmd2String(fmt.Sprintf("%s volume info %s | grep 'Status: ' | awk -F': ' '{print $2}'", gluster, name))
	if status != "Started" {
		master.MustCmdStd(gluster+" volume start "+name, os.Stdout)
	}
}

func (gfs *GlusterFS) Delete(data bool) {
	master := k8s.Config.Master()
	labels := labels(master)
	master.MustCmd("rm -rf /etc/profile.d/glusterfs.sh")

	err := master.CmdStd("kubectl delete namespaces glusterfs", os.Stdout)
	utils.Panic(err, "delete namespaces glusterfs")

	_, _ = master.Cmd("kubectl delete storageclasses.storage.k8s.io " + gfs.Heketi.Name)
	if data {
		nodes := tools.SearchLabelNode(master, labels)
		for _, nodeName := range nodes {
			node := hosts.Get(nodeName)
			node.MustCmd("rm -rf /etc/glusterfs/* /var/lib/glusterd/* /var/lib/heketi/*")
		}
		master.MustCmd(fmt.Sprintf("kubectl label node %s %s",
			strings.Join(nodes, " "),
			utils.KeyJoin(labels, " ", "-"),
		))
	}
}
