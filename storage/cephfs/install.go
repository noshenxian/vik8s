package cephfs

import (
	"fmt"
	"github.com/cheggaaa/pb"
	"github.com/google/uuid"
	"github.com/ihaiker/vik8s/install/hosts"
	"github.com/ihaiker/vik8s/install/k8s"
	"github.com/ihaiker/vik8s/install/repo"
	"github.com/ihaiker/vik8s/install/tools"
	"github.com/ihaiker/vik8s/libs/ssh"
	"github.com/ihaiker/vik8s/libs/utils"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	monPod = "kubectl get pods -n ceph -l app=ceph -l daemon=mon -o jsonpath={.items[0].metadata.name}"
	cli    = "kubectl exec -n ceph `" + monPod + "` --"
)

func (c *CephFS) Apply() {
	c.Repo.Set("ceph")
	c.ProvisionerRepo = repo.Suffix(repo.QuayIO(c.Repo.Url))

	if c.FSID == "" {
		c.FSID = uuid.New().String()
	}

	nodes := hosts.Gets(k8s.Config.Masters, k8s.Config.Nodes)
	c.Kube.AdminConf = k8s.Config.Master().MustCmd2String("cat /etc/kubernetes/admin.conf")
	c.Kube.VIP = k8s.Config.Kubernetes.ApiServerVIP

	c.chooseDeployNodes(nodes)
	c.addRepo(nodes)
	c.generateSecrets(nodes)

	//部署
	tools.MustScpAndApplyAssert(k8s.Config.Master(), "yaml/storage/ceph/ceph.yaml", c, templateFuns)

	c.wait(k8s.Config.Master(), "Waiting Ceph Monitor: ",
		"kubectl get ds ceph-mon -n ceph -o jsonpath='{.status.numberAvailable}'", len(c.Monitor.Selected))

	c.wait(k8s.Config.Master(), "Waiting Ceph Manager: ",
		"kubectl get ds ceph-mgr -n ceph -o jsonpath='{.status.numberAvailable}'", len(c.Manager.Nodes))

	c.enableDashboard(k8s.Config.Master())
	c.createRBDPool(k8s.Config.Master())
	c.createCephFSPool(k8s.Config.Master())
	c.addAlias(k8s.Config.Master())
}

func (c *CephFS) addRepo(nodes ssh.Nodes) {
	utils.Line("install ceph-common")
	ssh.Sync(nodes, func(i int, node *ssh.Node) {
		distro := fmt.Sprintf("el%s", node.MajorVersion)
		node.MustScpContent(repoFile(c.Version, distro), "/etc/yum.repos.d/ceph.repo")
		tools.Installs(node, "ceph-common")
	})
}

func (c *CephFS) wait(master *ssh.Node, info, cmd string, size int) {
	bar := pb.New(size)
	bar.SetTemplateString(info + " : {{counters . }}")
	defer bar.Finish()
	bar.SetWriter(os.Stdout).SetRefreshRate(time.Second)
	bar.Set(pb.Bytes, false).Set(pb.Terminal, true)
	bar.Start()
	for {
		out, err := master.Cmd2String(cmd, true)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		current, _ := strconv.Atoi(out)
		bar.SetCurrent(int64(current))
		if current == size {
			break
		}
		time.Sleep(time.Second)
	}
}

func (c *CephFS) enableDashboard(master *ssh.Node) {
	if !c.Dashboard.Enable {
		return
	}
	//enable dashboard
	master.MustCmd(fmt.Sprintf("%s ceph config set mgr mgr/dashboard/ssl false", cli))
	master.MustCmd(fmt.Sprintf("%s ceph config set mgr mgr/dashboard/server_port 7000", cli))
	master.MustCmd(fmt.Sprintf("%s ceph mgr module enable dashboard", cli))
	//> v15.+
	master.MustCmd(fmt.Sprintf("%s ceph dashboard ac-user-create %s %s administrator",
		cli, c.Dashboard.User, c.Dashboard.Password))
	//< v13.+ master.MustCmd(fmt.Sprintf("%s ceph dashboard set-login-credentials %s %s", cli, c.Dashboard.User, c.Dashboard.Password))
	//enable msgr2
	master.MustCmd(fmt.Sprintf("%s ceph mon enable-msgr2", cli))
}

func (c *CephFS) createRBDPool(master *ssh.Node) {
	master.MustCmdStd(fmt.Sprintf("%s ceph osd pool create kube 8 8", cli), os.Stdout)
	//rbd create kube/ceph-image -s 1G --image-format 2 --image-feature layering
	//master.MustCmdStd(fmt.Sprintf("%s rbd create rbd-image -s 1G --image-feature layering --pool rbd", cli), os.Stdout)
	//master.MustCmdStd(fmt.Sprintf("%s ceph osd pool application enable rbd rbd-image", cli), os.Stdout)
}

func (c *CephFS) createCephFSPool(master *ssh.Node) {
	if !c.MDS.Enable {
		return
	}
	master.MustCmdStd(fmt.Sprintf("%s ceph osd pool create cephfs_data 128", cli), os.Stdout)
	master.MustCmdStd(fmt.Sprintf("%s ceph osd pool create cephfs_metadata 64", cli), os.Stdout)
	master.MustCmdStd(fmt.Sprintf("%s ceph fs new cephfs cephfs_metadata cephfs_data", cli), os.Stdout)
}

func (c *CephFS) addAlias(master *ssh.Node) {
	master.MustScpContent([]byte(fmt.Sprintf(`alias ceph='%s ceph'
alias rdb='%s rdb'`, cli, cli)),
		"/etc/profile.d/ceph.sh")
	master.MustCmdStd(fmt.Sprintf("%s ceph -s", cli), os.Stdout)
}

func (c *CephFS) Delete(data bool) {
	_, _ = k8s.Config.Master().Cmd("rm -rf /etc/profile.d/ceph.sh")
	_, _ = k8s.Config.Master().Cmd("kubectl delete -n ceph storageclasses ceph-rbd")
	_, _ = k8s.Config.Master().Cmd("kubectl delete -n ceph storageclasses cephfs")
	_, _ = k8s.Config.Master().Cmd("kubectl delete ns ceph")
	nodes := k8s.Config.Master().MustCmd2String("kubectl get node " +
		"-o go-template='{{range .items}}" +
		"{{$node := .}}" +
		"{{range $k,$v := .metadata.labels}}" +
		"{{$node.metadata.name}} {{$k}}{{\"\\n\"}}" +
		"{{end}}" +
		"{{end}}' | grep \"sc.vik8s.io/ceph\"")
	for _, node := range strings.Split(nodes, "\n") {
		k8s.Config.Master().MustCmd(fmt.Sprintf("kubectl label node %s-", node))
	}
	if data {
		nodes := hosts.Gets(k8s.Config.Masters, k8s.Config.Nodes)
		ssh.Sync(nodes, func(i int, node *ssh.Node) {
			node.MustCmd("rm -rf /var/lib/ceph/* /etc/ceph/*")
		})
	}
}
