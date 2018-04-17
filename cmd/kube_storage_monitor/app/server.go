package app

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/caicloud/kube-storage-monitor/cmd/kube_storage_monitor/local_pv"

	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/util/wait"
)

type monitorOpt struct {
	kube_storage_types []string
}

func (mo *monitorOpt) AddFlags(fs *pflag.FlagSet) {
	fs.StringSliceVar(&mo.kube_storage_types, "kube-storage-types", mo.kube_storage_types, ""+
		"kube-storage-types is the backend storage drivers type, such as: local_pv, cephfs, rbd... ")
}

// NewMonitorServerCommand creates a *cobra.Command object with default parameters
func NewMonitorServerCommand() *cobra.Command {
	mo := &monitorOpt{}
	cmd := &cobra.Command{
		Use:  "kube_storage_monitor",
		Long: `The Kubernetes storage monitor monitors different types of storage driver PVs.`,
		Run: func(cmd *cobra.Command, args []string) {
			Run(mo, wait.NeverStop)
		},
	}
	mo.AddFlags(pflag.CommandLine)

	return cmd
}

func Run(mo *monitorOpt, stopCh <-chan struct{}) {
	if len(mo.kube_storage_types) == 0 {
		glog.Fatalf("kube-storage-types must be set")
	}
	// TODO(@NickrenREN): need to handle this more elegantly
	// check if we support the storage types
	err := checkStorageTypes(mo.kube_storage_types)
	if err != nil {
		glog.Errorf("check storage types error: %v", err)
		os.Exit(1)
	}
	// run specific storage monitor
	for _, sType := range mo.kube_storage_types {
		switch sType {
		// Add local_pv support at first
		// If we get to support other storage types, need to add here
		case "local_pv":
			go local_pv.RunLocalPVMonitor()
		}
	}
	<-stopCh
}

var (
	supported_storage_types = map[string]bool{}
)

// getSupportedStorageTypes returns the supported storage types
// Add local_pv support at first
// If we get to support other storage types, need to add here too
func getSupportedStorageTypes() {
	supported_storage_types["local_pv"] = true
}

func checkStorageTypes(storageTypes []string) error {
	getSupportedStorageTypes()
	for _, sType := range storageTypes {
		if !supported_storage_types[sType] {
			return fmt.Errorf("monitor does not support %s storage type now", sType)
		}
	}
	return nil
}
