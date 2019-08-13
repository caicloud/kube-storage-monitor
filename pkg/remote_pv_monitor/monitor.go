package remote_pv_monitor

import (
	"fmt"
	"time"

	"github.com/golang/glog"

	"github.com/caicloud/kube-storage-monitor/pkg/util"
	"github.com/caicloud/kube-storage-monitor/pkg/volume"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"

	"k8s.io/kubernetes/pkg/controller"
)

// Monitor checks PVs' health condition and taint them if they are unhealthy
type RemotePVMonitor struct {
	client   *kubernetes.Clientset
	recorder record.EventRecorder

	volumeMap util.VolumeMap

	volumePlugins           *map[string]volume.Plugin
	volumeMonitorConfigPath string

	volumeLister       corelisters.PersistentVolumeLister
	volumeListerSynced cache.InformerSynced
}

func NewRemoteMonitor(client *kubernetes.Clientset, volumePlugins *map[string]volume.Plugin, configFilePath string) *RemotePVMonitor {
	monitorName := fmt.Sprintf("remote-pv-monitor")

	broadcaster := record.NewBroadcaster()
	broadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: v1core.New(client.CoreV1().RESTClient()).Events("")})
	recorder := broadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: monitorName})

	remotePVMonitor := &RemotePVMonitor{
		client:                  client,
		recorder:                recorder,
		volumePlugins:           volumePlugins,
		volumeMonitorConfigPath: configFilePath,
	}
	remotePVMonitor.volumeMap = util.NewVolumeMap()

	informerFactory := informers.NewSharedInformerFactory(client, util.DefaultInformerResyncPeriod)
	volumeInformer := informerFactory.Core().V1().PersistentVolumes()
	volumeInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    remotePVMonitor.addVolume,
			UpdateFunc: remotePVMonitor.updateVolume,
			DeleteFunc: remotePVMonitor.deleteVolume,
		},
	)
	remotePVMonitor.volumeLister = volumeInformer.Lister()
	remotePVMonitor.volumeListerSynced = volumeInformer.Informer().HasSynced

	go informerFactory.Start(wait.NeverStop)

	return remotePVMonitor
}

func (remote *RemotePVMonitor) addVolume(obj interface{}) {
	volume, ok := obj.(*v1.PersistentVolume)
	if !ok {
		glog.Errorf("Expected PersistentVolume but handler received %#v", obj)
		return
	}

	remote.volumeMap.AddUpdateVolume(volume)
}

func (remote *RemotePVMonitor) updateVolume(oldObj, newObj interface{}) {
	volume, ok := newObj.(*v1.PersistentVolume)
	if !ok {
		glog.Errorf("Expected PersistentVolume but handler received %#v", newObj)
		return
	}

	remote.volumeMap.AddUpdateVolume(volume)
}

func (remote *RemotePVMonitor) deleteVolume(obj interface{}) {
	volume, ok := obj.(*v1.PersistentVolume)
	if !ok {
		glog.Errorf("Expected PersistentVolume but handler received %#v", obj)
		return
	}

	remote.volumeMap.DeleteVolume(volume)
}

// resync supplements short resync period of shared informers - we don't want
// all consumers of PV shared informer to have a short resync period,
// therefore we do our own.
func (remote *RemotePVMonitor) resync() {
	glog.V(4).Infof("resyncing remote pv monitor")

	volumes, err := remote.volumeLister.List(labels.NewSelector())
	if err != nil {
		glog.Warningf("cannot list volumes: %s", err)
		return
	}
	for _, volume := range volumes {
		remote.volumeMap.AddUpdateVolume(volume)
	}

	// delete the pv from map if the pv is already deleted
	pvs := remote.volumeMap.GetPVs()
	for _, pv := range pvs {
		_, err := remote.volumeLister.Get(pv.Name)
		if errors.IsNotFound(err) {
			remote.volumeMap.DeleteVolume(pv)
		}
	}
}

func (remote *RemotePVMonitor) Run(stopCh <-chan struct{}) {
	if !controller.WaitForCacheSync("remote-pv-monitor", stopCh, remote.volumeListerSynced) {
		return
	}

	go wait.Until(remote.resync, util.DefaultResyncPeriod, stopCh)
	go remote.CheckStatus()
	<-stopCh
}

func (remote *RemotePVMonitor) CheckStatus() {
	for {
		pvs := remote.volumeMap.GetPVs()
		for _, pv := range pvs {
			go remote.checkVolumeStatus(pv, remote.volumeMonitorConfigPath)
		}

		time.Sleep(util.DefaultResyncPeriod)
	}
}

func (remote *RemotePVMonitor) checkVolumeStatus(pv *v1.PersistentVolume, configFilePath string) {
	volumeType := getSupportedVolumeFromPVSpec(&pv.Spec)
	if len(volumeType) == 0 {
		glog.Errorf("unsupported volume type found in PV %#v", pv.Spec)
		return
	}
	plugin, ok := (*remote.volumePlugins)[volumeType]
	if !ok {
		glog.Errorf("%s is not supported volume for %#v", volumeType, pv.Spec)
		return
	}

	plugin.CheckVolumeStatus(pv, configFilePath)
}

// getSupportedVolumeFromPVSpec gets supported volume from PV spec
func getSupportedVolumeFromPVSpec(spec *v1.PersistentVolumeSpec) string {
	if spec.HostPath != nil {
		return "hostpath"
	}
	if spec.Cinder != nil {
		return "cinder"
	}
	return ""
}
