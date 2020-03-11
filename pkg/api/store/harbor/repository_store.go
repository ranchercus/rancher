package harbor

import (
	"errors"
	"fmt"
	"github.com/rancher/norman/types"
	client "github.com/rancher/rancher/pkg/harbor"
	"github.com/rancher/rancher/pkg/ref"
	clusterschema "github.com/rancher/types/apis/cluster.cattle.io/v3/schema"
	v3 "github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/types/config"
)

const (
	REPOSITORY_TYPE  = "/v3/cluster/schemas/harborRepository"
	RepositoryAPIURL = "/api/repositories"
)

func WrapRepositoryStore(store types.Store, mgmt *config.ScaledContext, proxyClient *client.ProxyClient) types.Store {
	storeWrapped := &RepositoryStore{
		Store:  store,
		Token:  mgmt.Management.Tokens(""),
		client: proxyClient,
	}
	return storeWrapped
}

type RepositoryStore struct {
	types.Store
	Token  v3.TokenInterface
	client *client.ProxyClient
}

func (p *RepositoryStore) ByID(apiContext *types.APIContext, schema *types.Schema, id string) (map[string]interface{}, error) {
	return nil, nil
}

func (p *RepositoryStore) List(apiContext *types.APIContext, schema *types.Schema, opt *types.QueryOptions) ([]map[string]interface{}, error) {
	var result []map[string]interface{}
	projectId := apiContext.Query.Get("project_id")
	if len(projectId) == 0 {
		return result, nil
	}
	_, pid := ref.Parse(projectId)

	harborApiContext := &client.ApiContext{
		APIContext:  apiContext,
		ClusterName: apiContext.SubContext[clusterschema.Version.SubContextSchema],
	}
	err := p.client.Get(harborApiContext, fmt.Sprintf("%s?project_id=%s", RepositoryAPIURL, pid), &result)
	if err != nil {
		return nil, err
	}

	for _, r := range result {
		r["type"] = REPOSITORY_TYPE
	}
	return result, nil
}

func (p *RepositoryStore) Create(apiContext *types.APIContext, schema *types.Schema, data map[string]interface{}) (map[string]interface{}, error) {
	return nil, errors.New("unsupported")
}
func (p *RepositoryStore) Update(apiContext *types.APIContext, schema *types.Schema, data map[string]interface{}, id string) (map[string]interface{}, error) {
	return nil, errors.New("unsupported")
}
func (p *RepositoryStore) Delete(apiContext *types.APIContext, schema *types.Schema, id string) (map[string]interface{}, error) {
	return nil, errors.New("unsupported")
}
func (p *RepositoryStore) Watch(apiContext *types.APIContext, schema *types.Schema, opt *types.QueryOptions) (chan map[string]interface{}, error) {
	return nil, nil
}
