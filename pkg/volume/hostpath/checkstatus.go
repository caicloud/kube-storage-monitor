package hostpath

import (
	"fmt"

	"github.com/caicloud/kube-storage-monitor/pkg/volume"

	"k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/cloudprovider"
)

const (
	pluginName = "hostpath"
)

type hostPathPlugin struct {
}

var _ volume.Plugin = &hostPathPlugin{}

// Init inits volume plugin
func (c *hostPathPlugin) Init(_ cloudprovider.Interface) {
}

// RegisterPlugin creates an uninitialized hostpath plugin
func RegisterPlugin() volume.Plugin {
	return &hostPathPlugin{}
}

// GetPluginName retrieves the name of the plugin
func GetPluginName() string {
	return pluginName
}

func (hostpath *hostPathPlugin) CheckVolumeStatus(pv *v1.PersistentVolume, configFilePath string) error {
	if pv == nil || pv.Spec.HostPath == nil {
		return fmt.Errorf("invalid HostPath PV: %v", pv)
	}

	return nil
}
