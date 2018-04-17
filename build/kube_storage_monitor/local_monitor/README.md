# Local PV monitor

Below is how to deploy monitor for local PVs

## Deployment

### Start local storage provisioner
The hostpath config of the provisioner is `/mnt/disks/vol/` and there is one
subdirectory `vol1` of it. So the provisioner will create a local pv automatically.

Below is the local PV

``` kubectl describe pv example-local-pv-1
Name:            example-local-pv-1
Labels:          <none>
Annotations:     volume.alpha.kubernetes.io/node-affinity={ "requiredDuringSchedulingIgnoredDuringExecution": { "nodeSelectorTerms": [ { "matchExpressions": [ { "key": "kubernetes.io/hostname", "operator": "In", "valu...
Finalizers:      [kubernetes.io/pv-protection]
StorageClass:    local-disks
Status:          Available
Claim:
Reclaim Policy:  Retain
Access Modes:    RWO
Capacity:        200Mi
Node Affinity:   <none>
Message:
Source:
    Type:  LocalVolume (a persistent volume backed by local storage on a node)
    Path:  /mnt/disks/vol/vol1
Events:    <none>
```

### Create ServiceAccount and ConfigMaps for monitor and start monitor Daemonset.
Notice that, here provisioner configmap is the same as local pv provisioner.
``` console
kubectl create -f kubernetes/admin-account.yaml
kubectl create -f kubernetes/configmap.yaml
kubectl create -f kubernetes/monitorconfig.yaml
kubectl create -f kubernetes/monitor.yaml
```

### Describe the kubernetes Daemonset to see if it succeeds
``` kubectl get daemonset
NAME                   DESIRED   CURRENT   READY     UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
local-volume-monitor   1         1         1         1            1           <none>          2m
```
``` kubectl describe daemonset local-volume-monitor
Name:           local-volume-monitor
Selector:       app=local-volume-monitor
Node-Selector:  <none>
Labels:         app=local-volume-monitor
Annotations:    <none>
Desired Number of Nodes Scheduled: 1
Current Number of Nodes Scheduled: 1
Number of Nodes Scheduled with Up-to-date Pods: 1
Number of Nodes Scheduled with Available Pods: 1
Number of Nodes Misscheduled: 0
Pods Status:  1 Running / 0 Waiting / 0 Succeeded / 0 Failed
Pod Template:
  Labels:           app=local-volume-monitor
  Service Account:  local-storage-admin
  Containers:
   monitor:
    Image:      quay.io/kube_storage_monitor/storage-monitor:latest
    Port:       <none>
    Host Port:  <none>
    Args:
      --kube-storage-types=local_pv
    Environment:
      MY_NODE_NAME:   (v1:spec.nodeName)
    Mounts:
      /etc/monitor/config from monitor-config (rw)
      /etc/provisioner/config from provisioner-config (ro)
      /mnt/disks/vol from local-disks (rw)
  Volumes:
   monitor-config:
    Type:      ConfigMap (a volume populated by a ConfigMap)
    Name:      local-monitor-config
    Optional:  false
   provisioner-config:
    Type:      ConfigMap (a volume populated by a ConfigMap)
    Name:      local-provisioner-config
    Optional:  false
   local-disks:
    Type:          HostPath (bare host directory volume)
    Path:          /mnt/disks/vol
    HostPathType:
Events:
  Type    Reason            Age   From                  Message
  ----    ------            ----  ----                  -------
  Normal  SuccessfulCreate  3m    daemonset-controller  Created pod: local-volume-monitor-rnr68
```
If error occurs, we can use `kubectl get pods` and `kubectl logs $podID` to see the error log and debug

### Unmount the mountpoint and delete the hostpath to see if they are detected
```
umount /mnt/disks/vol/vol1/
```
``` kubectl describe pv example-local-pv-1
Name:            example-local-pv-1
Labels:          <none>
Annotations:     FirstMarkTime=2018-04-17 07:31:02.388570492 +0000 UTC m=+600.033905921
                 NotMountPoint=yes
                 volume.alpha.kubernetes.io/node-affinity={ "requiredDuringSchedulingIgnoredDuringExecution": { "nodeSelectorTerms": [ { "matchExpressions": [ { "key": "kubernetes.io/hostname", "operator": "In", "valu...
Finalizers:      [kubernetes.io/pv-protection]
StorageClass:    local-disks
Status:          Available
Claim:
Reclaim Policy:  Retain
Access Modes:    RWO
Capacity:        200Mi
Node Affinity:   <none>
Message:
Source:
    Type:  LocalVolume (a persistent volume backed by local storage on a node)
    Path:  /mnt/disks/vol/vol1
Events:
  Type    Reason           Age   From                                                                 Message
  ----    ------           ----  ----                                                                 -------
  Normal  MarkPVSucceeded  54s   local-volume-monitor-127.0.0.1-40a8fb4d-4206-11e8-8e52-080027765304  Mark PV successfully with annotation key: NotMountPoint
```
```
rm -rf /mnt/disks/vol/vol1/
```
```
Name:            example-local-pv-1
Labels:          <none>
Annotations:     FirstMarkTime=2018-04-17 07:31:02.388570492 +0000 UTC m=+600.033905921
                 HostPathNotExist=yes
                 NotMountPoint=yes
                 volume.alpha.kubernetes.io/node-affinity={ "requiredDuringSchedulingIgnoredDuringExecution": { "nodeSelectorTerms": [ { "matchExpressions": [ { "key": "kubernetes.io/hostname", "operator": "In", "valu...
Finalizers:      [kubernetes.io/pv-protection]
StorageClass:    local-disks
Status:          Available
Claim:
Reclaim Policy:  Retain
Access Modes:    RWO
Capacity:        200Mi
Node Affinity:   <none>
Message:
Source:
    Type:  LocalVolume (a persistent volume backed by local storage on a node)
    Path:  /mnt/disks/vol/vol1
Events:
  Type    Reason           Age   From                                                                 Message
  ----    ------           ----  ----                                                                 -------
  Normal  MarkPVSucceeded  1m    local-volume-monitor-127.0.0.1-40a8fb4d-4206-11e8-8e52-080027765304  Mark PV successfully with annotation key: NotMountPoint
  Normal  MarkPVSucceeded  22s   local-volume-monitor-127.0.0.1-40a8fb4d-4206-11e8-8e52-080027765304  Mark PV successfully with annotation key: HostPathNotExist
```

We can see that, monitor can detect the unhealthy situations.

### Delete ServiceAccount, ConfigMaps and kubernetes Daemonset
``` console
kubectl delete -f ./kubernetes/
```
