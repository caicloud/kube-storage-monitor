package openstack

import (
	"k8s.io/kubernetes/pkg/cloudprovider/providers/openstack"
)

type OpenstackMonitor struct {
	*openstack.OpenStack
}

func (om *OpenstackMonitor) CheckVolumeStatus(volumeID string) error {
	return nil
}
