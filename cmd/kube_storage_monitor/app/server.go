package app

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/caicloud/kube-storage-monitor/cmd/kube_storage_monitor/local_pv"
	"github.com/caicloud/kube-storage-monitor/cmd/kube_storage_monitor/remote_pv"

	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/util/wait"
)

type monitorOpt struct {
	kubeStorageTypes  []string
	enableNodeWatcher bool

	storageDriver           string
	storageDriverConfigFile string
}

func (mo *monitorOpt) AddFlags(fs *pflag.FlagSet) {
	fs.StringSliceVar(&mo.kubeStorageTypes, "kube-storage-types", mo.kubeStorageTypes, ""+
		"kube-storage-types is the backend storage drivers type, such as: local_pv, cephfs, rbd... ")
	fs.StringVar(&mo.storageDriver, "storage-driver", mo.storageDriver, "storage driver name")
	fs.StringVar(&mo.storageDriverConfigFile, "storage-driver-config-file", mo.storageDriverConfigFile, "Path of the storage driver config file")

	fs.BoolVar(&mo.enableNodeWatcher, "enable-node-watcher", mo.enableNodeWatcher, ""+
		"enable-node-watcher shows whether we need to watch node events")
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
	if len(mo.kubeStorageTypes) == 0 && !mo.enableNodeWatcher {
		glog.Fatalf("either kube-storage-types or enable-node-watcher must be set")
	}
	if len(mo.kubeStorageTypes) > 0 {
		// TODO(@NickrenREN): need to handle this more elegantly
		// check if we support the storage types
		err := checkStorageTypes(mo.kubeStorageTypes)
		if err != nil {
			glog.Errorf("check storage types error: %v", err)
			os.Exit(1)
		}
		// run specific storage monitor
		for _, sType := range mo.kubeStorageTypes {
			switch sType {
			case "local_pv":
				go local_pv.RunLocalPVMonitor()
			case "cinder_pv", "hostpath_pv":
				go remote_pv.RunRemotePVMonitor(mo.storageDriver, mo.storageDriverConfigFile)
			}
		}
	} else {
		go local_pv.RunNodeWatcher()
	}

	<-stopCh
}

var (
	supportedStorageTypes = map[string]bool{}
)

// getSupportedStorageTypes returns the supported storage types
// Add local_pv support at first
// If we get to support other storage types, need to add here too
func getSupportedStorageTypes() {
	supportedStorageTypes["local_pv"] = true
	supportedStorageTypes["cinder_pv"] = true
	supportedStorageTypes["hostpath_pv"] = true
}

func checkStorageTypes(storageTypes []string) error {
	getSupportedStorageTypes()
	for _, sType := range storageTypes {
		if !supportedStorageTypes[sType] {
			return fmt.Errorf("monitor does not support %s storage type now", sType)
		}
	}
	return nil
}
