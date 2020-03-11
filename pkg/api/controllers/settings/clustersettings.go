package settings

import (
	"github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/labels"
)


type clusterSettingsProvider struct {
	settingsLister v3.ClusterSettingLister
}

func (s *clusterSettingsProvider) Get(name string) *v3.ClusterSetting {
	setting, err := s.settingsLister.Get("", name)
	if err != nil {
		logrus.Errorf("Getting cluster setting for %s error, error=[%v]", name, err)
		return nil
	}
	return setting
}

func (s *clusterSettingsProvider) GetAll() []*v3.ClusterSetting {
	settings, err := s.settingsLister.List("", labels.NewSelector())
	if err != nil {
		logrus.Errorf("Getting cluster settings error, error=[%v]", err)
		return nil
	}
	return settings
}