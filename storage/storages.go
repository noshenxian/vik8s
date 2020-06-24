package storage

import (
	"fmt"
	"github.com/ihaiker/vik8s/storage/cephfs"
	"github.com/ihaiker/vik8s/storage/glusterfs"
	"github.com/spf13/cobra"
)

type Storage interface {
	Name() string
	Description() string
	Flags(cmd *cobra.Command)
	Apply()
	Delete(data bool)
}

type storages []Storage

var Manager = storages{
	new(glusterfs.GlusterFS), new(cephfs.CephFS),
}

func (p *storages) Apply(name string) {
	fmt.Println("apply ", name)
	for _, plugin := range *p {
		if plugin.Name() == name {
			plugin.Apply()
			return
		}
	}
}

func (p *storages) Delete(name string, data bool) {
	fmt.Println("delete ", name)
	for _, plugin := range *p {
		if plugin.Name() == name {
			plugin.Delete(data)
			return
		}
	}
}

func (p *storages) SetDefault(name string, args []string) {

}
