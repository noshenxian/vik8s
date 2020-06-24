package cephfs

import (
	"fmt"
	"github.com/ihaiker/vik8s/install/k8s"
	"github.com/ihaiker/vik8s/install/tools"
	"github.com/ihaiker/vik8s/libs/ssh"
	"github.com/ihaiker/vik8s/libs/utils"
	"net"
	"os"
	"strings"
)

const (
	monLabel = "sc.vik8s.io/ceph-mon"
	monValue = "mon"

	mgrLabel = "sc.vik8s.io/ceph-mgr"
	mgrValue = "mgr"

	osdLabel = "sc.vik8s.io/ceph-osd"
	osdValue = "osd"

	mdsLabel = "sc.vik8s.io/ceph-mds"
	mdsValue = "mds"
)

//
// @param nodes 指定node
// @param label
// @return specified 是否指定
//
func (c *CephFS) nodeLabel(ns ssh.Nodes, nodes *[]string, labels *map[string]string, autoLabel map[string]string) (specified bool) {
	if len(*nodes) != 0 {
		*nodes = tools.SelectNodeNames(ns, *nodes)
		*labels = autoLabel
		return true
	} else if len(*labels) != 0 {
		*nodes = tools.SearchLabelNode(k8s.Config.Master(), *labels)
		return true
	} else {
		*labels = autoLabel
	}
	return false
}

func (c *CephFS) chooseMonitor(nodes ssh.Nodes, workers []string) {
	if c.nodeLabel(nodes, &c.Monitor.Nodes, &c.Monitor.NodeSelector, map[string]string{monLabel: monValue}) {
		return
	}
	c.Monitor.Nodes = workers[0:1]
	for _, node := range c.Monitor.Nodes {
		c.Monitor.Selected = append(c.Monitor.Selected, nodes.Get(node))
	}
}

func (c *CephFS) chooseManager(nodes ssh.Nodes, workers []string) {
	if c.nodeLabel(nodes, &c.Manager.Nodes, &c.Manager.NodeSelector, map[string]string{mgrLabel: mgrValue}) {
		return
	}
	c.Manager.Nodes = workers[1:2]
}

func (c *CephFS) chooseOSD(nodes ssh.Nodes, workers []string) {
	if !c.nodeLabel(nodes, &c.OSD.Nodes, &c.OSD.NodeSelector, map[string]string{osdLabel: osdValue}) {
		if len(workers) > 2 {
			c.OSD.Nodes = workers[1:]
		} else {
			c.OSD.Nodes = workers
		}
	}
	if c.OSD.Devices == "" {
		devices := strings.Split(nodes.Get(c.OSD.Nodes[0]).
			MustCmd2String("lsblk  -d -o NAME,TYPE | grep disk | awk '{print $1}'", false), "\n")
		c.OSD.Devices = devices[len(devices)-1]
	}
}

func (c *CephFS) chooseMDS(nodes ssh.Nodes, workers []string) {
	if c.nodeLabel(nodes, &c.MDS.Nodes, &c.MDS.NodeSelector, map[string]string{mdsLabel: mdsValue}) {
		return
	}
	c.MDS.Nodes = workers[0:1]
}

func (c *CephFS) print() {
	fmt.Println("------------------------------------------------------------")
	fmt.Println("Choose Monitor: ", utils.Join(c.Monitor.NodeSelector, ",", "="))
	fmt.Println("    Nodes:", strings.Join(c.Monitor.Nodes, ","))

	fmt.Println("Choose Manager: ", utils.Join(c.Manager.NodeSelector, ",", "="))
	fmt.Println("    Nodes:", strings.Join(c.Manager.Nodes, ","))

	fmt.Println("Choose ODS ( devices", c.OSD.Devices, "): ", utils.Join(c.OSD.NodeSelector, ",", "="))
	fmt.Println("    Nodes:", strings.Join(c.OSD.Nodes, ","))

	if c.MDS.Enable {
		fmt.Println("Choose MDS: ", utils.Join(c.MDS.NodeSelector, ",", "="))
		fmt.Println("    Nodes:", strings.Join(c.MDS.Nodes, ","))
	}
	fmt.Println("------------------------------------------------------------")
}

func (c *CephFS) chooseDeployNodes(nodes ssh.Nodes) {
	workers := tools.SearchLabelNode(k8s.Config.Master(), map[string]string{"node-role.kubernetes.io/master!": ""})
	utils.Assert(len(workers) >= 2, "work node must be greater than or equal to 2")

	c.chooseMonitor(nodes, workers)
	c.chooseManager(nodes, workers)
	c.chooseOSD(nodes, workers)
	if c.MDS.Enable {
		c.chooseMDS(nodes, workers)
	}
	c.PodNetwork = k8s.Config.Kubernetes.PodCIDR
	c.HostNetwork = c.getIPCidr()

	c.print()

	if c.PrintOnly {
		os.Exit(0)
	}

	tools.AutoLabelNodes(k8s.Config.Master(), c.Monitor.NodeSelector, c.Monitor.Nodes...)
	tools.AutoLabelNodes(k8s.Config.Master(), c.Manager.NodeSelector, c.Manager.Nodes...)
	tools.AutoLabelNodes(k8s.Config.Master(), c.OSD.NodeSelector, c.OSD.Nodes...)
	if c.MDS.Enable {
		tools.AutoLabelNodes(k8s.Config.Master(), c.MDS.NodeSelector, c.MDS.Nodes...)
	}
}

func (c *CephFS) getIPCidr() string {
	ip := net.ParseIP(c.Monitor.Selected[0].Host).To4()
	ip[3] = 0
	return fmt.Sprintf("%s/16", ip.String())
}
