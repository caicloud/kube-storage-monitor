package local_pv_monitor

import (
	"fmt"
	"io/ioutil"
	"path"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/golang/glog"
)

const (
	// MonitorConfigPath points to the path inside of the monitor container where configMap volume is mounted
	MonitorConfigPath = "/etc/monitor/config/"
)

type MonitorConfiguration struct {
	// LabelSelectorForPV is the label selector for monitor to filter PVs
	// +optional
	LabelSelectorForPV string `json:"labelSelectorForPV" yaml:"labelSelectorForPV"`
}

// LoadMonitorConfigs loads all configuration into a string and unmarshal it into MonitorConfiguration struct.
// The configuration is stored in the configmap which is mounted as a volume.
func LoadMonitorConfigs(configPath string, monitorConfig *MonitorConfiguration) error {
	files, err := ioutil.ReadDir(configPath)
	if err != nil {
		return err
	}
	data := make(map[string]string)
	for _, file := range files {
		if !file.IsDir() {
			if strings.Compare(file.Name(), "..data") != 0 {
				fileContents, err := ioutil.ReadFile(path.Join(configPath, file.Name()))
				if err != nil {
					glog.Infof("Could not read file: %s due to: %v", path.Join(configPath, file.Name()), err)
					return err
				}
				data[file.Name()] = string(fileContents)
			}
		}
	}
	return ConfigMapDataToMonitorConfig(data, monitorConfig)
}

// ConfigMapDataToMonitorConfig converts configmap data to monitor config.
func ConfigMapDataToMonitorConfig(data map[string]string, monitorConfig *MonitorConfiguration) error {
	rawYaml := ""
	for key, val := range data {
		rawYaml += key
		rawYaml += ": \n"
		rawYaml += insertSpaces(string(val))
	}

	if err := yaml.Unmarshal([]byte(rawYaml), monitorConfig); err != nil {
		return fmt.Errorf("fail to Unmarshal yaml due to: %#v", err)
	}

	return nil
}

func insertSpaces(original string) string {
	spaced := ""
	for _, line := range strings.Split(original, "\n") {
		spaced += "   "
		spaced += line
		spaced += "\n"
	}
	return spaced
}
