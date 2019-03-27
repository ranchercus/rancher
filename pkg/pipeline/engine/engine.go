package engine

import (
	"github.com/rancher/rancher/pkg/pipeline/engine/jenkins"
	mv3 "github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/types/apis/project.cattle.io/v3"
	"github.com/rancher/types/config"
	"k8s.io/client-go/tools/cache"
)

type PipelineEngine interface {
	PreCheck(execution *v3.PipelineExecution) (bool, error)
	RunPipelineExecution(execution *v3.PipelineExecution) error
	RerunExecution(execution *v3.PipelineExecution) error
	StopExecution(execution *v3.PipelineExecution) error
	GetStepLog(execution *v3.PipelineExecution, stage int, step int) (string, error)
	SyncExecution(execution *v3.PipelineExecution) (bool, error)
}

func New(cluster *config.UserContext) PipelineEngine {
	serviceLister := cluster.Core.Services("").Controller().Lister()
	podLister := cluster.Core.Pods("").Controller().Lister()
	secrets := cluster.Core.Secrets("")
	secretLister := secrets.Controller().Lister()
	managementSecretLister := cluster.Management.Core.Secrets("").Controller().Lister()
	sourceCodeCredentialLister := cluster.Management.Project.SourceCodeCredentials("").Controller().Lister()
	pipelineLister := cluster.Management.Project.Pipelines("").Controller().Lister()
	dialer := cluster.Management.Dialer
	tokenInformer := cluster.Management.Management.Tokens("").Controller().Informer()
	tokenInformer.AddIndexers(map[string]cache.IndexFunc{"authn.management.cattle.io/uid-key-index": uidKeyIndexer})

	engine := &jenkins.Engine{
		ServiceLister:              serviceLister,
		PodLister:                  podLister,
		Secrets:                    secrets,
		SecretLister:               secretLister,
		ManagementSecretLister:     managementSecretLister,
		SourceCodeCredentialLister: sourceCodeCredentialLister,
		PipelineLister:             pipelineLister,

		Dialer:       dialer,
		ClusterName:  cluster.ClusterName,
		TokenIndexer: tokenInformer.GetIndexer(),
		UserLister:   cluster.Management.Management.Users("").Controller().Lister(),
	}
	return engine
}
func uidKeyIndexer(obj interface{}) ([]string, error) {
	token, ok := obj.(*mv3.Token)
	if !ok {
		return []string{}, nil
	}

	return []string{token.UserID}, nil
}
