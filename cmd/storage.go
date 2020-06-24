package cmd

import (
	"github.com/ihaiker/vik8s/install/k8s"
	"github.com/ihaiker/vik8s/storage"
	"github.com/spf13/cobra"
	"strings"
)

var storageClassCmd = &cobra.Command{
	Use: "storage", Aliases: []string{"sc"}, Short: "Install StorageClass",
	PersistentPreRun: k8s.Config.LoadCmd,
}

func uninstallSCCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "uninstall", Run: func(cmd *cobra.Command, args []string) {
			name := cmd.Parent().Name()
			data, _ := cmd.Flags().GetBool("data")
			storage.Manager.Delete(name, data)
		},
	}
	cmd.Flags().Bool("data", false, "remove data folder")
	return cmd
}

func storageClassRun(cmd *cobra.Command, args []string) {
	name := cmd.Name()
	storage.Manager.Apply(name)
}

func init() {
	for _, s := range storage.Manager {
		cmd := &cobra.Command{
			Use: s.Name(), Long: s.Description(),
			Short: strings.SplitN(s.Description(), "\n", 2)[0],
			Run:   storageClassRun,
		}
		s.Flags(cmd)
		cmd.Flags().SortFlags = false
		storageClassCmd.AddCommand(cmd)
		cmd.AddCommand(uninstallSCCmd())
	}
	rootCmd.AddCommand(storageClassCmd)
}
