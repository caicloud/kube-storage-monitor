package node_failure_controller

import (
	"fmt"
	"time"

	"github.com/golang/glog"

	"github.com/caicloud/kube-storage-monitor/pkg/util"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	"k8s.io/kubernetes/pkg/controller"
)

// NodeWatcher watches nodes conditions
type NodeWatcher struct {
	client   *kubernetes.Clientset
	recorder record.EventRecorder

	nodeQueue *workqueue.Type
	nodeMap   NodeMap

	nodeLister       corelisters.NodeLister
	nodeListerSynced cache.InformerSynced

	volumeLister       corelisters.PersistentVolumeLister
	volumeListerSynced cache.InformerSynced

	// mark the time when we first found the node is broken
	nodeFirstBrokenMap map[string]time.Time
}

// NewNodeWatcher creates a node watcher object that will watch the nodes
func NewNodeWatcher(client *kubernetes.Clientset) *NodeWatcher {

	watcherName := fmt.Sprintf("node-watcher")

	broadcaster := record.NewBroadcaster()
	broadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: v1core.New(client.CoreV1().RESTClient()).Events("")})
	recorder := broadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: watcherName})

	watcher := &NodeWatcher{
		client:             client,
		recorder:           recorder,
		nodeQueue:          workqueue.NewNamed("nodes"),
		nodeFirstBrokenMap: make(map[string]time.Time),
	}
	watcher.nodeMap = NewNodeMap()

	informerFactory := informers.NewSharedInformerFactory(client, util.DefaultInformerResyncPeriod)
	nodeInformer := informerFactory.Core().V1().Nodes()
	nodeInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) { watcher.enqueueWork(watcher.nodeQueue, obj) },
			UpdateFunc: func(oldObj, newObj interface{}) {
				watcher.enqueueWork(watcher.nodeQueue, newObj)
			},
			DeleteFunc: func(obj interface{}) {
				watcher.enqueueWork(watcher.nodeQueue, obj)
			},
		},
	)
	watcher.nodeLister = nodeInformer.Lister()
	watcher.nodeListerSynced = nodeInformer.Informer().HasSynced

	volumeInformer := informerFactory.Core().V1().PersistentVolumes()
	watcher.volumeLister = volumeInformer.Lister()
	watcher.volumeListerSynced = volumeInformer.Informer().HasSynced

	go informerFactory.Start(wait.NeverStop)

	// fill map at first with data from ETCD
	watcher.flushFromETCDFirst()

	return watcher
}

// enqueueWork adds node to given work queue.
func (watcher *NodeWatcher) enqueueWork(queue workqueue.Interface, obj interface{}) {
	// Beware of "xxx deleted" events
	if unknown, ok := obj.(cache.DeletedFinalStateUnknown); ok && unknown.Obj != nil {
		obj = unknown.Obj
	}
	objName, err := controller.KeyFunc(obj)
	if err != nil {
		glog.Errorf("failed to get key from object: %v", err)
		return
	}
	glog.V(6).Infof("enqueued %q for sync", objName)
	queue.Add(objName)
}

// flushFromETCDFirst fill map with data from etcd at first
func (watcher *NodeWatcher) flushFromETCDFirst() error {
	nodes, err := watcher.client.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	if len(nodes.Items) == 0 {
		glog.Infof("no nodes in ETCD")
		return nil
	}

	for _, node := range nodes.Items {
		nodeClone := node.DeepCopy()
		objName, err := controller.KeyFunc(nodeClone)
		if err != nil {
			return fmt.Errorf("couldn't get key for object %+v: %v", nodeClone, err)
		}
		glog.Infof("add node:%s from etcd", objName)
		watcher.nodeMap.UpdateNode(objName, nodeClone)
		watcher.enqueueWork(watcher.nodeQueue, nodeClone)
	}
	return nil
}

// resync supplements short resync period of shared informers - we don't want
// all consumers of Node shared informer to have a short resync period,
// therefore we do our own.
func (watcher *NodeWatcher) resync() {
	glog.V(4).Infof("resyncing Node watcher")

	nodes, err := watcher.nodeLister.List(labels.NewSelector())
	if err != nil {
		glog.Warningf("cannot list nodes: %s", err)
		return
	}
	for _, node := range nodes {
		watcher.enqueueWork(watcher.nodeQueue, node)
	}
}

// Run starts all of this controller's control loops
func (watcher *NodeWatcher) Run(stopCh <-chan struct{}) {
	defer watcher.nodeQueue.ShutDown()
	if !controller.WaitForCacheSync("node watcher", stopCh, watcher.nodeListerSynced, watcher.volumeListerSynced) {
		return
	}

	//go watcher.WatchNodes()
	go wait.Until(watcher.resync, util.DefaultResyncPeriod, stopCh)
	go wait.Until(watcher.WatchNodes, util.DefaultResyncPeriod, stopCh)
	<-stopCh
}

// WatchNodes periodically checks if nodes break down
func (watcher *NodeWatcher) WatchNodes() {
	workFunc := func() bool {
		keyObj, quit := watcher.nodeQueue.Get()
		if quit {
			return true
		}
		defer watcher.nodeQueue.Done(keyObj)
		key := keyObj.(string)
		glog.V(4).Infof("volumeWorker[%s]", key)

		_, name, err := cache.SplitMetaNamespaceKey(key)
		if err != nil {
			glog.Errorf("error getting name of node %q from informer: %v", key, err)
			return false
		}
		node, err := watcher.nodeLister.Get(name)
		if err == nil {
			// The node still exists in informer cache, the event must have
			// been add/update/sync
			watcher.updateNode(key, node)
			return false
		}
		if !errors.IsNotFound(err) {
			glog.V(2).Infof("error getting node %q from informer: %v", key, err)
			return false
		}

		// The node is not in informer cache, the event must be
		// "delete"
		nodeObj := watcher.nodeMap.GetNode(key)
		if nodeObj == nil {
			// The controller has already processed the delete event and
			// deleted the node from its cache
			glog.Infof("deletion of node %q was already processed", key)
			return false
		}
		watcher.deleteNode(key, nodeObj)
		return false
	}
	for {
		if quit := workFunc(); quit {
			glog.Infof("volume worker queue shutting down")
			return
		}
	}
}

func (watcher *NodeWatcher) updateNode(key string, node *v1.Node) {
	watcher.nodeMap.UpdateNode(key, node)

	// need to revisit this later
	if watcher.isNodeBroken(node) {
		glog.Infof("node: %s is broken", node.Name)
		// mark all local PVs on this node
		// try several times again
		var err error
		for i := 0; i < util.UpdatePVRetryCount; i++ {
			err = watcher.markLocalPVsOnNode(node, util.NodeFailure, "true")
			if err != nil {
				glog.V(4).Infof("marking local PVs failed: %v", err)
				time.Sleep(util.UpdatePVInterval)
				continue
			}
			break
		}
		if err != nil {
			glog.Infof("mark local PV failed, re-enqueue")
			// if error happened, re-enqueue
			watcher.enqueueWork(watcher.nodeQueue, node)
			return
		}
		// node watcher consider this node is broken, and have marked the local PVs on it,
		// so remove this node from the map cache
		watcher.nodeMap.DeleteNode(key)

		// node is broken and local PVs are marked, remove it from map
		objName, err := controller.KeyFunc(node)
		if err != nil {
			glog.Errorf("failed to get key from object: %v", err)
			return
		}
		_, ok := watcher.nodeFirstBrokenMap[objName]
		if ok {
			delete(watcher.nodeFirstBrokenMap, objName)
		}
	}

}

func (watcher *NodeWatcher) isNodeBroken(node *v1.Node) bool {
	if node.Status.Phase == v1.NodeTerminated {
		return true
	}
	objName, err := controller.KeyFunc(node)
	if err != nil {
		glog.Errorf("failed to get key from object: %v", err)
		watcher.enqueueWork(watcher.nodeQueue, node)
		return false
	}

	for _, condition := range node.Status.Conditions {
		if condition.Type == v1.NodeReady && condition.Status != v1.ConditionTrue {
			now := time.Now()
			firstMarkTime, ok := watcher.nodeFirstBrokenMap[objName]
			if ok {
				timeInterval := now.Sub(firstMarkTime)
				if timeInterval.Seconds() > util.DefaultNodeNotReadyTimeDuration.Seconds() {
					return true
				} else {
					glog.V(6).Infof("node:%s is not ready, but less than 2 minutes, re-enqueue", node.Name)
					// NotReady status lasts less than 2 minutes
					// re-enqueue
					watcher.enqueueWork(watcher.nodeQueue, node)
					return false
				}
			} else {
				// first time to mark the node NotReady
				watcher.nodeFirstBrokenMap[objName] = now
				watcher.enqueueWork(watcher.nodeQueue, node)
				return false
			}
		}
	}

	// The node status is ok, but if it was marked before, remove the mark
	_, ok := watcher.nodeFirstBrokenMap[objName]
	if ok {
		delete(watcher.nodeFirstBrokenMap, objName)
	}
	return false
}

func (watcher *NodeWatcher) deleteNode(key string, node *v1.Node) {
	glog.Infof("node:%s is deleted, so mark the local PVs on it", node.Name)
	watcher.nodeMap.DeleteNode(key)

	// mark all local PVs on this node
	// try several times again
	for i := 0; i < util.UpdatePVRetryCount; i++ {
		err := watcher.markLocalPVsOnNode(node, util.NodeFailure, "true")
		if err != nil {
			glog.V(4).Infof("marking local PVs failed: %v", err)
			time.Sleep(util.UpdatePVInterval)
			continue
		}
		return
	}

	// when we reach here, means that marking local failed, re-enqueue
	watcher.enqueueWork(watcher.nodeQueue, node)
}

func (watcher *NodeWatcher) markLocalPVsOnNode(node *v1.Node, ann, val string) error {
	pvs, err := watcher.volumeLister.List(labels.NewSelector())
	if err != nil {
		glog.Warningf("cannot list pvs: %s", err)
		return err
	}

	var markErrHappened bool
	for _, pv := range pvs {
		// if the pv is not local pv, skip
		if pv.Spec.Local == nil {
			continue
		}
		// check node and pv affinity
		fit, err := util.CheckNodeAffinity(pv, node.Labels)
		if err != nil {
			glog.Errorf("check node affinity error: %v", err)
			return err
		}
		if !fit {
			glog.Errorf("pv: %s does not belong to this node: %s", pv.Name, node.Name)
			continue
		}

		err = util.MarkPV(watcher.client, watcher.recorder, pv, ann, val, nil)
		if err != nil {
			markErrHappened = true
			glog.Errorf("mark local PV: %s failed, error: %v", pv.Name, err)
		}
	}

	if markErrHappened {
		return fmt.Errorf("error happened when marking local PVs")
	}
	return nil
}
