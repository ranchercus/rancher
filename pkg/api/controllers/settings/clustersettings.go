package settings

import (
	"github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/sirupsen/logrus"
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