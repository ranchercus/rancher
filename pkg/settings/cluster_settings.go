package settings

import (
	"github.com/rancher/types/apis/management.cattle.io/v3"
)

var clusterProvider ClusterSettingsProvider

type ClusterSettingsProvider interface {
	Get(name string) *v3.ClusterSetting
}

func SetClusterProvider(p ClusterSettingsProvider) error {
	clusterProvider = p
	return nil
}

func GetPipelineSetting(clusterName string) *v3.PipelineSetting {
	setting := clusterProvider.Get(clusterName)
	if setting == nil {
		return &v3.PipelineSetting{}
	} else {
		ps := setting.Spec.PipelineSetting
		return &ps
	}
}

func GetWorkloadSetting(clusterName string) *v3.WorkloadSetting {
	setting := clusterProvider.Get(clusterName)
	if setting == nil {
		return &v3.WorkloadSetting{}
	} else {
		ws := setting.Spec.WorkloadSetting
		return &ws
	}
}