package util

import (
	"sync"

	"k8s.io/api/core/v1"
)

// VolumeMap is the interface to store volumes
type VolumeMap interface {
	AddUpdateVolume(pv *v1.PersistentVolume)

	DeleteVolume(pv *v1.PersistentVolume)

	GetPVs() []*v1.PersistentVolume
}

type volumesMap struct {
	// for guarding access to pvs map
	sync.RWMutex

	// local storage PV map of unique pv name and pv obj
	volumeMap map[string]*v1.PersistentVolume
}

// NewVolumeMap returns new VolumeMap which acts as a cache
// for holding storage PVs.
func NewVolumeMap() VolumeMap {
	volumeMap := &volumesMap{}
	volumeMap.volumeMap = make(map[string]*v1.PersistentVolume)
	return volumeMap
}

func (lvm *volumesMap) AddUpdateVolume(pv *v1.PersistentVolume) {
	lvm.Lock()
	defer lvm.Unlock()

	lvm.volumeMap[pv.Name] = pv
}

func (lvm *volumesMap) DeleteVolume(pv *v1.PersistentVolume) {
	lvm.Lock()
	defer lvm.Unlock()

	delete(lvm.volumeMap, pv.Name)
}

func (lvm *volumesMap) GetPVs() []*v1.PersistentVolume {
	lvm.Lock()
	defer lvm.Unlock()

	pvs := []*v1.PersistentVolume{}
	for _, pv := range lvm.volumeMap {
		pvs = append(pvs, pv)
	}

	return pvs
}
