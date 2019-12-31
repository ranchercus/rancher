//Author: Zac+
package roletemplatebinding

import (
	"github.com/rancher/norman/store/transform"
	"github.com/rancher/norman/types"
	"github.com/rancher/rancher/pkg/harbor"
	"github.com/rancher/rancher/pkg/ref"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/types/client/management/v3"
	"github.com/rancher/types/config"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

type store struct {
	types.Store
	project v3.ProjectInterface
	user    v3.UserInterface
}

func SetStore(schema *types.Schema, mgmt *config.ScaledContext) {
	s := &store{
		Store:   schema.Store,
		project: mgmt.Management.Projects(""),
		user:    mgmt.Management.Users(""),
	}
	t := &transform.Store{
		Store: s,
		Transformer: func(apiContext *types.APIContext, schema *types.Schema, data map[string]interface{}, opt *types.QueryOptions) (map[string]interface{}, error) {
			return data, nil
		},
	}
	schema.Store = t
}

func (s *store) Create(apiContext *types.APIContext, schema *types.Schema, data map[string]interface{}) (map[string]interface{}, error) {
	created, err := s.Store.Create(apiContext, schema, data)

	if err == nil {
		go s.syncRegistry(apiContext, created, "create")
	}
	return created, err
}

func (s *store) Update(apiContext *types.APIContext, schema *types.Schema, data map[string]interface{}, id string) (map[string]interface{}, error) {
	updated, err := s.Store.Update(apiContext, schema, data, id)

	if err == nil {
		go s.syncRegistry(apiContext, updated, "update")
	}
	return updated, err
}

func (s *store) Delete(apiContext *types.APIContext, schema *types.Schema, id string) (map[string]interface{}, error) {
	deleted, err := s.Store.Delete(apiContext, schema, id)

	if err == nil {
		go s.syncRegistry(apiContext, deleted, "delete")
	}
	return deleted, err
}

func (s *store) syncRegistry(apiContext *types.APIContext, data map[string]interface{}, op string) {
	roleTemplateId, _ := data[client.ClusterRoleTemplateBindingFieldRoleTemplateID].(string)
	projectId, _ := data["projectId"].(string)
	userId, _ := data[client.ClusterRoleTemplateBindingFieldUserID].(string)
	userPrincipalId, _ := data[client.ClusterRoleTemplateBindingFieldUserPrincipalID].(string)
	uid := ""
	if userId != "" {
		uid = userId
	} else {
		_, preUid := ref.Parse(userPrincipalId)
		if strings.HasPrefix(preUid, "//") && len(preUid) > 2 {
			uid = preUid[2:]
		} else {
			uid = preUid
		}
	}

	ns, pid := ref.Parse(projectId)
	project, err := s.project.GetNamespaced(ns, pid, v1.GetOptions{})
	if err != nil {
		logrus.Warningf("sync %s registry, get project[%s] error: %v", op, projectId, err)
		return
	}
	user, err := s.user.Get(uid, v1.GetOptions{})
	if err != nil {
		logrus.Warningf("sync %s registry, get user[%s] error: %v", op, uid, err)
		return
	}
	switch op {
	case "create":
		harbor.SyncAddProjectMember(apiContext, project.Spec.DisplayName, user.Username, roleTemplateId, ns)
	case "update":
		harbor.SyncUpdateProjectMember(apiContext, project.Spec.DisplayName, user.Username, roleTemplateId, ns)
	case "delete":
		harbor.SyncDeleteProjectMember(apiContext, project.Spec.DisplayName, user.Username, ns)
	}
}

//Author: Zac-
