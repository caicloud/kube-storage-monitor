package volume

import (
	"k8s.io/api/core/v1"

	"k8s.io/kubernetes/pkg/cloudprovider"
)

// Plugin defines functions that should be implemented by the volume plugin
type Plugin interface {
	// Init inits volume plugin
	Init(cloudprovider.Interface)

	// CheckVolumeStatus checks if the specific volume is healthy or not
	CheckVolumeStatus(pv *v1.PersistentVolume, configFilePath string) error
}
