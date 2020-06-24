package cephfs

import (
	"github.com/ihaiker/vik8s/install/repo"
	"github.com/ihaiker/vik8s/libs/flags"
	"github.com/ihaiker/vik8s/libs/ssh"
	"github.com/spf13/cobra"
)

type (
	CephFS struct {
		Repo repo.Repo

		Version string `help:"ceph/ceph release version.\nYou can get it by command：curl -s https://registry.hub.docker.com/v2/repositories/ceph/ceph/tags/?page_size=100 | jq '.\"results\"[].name'" def:"v15.2.3"`
		FSID    string `help:"he fsid is a unique identifier for the cluster. " def:"10241024-1024-1024-1024-102410241024"`

		PrintOnly bool
		/*
			Monitors: Ceph Monitor维护着展示集群状态的各种图表，包括监视器图、 OSD 图、归置组（ PG ）图、和 CRUSH 图。
			Ceph 保存着发生在Monitors 、 OSD 和 PG上的每一次状态变更的历史信息（称为 epoch ）。
			由于采用Paxos算法多节点同步，所以数量必须是基数个
		*/
		Monitor struct { //ceph-mon
			Nodes        []string          `help:"" def:""`
			NodeSelector map[string]string `help:"label: sc.vik8s.io/ceph-mon=mon" def:""`
			Selected     ssh.Nodes         `flag:"-"`
		}
		/*
			Managers: Ceph Manager守护进程（ceph-mgr）负责跟踪运行时指标和Ceph集群的当前状态，
			包括存储利用率，当前性能指标和系统负载。 Ceph Manager守护程序还托管基于python的模块，
			以管理和公开Ceph集群信息，包括基于Web的Ceph仪表板和REST API。

			高可用性
			通常，您应该在每个运行ceph-mon守护程序的主机上设置ceph-mgr，以实现相同级别的可用性。
			默认情况下，无论哪个先出现的ceph-mgr实例都将由监视器激活，而其他实例将成为备用实例。在ceph-mgr守护程序之间不需要仲裁。
			如果活动守护程序无法将信标发送到监视器的时间超过了mon mgr信标宽限期（默认为30秒），那么它将被备用服务器替换。
			如果您想抢先进行故障转移，则可以使用ceph mgr fail <mgr name>将ceph-mgr守护进程明确标记为失败。
		*/
		Manager struct { //ceph-mgr
			Nodes        []string          `help:"" def:""`
			NodeSelector map[string]string `help:"label: sc.vik8s.io/ceph-mgr=mgr" def:""`
		}

		/*
			Ceph OSDs: Ceph OSD 守护进程（ Ceph OSD ）的功能是存储数据，
			处理数据的复制、恢复、回填、再均衡，并通过检查其他OSD 守护进程的心跳来向 Ceph Monitors 提供一些监控信息。
			当 Ceph 存储集群设定为有2个副本时，至少需要2个 OSD 守护进程，集群才能达到 active+clean 状态（ Ceph 默认有3个副本，但你可以调整副本数）。
		*/
		OSD struct { //ceph-osd , object storage daemon
			Nodes        []string          `help:"" def:""`
			NodeSelector map[string]string `help:"label: sc.vik8s.io/ceph-osd=osd" def:""`
			Devices      string            `help:"the devices name, for example: sdb"`
		}

		//MDSs: Ceph 元数据服务器（ MDS ）为 Ceph 文件系统存储元数据（也就是说，Ceph 块设备和 Ceph 对象存储不使用MDS ）。
		//元数据服务器使得 POSIX 文件系统的用户们，可以在不对 Ceph 存储集群造成负担的前提下，执行诸如 ls、find 等基本命令。
		MDS struct { //ceph-mds , Ceph Metadata Server
			Enable       bool              `help:"enable mds" def:"false"`
			Nodes        []string          `help:"" def:""`
			NodeSelector map[string]string `help:"label: sc.vik8s.io/ceph-mds=mds" def:""`
		}

		/*
			Ceph仪表板是基于Web的内置Ceph管理和监视应用程序，用于管理集群的各个方面和对象。它作为Ceph Manager守护程序模块实现。
		*/
		Dashboard struct {
			Enable   bool   `help:"Enable dashboard" def:"true"`
			User     string `help:"the dashboard user" def:"admin"`
			Password string `help:"the dashboard password" def:"vik8s@ceph"`
			Ingress  string `help:"add ceph dashboard ingress" def:"ceph.vik8s.io"`
		}

		Secrets     Secrets `flag:"-"`
		HostNetwork string  `flag:"-"`
		PodNetwork  string  `flag:"-"`

		ProvisionerRepo string `flag:"-"`

		Kube struct {
			AdminConf string
			VIP       string
		} `flag:"-"`
	}
)

func (c *CephFS) Name() string {
	return "ceph"
}

func (c *CephFS) Description() string {
	return `install cephfs cluster. support v15.+
Parameter Description:
--xxx.nodes:
Choose a set of nodes and block device, if this parameter is set, the --xxx.nodeselector parameter will be modified to '{{.Label}}={{.Value}}', 
and this label will be automatically added to these nodes.

rules: [ip1-ipN]:device1,device2 or [hostname]:device1,device2

for example: 
    --xxx.nodes=10.24.0.10:sdb,sdc --xxx.nodes=vm10:sdb --xxx.nodes=10.24.0.10-10.24.0.12:sdb
    --xxx.nodes=10.24.0.10 --xxx.nodes=10.24.0.13-10.24.0.15 --xxx.nodes=hostname1 --xxx.nodes=hostname2

If the --xxx.node and --xxx.nodeselector parameters are not set, 
the system will automatically find the worker node 
and label {xxx.label}={xxx.value} and modify --xxx.nodeselector parameter.
`
}

func (c *CephFS) Flags(cmd *cobra.Command) {
	flags.Flags(cmd.Flags(), c, "")
}
