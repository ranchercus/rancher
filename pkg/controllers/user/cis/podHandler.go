package cis

import (
	"fmt"

	v3 "github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type podHandler struct {
	mgmtCtxClusterScanClient v3.ClusterScanInterface
	clusterClient            v3.ClusterInterface
	clusterLister            v3.ClusterLister
	clusterNamespace         string
}

func (ph *podHandler) Sync(key string, pod *corev1.Pod) (runtime.Object, error) {
	if pod == nil || pod.DeletionTimestamp != nil || pod.Name != DefaultSonobuoyPodName {
		return nil, nil
	}
	// Check the annotation to see if it's done processing
	if _, ok := pod.Annotations[SonobuoyCompletionAnnotation]; !ok {
		return nil, nil
	}

	owner, ok := pod.Annotations[CisHelmChartOwner]
	if !ok {
		return nil, fmt.Errorf("sonobuoy done, but couldn't find owner annotation")
	}

	cs, err := ph.mgmtCtxClusterScanClient.Get(owner, v1.GetOptions{})
	if err != nil {
		if !kerrors.IsNotFound(err) {
			return nil, fmt.Errorf("error fetching cluster scan object: %v", owner)
		}
		return nil, nil
	}

	if v3.ClusterScanConditionCompleted.IsUnknown(cs) &&
		!v3.ClusterScanConditionCompleted.IsFalse(cs) {
		v3.ClusterScanConditionCompleted.False(cs)
		_, err = ph.mgmtCtxClusterScanClient.Update(cs)
		if err != nil {
			return nil, fmt.Errorf("error updating condition of cluster scan object: %v", owner)
		}
		logrus.Infof("Marking CIS scan complete: %v", owner)
	}
	return nil, nil
}
