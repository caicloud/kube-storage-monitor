/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package local_pv

import (
	"os"

	"github.com/golang/glog"
	lvmonitor "github.com/caicloud/kube-storage-monitor/pkg/local_pv_monitor"
	"github.com/kubernetes-incubator/external-storage/local-volume/provisioner/pkg/common"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

var monitorConfig lvmonitor.MonitorConfiguration
var provisionerConfig common.ProvisionerConfiguration

func initConfig() {
	provisionerConfig = common.ProvisionerConfiguration{
		StorageClassConfig: make(map[string]common.MountConfig),
	}
	monitorConfig = lvmonitor.MonitorConfiguration{}

	if err := common.LoadProvisionerConfigs(common.ProvisionerConfigPath, &provisionerConfig); err != nil {
		glog.Fatalf("Error parsing Provisioner's configuration: %#v. Exiting...\n", err)
	}

	if err := lvmonitor.LoadMonitorConfigs(lvmonitor.MonitorConfigPath, &monitorConfig); err != nil {
		glog.Fatalf("Error parsing Monitor's configuration: %#v. Exiting...\n", err)
	}

	glog.Infof("Configuration parsing has been completed, ready to run...")
}

func RunLocalPVMonitor() {
	initConfig()

	nodeName := os.Getenv("MY_NODE_NAME")
	if nodeName == "" {
		glog.Fatalf("MY_NODE_NAME environment variable not set\n")
	}

	client := common.SetupClient()
	node := getNode(client, nodeName)

	glog.Info("Starting local PVs monitor \n")
	lvmonitor.NewLocalPVMonitor(client, &common.UserConfig{
		Node:            node,
		DiscoveryMap:    provisionerConfig.StorageClassConfig,
		NodeLabelsForPV: provisionerConfig.NodeLabelsForPV,
		UseAlphaAPI: provisionerConfig.UseAlphaAPI,
	}, &monitorConfig).Run(wait.NeverStop)
}

func getNode(client *kubernetes.Clientset, name string) *v1.Node {
	node, err := client.CoreV1().Nodes().Get(name, metav1.GetOptions{})
	if err != nil {
		glog.Fatalf("Could not get node information: %v", err)
	}
	return node
}


