package util

import (
	"fmt"
	"time"

	esutil "github.com/kubernetes-incubator/external-storage/lib/util"

	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/pkg/apis/core/v1/helper"
	"k8s.io/kubernetes/pkg/volume/util"
)

const (
	// DefaultInformerResyncPeriod is the resync period of informer
	DefaultInformerResyncPeriod = 5 * time.Second

	// DefaultMonitorResyncPeriod is the resync period
	DefaultResyncPeriod = 30 * time.Second

	// UpdatePVRetryCount is the retry count of PV updating
	UpdatePVRetryCount = 5

	// UpdatePVInterval is the interval of PV updating
	UpdatePVInterval = 5 * time.Millisecond

	// DefaultNodeNotReadyTimeDuration is the default time interval we need to consider node broken if it keeps NotReady
	DefaultNodeNotReadyTimeDuration = 120 * time.Second
)

// marking event related const vars
const (
	MarkPVFailed    = "MarkPVFailed"
	MarkPVSucceeded = "MarkPVSucceeded"

	HostPathNotExist  = "HostPathNotExist"
	MisMatchedVolSize = "MisMatchedVolSize"
	NotMountPoint     = "NotMountPoint"

	NodeFailure = "NodeFailure"

	FirstMarkTime = "FirstMarkTime"
)

// RoundDownCapacityPretty rounds down the capacity to an easy to read value.
func RoundDownCapacityPretty(capacityBytes int64) int64 {
	easyToReadUnitsBytes := []int64{esutil.GiB, esutil.MiB}

	// Round down to the nearest easy to read unit
	// such that there are at least 10 units at that size.
	for _, easyToReadUnitBytes := range easyToReadUnitsBytes {
		// Round down the capacity to the nearest unit.
		size := capacityBytes / easyToReadUnitBytes
		if size >= 10 {
			return size * easyToReadUnitBytes
		}
	}
	return capacityBytes
}

// GetDirUsageByte returns usage in bytes about a mounted filesystem.
// fullPath is the pathname of any file within the mounted filesystem. Usage
// returned here is block being used * fragment size (aka block size).
func GetDirUsageByte(fullPath string) (*resource.Quantity, error) {
	usage, err := util.Du(fullPath)
	return usage, err
}

// CheckNodeAffinity looks at the PV node affinity, and checks if the node has the same corresponding labels
// This ensures that we don't mount a volume that doesn't belong to this node
func CheckNodeAffinity(pv *v1.PersistentVolume, nodeLabels map[string]string) (bool, error) {
	fit, err := checkAlphaNodeAffinity(pv, nodeLabels)
	if err != nil {
		return false, err
	}
	if fit {
		return true, nil
	}
	return checkVolumeNodeAffinity(pv, nodeLabels)
}

func checkAlphaNodeAffinity(pv *v1.PersistentVolume, nodeLabels map[string]string) (bool, error) {
	affinity, err := helper.GetStorageNodeAffinityFromAnnotation(pv.Annotations)
	if err != nil {
		return false, fmt.Errorf("error getting storage node affinity: %v", err)
	}
	if affinity == nil {
		return false, nil
	}

	if affinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
		terms := affinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
		glog.V(10).Infof("Match for RequiredDuringSchedulingIgnoredDuringExecution node selector terms %+v", terms)
		for _, term := range terms {
			selector, err := helper.NodeSelectorRequirementsAsSelector(term.MatchExpressions)
			if err != nil {
				return false, fmt.Errorf("failed to parse MatchExpressions: %v", err)
			}
			if !selector.Matches(labels.Set(nodeLabels)) {
				return false, fmt.Errorf("NodeSelectorTerm %+v does not match node labels", term.MatchExpressions)
			}
		}
	}
	return true, nil
}

func checkVolumeNodeAffinity(pv *v1.PersistentVolume, nodeLabels map[string]string) (bool, error) {
	if pv.Spec.NodeAffinity == nil {
		return false, nil
	}

	if pv.Spec.NodeAffinity.Required != nil {
		terms := pv.Spec.NodeAffinity.Required.NodeSelectorTerms
		glog.V(10).Infof("Match for Required node selector terms %+v", terms)
		for _, term := range terms {
			selector, err := helper.NodeSelectorRequirementsAsSelector(term.MatchExpressions)
			if err != nil {
				return false, fmt.Errorf("failed to parse MatchExpressions: %v", err)
			}
			if !selector.Matches(labels.Set(nodeLabels)) {
				return false, fmt.Errorf("NodeSelectorTerm %+v does not match node labels", term.MatchExpressions)
			}
		}
	}
	return true, nil
}

// MarkPV marks PV by adding annotation
func MarkPV(client *kubernetes.Clientset, recorder record.EventRecorder, pv *v1.PersistentVolume, ann, value string, volumeMap VolumeMap) error {
	// The volume from method args can be pointing to watcher cache. We must not
	// modify these, therefore create a copy.
	volumeClone := pv.DeepCopy()
	var eventMes string

	// mark PV
	_, ok := volumeClone.ObjectMeta.Annotations[ann]
	if ok {
		glog.V(10).Infof("PV: %s is already marked with ann: %s", volumeClone.Name, ann)
		return nil
	}
	metav1.SetMetaDataAnnotation(&volumeClone.ObjectMeta, ann, value)
	_, ok = volumeClone.ObjectMeta.Annotations[FirstMarkTime]
	if !ok {
		firstMarkTime := time.Now()
		metav1.SetMetaDataAnnotation(&volumeClone.ObjectMeta, FirstMarkTime, firstMarkTime.String())
	}

	var err error
	var newVol *v1.PersistentVolume
	// Try to update the PV object several times
	for i := 0; i < UpdatePVRetryCount; i++ {
		glog.V(4).Infof("try to update PV: %s", pv.Name)
		newVol, err = client.CoreV1().PersistentVolumes().Update(volumeClone)
		if err != nil {
			glog.V(4).Infof("updating PersistentVolume[%s] failed: %v", volumeClone.Name, err)
			time.Sleep(UpdatePVInterval)
			continue
		}
		if volumeMap != nil {
			volumeMap.AddUpdateVolume(newVol)
		}
		glog.V(4).Infof("updating PersistentVolume[%s] successfully", newVol.Name)
		eventMes = "Mark PV successfully with annotation key: " + ann
		recorder.Event(pv, v1.EventTypeNormal, MarkPVSucceeded, eventMes)

		return nil
	}

	eventMes = "Failed to Mark PV with annotation key: " + ann
	recorder.Event(pv, v1.EventTypeWarning, MarkPVFailed, "Failed to Mark PV")

	return err
}
