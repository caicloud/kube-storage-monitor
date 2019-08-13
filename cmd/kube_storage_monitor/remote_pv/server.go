package remote_pv

import (
	"fmt"

	"github.com/caicloud/kube-storage-monitor/pkg/remote_pv_monitor"
	"github.com/caicloud/kube-storage-monitor/pkg/volume"
	"github.com/caicloud/kube-storage-monitor/pkg/volume/cinder"
	"github.com/caicloud/kube-storage-monitor/pkg/volume/hostpath"
	"github.com/golang/glog"

	"github.com/kubernetes-incubator/external-storage/local-volume/provisioner/pkg/common"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/kubernetes/pkg/cloudprovider"
	"k8s.io/kubernetes/pkg/cloudprovider/providers/openstack"
)

var (
	volumePlugins = make(map[string]volume.Plugin)
)

func RunRemotePVMonitor(storageDriver, storageDriverConfigFile string) {
	err := buildVolumePlugins(storageDriver, storageDriverConfigFile)
	if err != nil {
		glog.Fatalf("build volume plugins error: %v", err)
	}

	client := common.SetupClient()
	remote_pv_monitor.NewRemoteMonitor(client, &volumePlugins, storageDriverConfigFile).Run(wait.NeverStop)
}

func buildVolumePlugins(storageDriver, storageDriverConfigFile string) error {
	if storageDriver == cinder.GetPluginName() {
		cloud, err := cloudprovider.InitCloudProvider(openstack.ProviderName, storageDriverConfigFile)
		if err == nil {
			cinderPlugin := cinder.RegisterPlugin()
			cinderPlugin.Init(cloud)
			volumePlugins[cinder.GetPluginName()] = cinderPlugin
			glog.Infof("Register cloudprovider %s", cinder.GetPluginName())

		} else {
			return fmt.Errorf("failed to initialize cloudprovider: %v", err)
		}
	}
	volumePlugins[hostpath.GetPluginName()] = hostpath.RegisterPlugin()
	return nil
}
