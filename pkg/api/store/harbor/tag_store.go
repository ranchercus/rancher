package harbor

import (
	"errors"
	"fmt"
	"github.com/rancher/norman/types"
	client "github.com/rancher/rancher/pkg/harbor"
	"github.com/rancher/rancher/pkg/settings"
	clusterschema "github.com/rancher/types/apis/cluster.cattle.io/v3/schema"
	v3 "github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/types/config"
)

const (
	TAG_TYPE  = "/v3/cluster/schemas/harborTag"
	TagAPIURL = "/api/repositories/%s/tags"
)

func WrapTagStore(store types.Store, mgmt *config.ScaledContext, proxyClient *client.ProxyClient) types.Store {
	storeWrapped := &TagStore{
		Store:  store,
		Token:  mgmt.Management.Tokens(""),
		client: proxyClient,
	}
	return storeWrapped
}

type TagStore struct {
	types.Store
	Token  v3.TokenInterface
	client *client.ProxyClient
}

func (p *TagStore) ByID(apiContext *types.APIContext, schema *types.Schema, id string) (map[string]interface{}, error) {
	return nil, nil
}

func (p *TagStore) List(apiContext *types.APIContext, schema *types.Schema, opt *types.QueryOptions) ([]map[string]interface{}, error) {
	var result []map[string]interface{}
	repo := apiContext.Query.Get("repo")

	harborApiContext := &client.ApiContext{
		APIContext:  apiContext,
		ClusterName: apiContext.SubContext[clusterschema.Version.SubContextSchema],
	}
	err := p.client.Get(harborApiContext, fmt.Sprintf(TagAPIURL, repo), &result)
	if err != nil {
		return nil, err
	}

	var registry string
	subCtx, _ := apiContext.SubContext[clusterschema.Version.SubContextSchema]
	rs := settings.GetRegistrySetting(subCtx)
	if rs != nil {
		registry = rs.Host
	}

	for _, r := range result {
		r["id"] = r["digest"]
		r["type"] = TAG_TYPE
		r["repo"] = repo
		r["full_name"] = fmt.Sprintf("%s/%s:%s", registry, repo, r["name"])
	}
	return result, nil
}

func (p *TagStore) Create(apiContext *types.APIContext, schema *types.Schema, data map[string]interface{}) (map[string]interface{}, error) {
	return nil, errors.New("unsupported")
}
func (p *TagStore) Update(apiContext *types.APIContext, schema *types.Schema, data map[string]interface{}, id string) (map[string]interface{}, error) {
	return nil, errors.New("unsupported")
}
func (p *TagStore) Delete(apiContext *types.APIContext, schema *types.Schema, id string) (map[string]interface{}, error) {
	return nil, errors.New("unsupported")
}
func (p *TagStore) Watch(apiContext *types.APIContext, schema *types.Schema, opt *types.QueryOptions) (chan map[string]interface{}, error) {
	return nil, nil
}
