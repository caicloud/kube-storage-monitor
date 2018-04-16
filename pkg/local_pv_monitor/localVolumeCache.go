package local_pv_monitor

import (
	"sync"

	"k8s.io/api/core/v1"
)

// LocalVolumeMap is the interface to store local volumes
type LocalVolumeMap interface {
	AddLocalVolume(pv *v1.PersistentVolume)

	UpdateLocalVolume(newPV *v1.PersistentVolume)

	DeleteLocalVolume(pv *v1.PersistentVolume)

	GetPVs() []*v1.PersistentVolume
}

type localVolumeMap struct {
	// for guarding access to pvs map
	sync.RWMutex

	// local storage PV map of unique pv name and pv obj
	volumeMap map[string]*v1.PersistentVolume
}

// NewLocalVolumeMap returns new LocalVolumeMap which acts as a cache
// for holding local storage PVs.
func NewLocalVolumeMap() LocalVolumeMap {
	localVolumeMap := &localVolumeMap{}
	localVolumeMap.volumeMap = make(map[string]*v1.PersistentVolume)
	return localVolumeMap
}

// TODO: just add local storage PVs which belongs to the specific node
func (lvm *localVolumeMap) AddLocalVolume(pv *v1.PersistentVolume) {
	lvm.Lock()
	defer lvm.Unlock()

	lvm.volumeMap[pv.Name] = pv
}

func (lvm *localVolumeMap) UpdateLocalVolume(newPV *v1.PersistentVolume) {
	lvm.Lock()
	defer lvm.Unlock()

	lvm.volumeMap[newPV.Name] = newPV
}

func (lvm *localVolumeMap) DeleteLocalVolume(pv *v1.PersistentVolume) {
	lvm.Lock()
	defer lvm.Unlock()

	delete(lvm.volumeMap, pv.Name)
}

func (lvm *localVolumeMap) GetPVs() []*v1.PersistentVolume {
	lvm.Lock()
	defer lvm.Unlock()

	pvs := []*v1.PersistentVolume{}
	for _, pv := range lvm.volumeMap {
		pvs = append(pvs, pv)
	}

	return pvs
}
