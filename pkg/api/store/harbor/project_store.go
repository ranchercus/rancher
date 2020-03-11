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
	PROJECT_TYPE  = "/v3/cluster/schemas/harborProject"
	ProjectAPIURL = "/api/projects"
)

func WrapProjectStore(store types.Store, mgmt *config.ScaledContext, proxyClient *client.ProxyClient) types.Store {
	storeWrapped := &ProjectStore{
		Store:  store,
		Token:  mgmt.Management.Tokens(""),
		client: proxyClient,
	}
	return storeWrapped
}

type ProjectStore struct {
	types.Store
	Token  v3.TokenInterface
	client *client.ProxyClient
}

func (p *ProjectStore) ByID(apiContext *types.APIContext, schema *types.Schema, id string) (map[string]interface{}, error) {
	var result map[string]interface{}
	_, pid := ref.Parse(id)

	harborApiContext := &client.ApiContext{
		APIContext:  apiContext,
		ClusterName: apiContext.SubContext[clusterschema.Version.SubContextSchema],
	}
	err := p.client.Get(harborApiContext, fmt.Sprintf("%s/%s", ProjectAPIURL, pid), &result)
	if err != nil {
		return nil, err
	}

	result["harborMeta"] = result["metadata"]
	result["id"] = result["project_id"]
	delete(result, "metadata")
	result["type"] = PROJECT_TYPE
	return result, nil
}

func (p *ProjectStore) List(apiContext *types.APIContext, schema *types.Schema, opt *types.QueryOptions) ([]map[string]interface{}, error) {
	var result []map[string]interface{}

	harborApiContext := &client.ApiContext{
		APIContext:  apiContext,
		ClusterName: apiContext.SubContext[clusterschema.Version.SubContextSchema],
	}
	err := p.client.Get(harborApiContext, ProjectAPIURL, &result)

	if err != nil {
		return nil, err
	}
	for _, r := range result {
		r["harborMeta"] = r["metadata"]
		r["id"] = fmt.Sprintf("%v:%v", r["name"], r["project_id"])
		delete(r, "metadata")
		r["type"] = PROJECT_TYPE
	}
	return result, nil
}

func (p *ProjectStore) Create(apiContext *types.APIContext, schema *types.Schema, data map[string]interface{}) (map[string]interface{}, error) {
	return nil, errors.New("unsupported")
}
func (p *ProjectStore) Update(apiContext *types.APIContext, schema *types.Schema, data map[string]interface{}, id string) (map[string]interface{}, error) {
	return nil, errors.New("unsupported")
}
func (p *ProjectStore) Delete(apiContext *types.APIContext, schema *types.Schema, id string) (map[string]interface{}, error) {
	return nil, errors.New("unsupported")
}
func (p *ProjectStore) Watch(apiContext *types.APIContext, schema *types.Schema, opt *types.QueryOptions) (chan map[string]interface{}, error) {
	return nil, nil
}
