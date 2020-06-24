package cephfs

import (
	"github.com/ihaiker/vik8s/install/repo"
	"github.com/ihaiker/vik8s/install/tools"
)

func repoFile(version, distro string) []byte {
	return tools.Template(`
[ceph]
name=Ceph packages for $basearch
baseurl={{.Url}}/rpm-{{.Version}}/{{.Distro}}/$basearch
enabled=1
priority=2
gpgcheck=1
gpgkey={{.Url}}/keys/release.asc

[ceph-noarch]
name=Ceph noarch packages
baseurl={{.Url}}/rpm-{{.Version}}/{{.Distro}}/noarch
enabled=1
priority=2
gpgcheck=1
gpgkey={{.Url}}/keys/release.asc

[ceph-source]
name=Ceph source packages
baseurl={{.Url}}/rpm-{{.Version}}/{{.Distro}}/SRPMS
enabled=0
priority=2
gpgcheck=1
gpgkey={{.Url}}/keys/release.asc
`, tools.Json{"Url": repo.Ceph(), "Version": version[1:], "Distro": distro}).Bytes()
}
