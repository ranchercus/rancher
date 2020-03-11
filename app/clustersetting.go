//Author: Zac+
package app

import (
	v3 "github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/types/config"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func addClusterSetting(management *config.ManagementContext) error {
	clusters, err := management.Management.Clusters("").List(v1.ListOptions{})
	if err != nil {
		return err
	}

	for _, c := range clusters.Items {
		_, err = management.Management.ClusterSettings("").ObjectClient().Create(&v3.ClusterSetting{
			ObjectMeta: v1.ObjectMeta{
				Name: c.Name,
			},
			Spec: v3.ClusterSettingSpec{},
		})

		if err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
	}
	return nil
}

//Author: Zac-
