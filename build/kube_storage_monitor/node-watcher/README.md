# Node watcher

Below is how to deploy node watcher to watch node failure conditions

## Deployment

### Create a kubernetes cluster containing two nodes(192.168.0.101, 192.168.0.105)
``` kubectl get nodes
NAME            STATUS     ROLES     AGE       VERSION
192.168.0.101   Ready      <none>    8h        v1.10.0-alpha.1.5216+9b7439d77d0b29
192.168.0.105   NotReady   <none>    30m       v1.10.0-alpha.1.1689+77d6ca8d4bb0b1-dirty
```

### Create ServiceAccount and Node watcher Deployment.
Notice that, here provisioner configmap is the same as local pv provisioner.
``` console
kubectl create -f kubernetes/admin-account.yaml
kubectl create -f kubernetes/node-watcher.yaml
```

### Describe the kubernetes Deployment to see if it succeeds
``` kubectl get deployment
NAME           DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
node-watcher   1         1         1            1           6s
```
``` kubectl describe deployment node-watcher
Name:                   node-watcher
Namespace:              default
CreationTimestamp:      Wed, 02 May 2018 18:39:28 +0800
Labels:                 app=node-watcher
Annotations:            deployment.kubernetes.io/revision=1
Selector:               app=node-watcher
Replicas:               1 desired | 1 updated | 1 total | 1 available | 0 unavailable
StrategyType:           RollingUpdate
MinReadySeconds:        0
RollingUpdateStrategy:  1 max unavailable, 1 max surge
Pod Template:
  Labels:           app=node-watcher
  Service Account:  node-watcher-admin
  Containers:
   watcher:
    Image:      quay.io/kube_storage_monitor/storage-monitor:latest
    Port:       <none>
    Host Port:  <none>
    Args:
      --enable-node-watcher=true
    Environment:  <none>
    Mounts:       <none>
  Volumes:        <none>
Conditions:
  Type           Status  Reason
  ----           ------  ------
  Available      True    MinimumReplicasAvailable
  Progressing    True    NewReplicaSetAvailable
OldReplicaSets:  <none>
NewReplicaSet:   node-watcher-76774dfbd8 (1/1 replicas created)
Events:
  Type    Reason             Age   From                   Message
  ----    ------             ----  ----                   -------
  Normal  ScalingReplicaSet  42s   deployment-controller  Scaled up replica set node-watcher-76774dfbd8 to 1
```
If error occurs, we can use `kubectl get pods` and `kubectl logs $podID` to see the error log and debug

### Create a local PV which will be scheduled to 192.168.0.105
Local PV yaml file is:
```
apiVersion: v1
kind: PersistentVolume
metadata:
  name: example-local-pv-1
spec:
  capacity:
    storage: 200Mi
  accessModes:
  - ReadWriteOnce
  persistentVolumeReclaimPolicy: Retain
  storageClassName: local-storage
  local:
    path: /mnt/disks/vol/vol1
  nodeAffinity:
    required:
      nodeSelectorTerms:
      - matchExpressions:
        - key: kubernetes.io/hostname
          operator: In
          values:
          - 192.168.0.105
```
Create the local PV:
```
NAME                 CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS      CLAIM     STORAGECLASS    REASON    AGE
example-local-pv-1   200Mi      RWO            Retain           Available             local-storage             3s
```
```
Name:              example-local-pv-1
Labels:            <none>
Annotations:       <none>
Finalizers:        [kubernetes.io/pv-protection]
StorageClass:      local-storage
Status:            Available
Claim:
Reclaim Policy:    Retain
Access Modes:      RWO
Capacity:          200Mi
Node Affinity:
  Required Terms:
    Term 0:        kubernetes.io/hostname in [192.168.0.105]
Message:
Source:
    Type:  LocalVolume (a persistent volume backed by local storage on a node)
    Path:  /mnt/disks/vol/vol1
Events:    <none>
```

### Make node 192.168.0.105 NotReady to see if the local PV is marked
``` kubectl get nodes
NAME            STATUS     ROLES     AGE       VERSION
192.168.0.101   Ready      <none>    8h        v1.10.0-alpha.1.5216+9b7439d77d0b29
192.168.0.105   NotReady   <none>    36m       v1.10.0-alpha.1.1689+77d6ca8d4bb0b1-dirty
```
``` kubectl describe pv example-local-pv-1
Name:              example-local-pv-1
Labels:            <none>
Annotations:       FirstMarkTime=2018-05-02 10:45:29.627889192 +0000 UTC m=+360.587864632
                   NodeFailure=true
Finalizers:        [kubernetes.io/pv-protection]
StorageClass:      local-storage
Status:            Available
Claim:
Reclaim Policy:    Retain
Access Modes:      RWO
Capacity:          200Mi
Node Affinity:
  Required Terms:
    Term 0:        kubernetes.io/hostname in [192.168.0.105]
Message:
Source:
    Type:  LocalVolume (a persistent volume backed by local storage on a node)
    Path:  /mnt/disks/vol/vol1
Events:
  Type    Reason           Age   From          Message
  ----    ------           ----  ----          -------
  Normal  MarkPVSucceeded  2m    node-watcher  Mark PV successfully with annotation key: NodeFailure
```

We can see that, local PV is marked.

### Delete the ServiceAccount and kubernete Deployment
``` console
kubectl delete -f ./kubernetes/
```

