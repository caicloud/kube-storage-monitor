package cinder

import (
	"fmt"

	"github.com/caicloud/kube-storage-monitor/pkg/cloudprovider/openstack"
	"github.com/caicloud/kube-storage-monitor/pkg/volume"

	"k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/cloudprovider"
)

const (
	pluginName = "cinder"
)

type cinderPlugin struct {
	cloud *openstack.OpenstackMonitor
}

var _ volume.Plugin = &cinderPlugin{}

// Init inits volume plugin
func (c *cinderPlugin) Init(cloud cloudprovider.Interface) {
	c.cloud = cloud.(*openstack.OpenstackMonitor)
}

// RegisterPlugin creates an uninitialized cinder plugin
func RegisterPlugin() volume.Plugin {
	return &cinderPlugin{}
}

// GetPluginName retrieves the name of the plugin
func GetPluginName() string {
	return pluginName
}

func (cinder *cinderPlugin) CheckVolumeStatus(pv *v1.PersistentVolume, configFilePath string) error {
	if pv == nil || pv.Spec.Cinder == nil {
		return fmt.Errorf("invalid Cinder PV: %v", pv)
	}

	volumeID := pv.Spec.Cinder.VolumeID
	return cinder.cloud.CheckVolumeStatus(volumeID)
}
