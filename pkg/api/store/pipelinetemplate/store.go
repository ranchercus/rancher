package pipelinetemplate

import (
	"github.com/rancher/norman/types"
	"github.com/rancher/norman/types/convert"
	client "github.com/rancher/types/client/management/v3"
	"regexp"
	"strings"
)

var reg = regexp.MustCompile(`\${\s*([0-9a-zA-Z_\-.@#]+)\s*}`)

type Store struct {
	types.Store
}

func NewStore(store types.Store) *Store {
	return &Store{
		Store:    store,
	}
}

func (s *Store) Create(apiContext *types.APIContext, schema *types.Schema, data map[string]interface{}) (map[string]interface{}, error) {
	data = s.extractParams(data)
	return s.Store.Create(apiContext, schema, data)
}

func (s *Store) Update(apiContext *types.APIContext, schema *types.Schema, data map[string]interface{}, id string) (map[string]interface{}, error) {
	data = s.extractParams(data)
	return s.Store.Update(apiContext, schema, data, apiContext.ID)
}

func (s *Store) extractParams(data map[string]interface{})  map[string]interface{}{
	template := convert.ToString(data[client.PipelineTemplateFieldTemplate])
	matchResult := reg.FindAllStringSubmatch(template, -1)

	paramMap := make(map[string]interface{})
	for _, v := range matchResult {
		if len(v) == 2 {
			param := strings.ToUpper(strings.TrimSpace(v[1]))
			paramMap[param] = nil
		}
	}

	data[client.PipelineTemplateFieldQuestions] = make([]interface{}, 0)

	for k, _ := range paramMap {
		data[client.PipelineTemplateFieldQuestions] = append(data[client.PipelineTemplateFieldQuestions].([]interface{}), k)
	}

	return data
}