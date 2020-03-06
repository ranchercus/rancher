package settings

import (
	"github.com/rancher/types/apis/management.cattle.io/v3"
)

var clusterProvider ClusterSettingsProvider

type ClusterSettingsProvider interface {
	Get(name string) *v3.ClusterSetting
	GetAll() []*v3.ClusterSetting
}

func SetClusterProvider(p ClusterSettingsProvider) error {
	clusterProvider = p
	return nil
}

func GetClusterSetting(clusterName string) *v3.ClusterSetting {
	return clusterProvider.Get(clusterName)
}

func GetClusterSettings() []*v3.ClusterSetting {
	return clusterProvider.GetAll()
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

func GetRegistrySetting(clusterName string) *v3.RegistrySetting {
	setting := clusterProvider.Get(clusterName)
	if setting == nil {
		return &v3.RegistrySetting{}
	} else {
		ws := setting.Spec.RegistrySetting
		return &ws
	}
}