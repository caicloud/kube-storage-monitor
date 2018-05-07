package local_pv_monitor

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/caicloud/kube-storage-monitor/pkg/util"
	"github.com/golang/glog"
	lvcache "github.com/kubernetes-incubator/external-storage/local-volume/provisioner/pkg/cache"
	"github.com/kubernetes-incubator/external-storage/local-volume/provisioner/pkg/common"
	lvutil "github.com/kubernetes-incubator/external-storage/local-volume/provisioner/pkg/util"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/pkg/apis/core/v1/helper"
	"k8s.io/kubernetes/pkg/util/mount"
)

const (
	// DefaultInformerResyncPeriod is the resync period of informer
	DefaultInformerResyncPeriod = 15 * time.Second

	// DefaultMonitorResyncPeriod is the resync period of monitor
	DefaultMonitorResyncPeriod = 1 * time.Minute

	// UpdatePVRetryCount is the retry count of PV updating
	UpdatePVRetryCount = 5

	// UpdatePVInterval is the interval of PV updating
	UpdatePVInterval = 5 * time.Millisecond
)

// marking event related const vars
const (
	MarkPVFailed    = "MarkPVFailed"
	MarkPVSucceeded = "MarkPVSucceeded"

	HostPathNotExist  = "HostPathNotExist"
	MisMatchedVolSize = "MisMatchedVolSize"
	NotMountPoint     = "NotMountPoint"

	FirstMarkTime = "FirstMarkTime"
)

// PVUnhealthyKeys stores all the unhealthy marking keys
var PVUnhealthyKeys []string

func init() {
	PVUnhealthyKeys = append(PVUnhealthyKeys, HostPathNotExist)
	PVUnhealthyKeys = append(PVUnhealthyKeys, MisMatchedVolSize)
	PVUnhealthyKeys = append(PVUnhealthyKeys, NotMountPoint)
}

// Monitor checks PVs' health condition and taint them if they are unhealthy
type LocalPVMonitor struct {
	config *MonitorConfiguration

	//*common.RuntimeConfig
	*common.RuntimeConfig

	volumeLW         cache.ListerWatcher
	volumeController cache.Controller

	localVolumeMap LocalVolumeMap

	hasRun     bool
	hasRunLock *sync.Mutex
}

// NewMonitor creates a monitor object that will scan through
// the configured directories and check volume status
func NewLocalPVMonitor(client *kubernetes.Clientset, config *common.UserConfig, monitorConfig *MonitorConfiguration) *LocalPVMonitor {

	monitorName := fmt.Sprintf("local-volume-monitor-%v-%v", config.Node.Name, config.Node.UID)

	broadcaster := record.NewBroadcaster()
	broadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: v1core.New(client.CoreV1().RESTClient()).Events("")})
	recorder := broadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: monitorName})

	runtimeConfig := &common.RuntimeConfig{
		UserConfig: config,
		Cache:      lvcache.NewVolumeCache(),
		VolUtil:    lvutil.NewVolumeUtil(),
		APIUtil:    lvutil.NewAPIUtil(client),
		Client:     client,
		Name:       monitorName,
		Recorder:   recorder,
		Mounter:    mount.New("" /* default mount path */),
	}

	monitor := &LocalPVMonitor{
		config:        monitorConfig,
		RuntimeConfig: runtimeConfig,
		hasRun:        false,
		hasRunLock:    &sync.Mutex{},
	}

	labelOps := metav1.ListOptions{
		LabelSelector: labels.Everything().String(),
	}
	if len(monitor.config.LabelSelectorForPV) > 0 {
		labelOps.LabelSelector = monitor.config.LabelSelectorForPV
	}

	monitor.localVolumeMap = NewLocalVolumeMap()

	monitor.volumeLW = &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return runtimeConfig.Client.CoreV1().PersistentVolumes().List(labelOps)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return runtimeConfig.Client.CoreV1().PersistentVolumes().Watch(labelOps)
		},
	}
	_, monitor.volumeController = cache.NewInformer(
		monitor.volumeLW,
		&v1.PersistentVolume{},
		DefaultInformerResyncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    monitor.addVolume,
			UpdateFunc: monitor.updateVolume,
			DeleteFunc: monitor.deleteVolume,
		},
	)

	// fill map at first with data from ETCD
	monitor.flushFromETCDFirst()

	return monitor
}

// flushFromETCDFirst fill map with data from etcd at first
func (monitor *LocalPVMonitor) flushFromETCDFirst() error {
	pvs, err := monitor.Client.CoreV1().PersistentVolumes().List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	if len(pvs.Items) == 0 {
		glog.Infof("no pv in ETCD at first")
		return nil
	}

	for _, pv := range pvs.Items {
		monitor.localVolumeMap.AddLocalVolume(&pv)
	}
	return nil
}

func (monitor *LocalPVMonitor) addVolume(obj interface{}) {
	volume, ok := obj.(*v1.PersistentVolume)
	if !ok {
		glog.Errorf("Expected PersistentVolume but handler received %#v", obj)
		return
	}

	monitor.localVolumeMap.AddLocalVolume(volume)

}

func (monitor *LocalPVMonitor) updateVolume(oldObj, newObj interface{}) {
	newVolume, ok := newObj.(*v1.PersistentVolume)
	if !ok {
		glog.Errorf("Expected PersistentVolume but handler received %#v", newObj)
		return
	}

	monitor.localVolumeMap.UpdateLocalVolume(newVolume)
}

func (monitor *LocalPVMonitor) deleteVolume(obj interface{}) {
	volume, ok := obj.(*v1.PersistentVolume)
	if !ok {
		glog.Errorf("Expected PersistentVolume but handler received %#v", obj)
		return
	}

	monitor.localVolumeMap.DeleteLocalVolume(volume)

}

// Run starts all of this controller's control loops
func (monitor *LocalPVMonitor) Run(stopCh <-chan struct{}) {
	// glog.Infof("Starting local volume monitor %s!", string(monitor.RuntimeConfig.Name))
	monitor.hasRunLock.Lock()
	monitor.hasRun = true
	monitor.hasRunLock.Unlock()
	go monitor.volumeController.Run(stopCh)

	go monitor.MonitorLocalVolumes()
	<-stopCh
}

// HasRun returns whether the volume controller has Run
func (monitor *LocalPVMonitor) HasRun() bool {
	monitor.hasRunLock.Lock()
	defer monitor.hasRunLock.Unlock()
	return monitor.hasRun
}

// MonitorLocalVolumes checks local PVs periodically
func (monitor *LocalPVMonitor) MonitorLocalVolumes() {
	for {
		if monitor.HasRun() {
			pvs := monitor.localVolumeMap.GetPVs()
			for _, pv := range pvs {
				monitor.checkStatus(pv)
			}
		}

		time.Sleep(DefaultMonitorResyncPeriod)
	}
}

// checkStatus checks pv health condition
func (monitor *LocalPVMonitor) checkStatus(pv *v1.PersistentVolume) {
	// check if PV is local storage
	if pv.Spec.Local == nil {
		glog.Infof("PV: %s is not local storage", pv.Name)
		return
	}
	// check node and pv affinity
	fit, err := CheckNodeAffinity(pv, monitor.Node.Labels)
	if err != nil {
		glog.Errorf("check node affinity error: %v", err)
		return
	}
	if !fit {
		glog.Errorf("pv: %s does not belong to this node: %s", pv.Name, monitor.Node.Name)
		return
	}

	// check if host dir still exists
	mountPath, continueThisCheck := monitor.checkHostDir(pv)
	if !continueThisCheck {
		glog.Errorf("Host dir is modified, PV should be marked")
		return
	}

	// check if it is still a mount point
	continueThisCheck = monitor.checkMountPoint(mountPath, pv)
	if !continueThisCheck {
		glog.Errorf("Retrieving mount points error or %s is not a mount point any more", mountPath)
		return
	}

	// check PV size: PV capacity must not be greater than device capacity and PV used bytes must not be greater that PV capacity
	if pv.Spec.VolumeMode != nil && *pv.Spec.VolumeMode == v1.PersistentVolumeBlock {
		monitor.checkPVAndBlockSize(mountPath, pv)
	} else {
		monitor.checkPVAndFSSize(mountPath, pv)
	}

}

func (monitor *LocalPVMonitor) checkMountPoint(mountPath string, pv *v1.PersistentVolume) bool {
	// Retrieve list of mount points to iterate through discovered paths (aka files) below
	mountPoints, mountPointsErr := monitor.RuntimeConfig.Mounter.List()
	if mountPointsErr != nil {
		glog.Errorf("Error retrieving mount points: %v", mountPointsErr)
		return false
	}
	// Check if mountPath is still a mount point
	for _, mp := range mountPoints {
		if mp.Path == mountPath {
			glog.V(10).Infof("mountPath is still a mount point: %s", mountPath)
			return true
		}
	}

	glog.V(6).Infof("mountPath is not a mount point any more: %s", mountPath)
	err := monitor.markPV(pv, NotMountPoint, "yes")
	if err != nil {
		glog.Errorf("mark PV: %s failed, err: %v", pv.Name, err)
	}
	return false

}

func (monitor *LocalPVMonitor) checkHostDir(pv *v1.PersistentVolume) (mountPath string, continueThisCheck bool) {
	var err error
	for _, config := range monitor.DiscoveryMap {
		if strings.Contains(pv.Spec.Local.Path, config.HostDir) {
			mountPath, err = common.GetContainerPath(pv, config)
			if err != nil {
				glog.Errorf("get container path error: %v", err)
			}
			break
		}
	}
	if len(mountPath) == 0 {
		// can not find mount path, this may because: admin modify config(hostpath)
		// mark PV and send a event
		err = monitor.markPV(pv, HostPathNotExist, "yes")
		if err != nil {
			glog.Errorf("mark PV: %s failed, err: %v", pv.Name, err)
		}
		return
	}
	dir, dirErr := monitor.VolUtil.IsDir(mountPath)
	bl, blErr := monitor.VolUtil.IsBlock(mountPath)
	if !dir && !bl && (dirErr != nil || blErr != nil) {
		// mountPath does not exist or is not a directory
		// mark PV and send a event
		err = monitor.markPV(pv, HostPathNotExist, "yes")
		if err != nil {
			glog.Errorf("mark PV: %s failed, err: %v", pv.Name, err)
		}
		return
	}
	continueThisCheck = true
	return

}

func (monitor *LocalPVMonitor) checkPVAndFSSize(mountPath string, pv *v1.PersistentVolume) {
	capacityByte, err := monitor.VolUtil.GetFsCapacityByte(mountPath)
	if err != nil {
		glog.Errorf("Path %q fs stats error: %v", mountPath, err)
		return
	}
	// actually if PV is provisioned dynamically by provisioner, the two values must be equal, but the PV may be
	// created manually, so the PV capacity must not be greater than FS capacity
	storage := pv.Spec.Capacity[v1.ResourceStorage]
	if storage.Value() > util.RoundDownCapacityPretty(capacityByte) {
		glog.Errorf("PV capacity must not be greater that FS capacity, PV capacity: %v, FS capacity: %v", storage.Value(), util.RoundDownCapacityPretty(capacityByte))
		// mark PV and send a event
		err = monitor.markPV(pv, MisMatchedVolSize, "yes")
		if err != nil {
			glog.Errorf("mark PV: %s failed, err: %v", pv.Name, err)
		}
		return
	}
	// make sure that PV usage is not greater than PV capacity
	usage, err := util.GetDirUsageByte(mountPath)
	if err != nil {
		glog.Errorf("Path %q fs stats error: %v", mountPath, err)
		return
	}
	if usage.Value() > storage.Value() {
		glog.Errorf("PV usage must not be greater than PV capacity, usage: %v, capacity: %v", usage.Value(), storage.Value())
		// mark PV and send a event
		err = monitor.markPV(pv, MisMatchedVolSize, "yes")
		if err != nil {
			glog.Errorf("mark PV: %s failed, err: %v", pv.Name, err)
		}
		return
	}
}

func (monitor *LocalPVMonitor) checkPVAndBlockSize(mountPath string, pv *v1.PersistentVolume) {
	capacityByte, err := monitor.VolUtil.GetBlockCapacityByte(mountPath)
	if err != nil {
		glog.Errorf("Path %q block stats error: %v", mountPath, err)
		return
	}
	// actually if PV is provisioned dynamically by provisioner, the two values must be equal, but the PV may be
	// created manually, so the PV capacity must not be greater than block device capacity
	storage := pv.Spec.Capacity[v1.ResourceStorage]
	if storage.Value() > util.RoundDownCapacityPretty(capacityByte) {
		glog.Errorf("PV capacity must not be greater that FS capacity, PV capacity: %v, FS capacity: %v", storage.Value(), util.RoundDownCapacityPretty(capacityByte))
		// mark PV and send a event
		err = monitor.markPV(pv, MisMatchedVolSize, "yes")
		if err != nil {
			glog.Errorf("mark PV: %s failed, err: %v", pv.Name, err)
		}
		return
	}

	// make sure that PV usage is not greater than PV capacity
	// we can not get raw block device usage for now, so skip this check

	return
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

// markPV marks PV by adding annotation
func (monitor *LocalPVMonitor) markPV(pv *v1.PersistentVolume, ann, value string) error {
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
		newVol, err = monitor.Client.CoreV1().PersistentVolumes().Update(volumeClone)
		if err != nil {
			glog.V(4).Infof("updating PersistentVolume[%s] failed: %v", volumeClone.Name, err)
			time.Sleep(UpdatePVInterval)
			continue
		}
		monitor.localVolumeMap.UpdateLocalVolume(newVol)
		glog.V(4).Infof("updating PersistentVolume[%s] successfully", newVol.Name)
		eventMes = "Mark PV successfully with annotation key: " + ann
		monitor.Recorder.Event(pv, v1.EventTypeNormal, MarkPVSucceeded, eventMes)

		return nil
	}

	eventMes = "Failed to Mark PV with annotation key: " + ann
	monitor.Recorder.Event(pv, v1.EventTypeWarning, MarkPVFailed, "Failed to Mark PV")

	return err
}
