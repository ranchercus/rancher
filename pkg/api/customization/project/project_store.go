package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rancher/rancher/pkg/harbor"
	"github.com/rancher/rancher/pkg/ref"
	"k8s.io/client-go/tools/cache"
	"strings"

	"github.com/mitchellh/mapstructure"
	"github.com/rancher/norman/api/access"
	"github.com/rancher/norman/httperror"
	"github.com/rancher/norman/types"
	"github.com/rancher/norman/types/convert"
	"github.com/rancher/norman/types/values"
	"github.com/rancher/rancher/pkg/clustermanager"
	"github.com/rancher/rancher/pkg/resourcequota"
	v3 "github.com/rancher/types/apis/management.cattle.io/v3"
	mgmtschema "github.com/rancher/types/apis/management.cattle.io/v3/schema"
	mgmtclient "github.com/rancher/types/client/management/v3"
	"github.com/rancher/types/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"github.com/rancher/types/client/management/v3"
)

const roleTemplatesRequired = "authz.management.cattle.io/creator-role-bindings"
const quotaField = "resourceQuota"
const namespaceQuotaField = "namespaceDefaultResourceQuota"
//Auhtor: Zac+
const projectByNameIndex = "auth.management.cattle.io/project-by-name"
//Author: Zac-

type projectStore struct {
	types.Store
	projectLister      v3.ProjectLister
	roleTemplateLister v3.RoleTemplateLister
	scaledContext      *config.ScaledContext
	clusterLister      v3.ClusterLister
	//Auhtor: Zac+
	projectIndexer     cache.Indexer
	//Auhtor: Zac-
}

func SetProjectStore(schema *types.Schema, mgmt *config.ScaledContext) {
	//Auhtor: Zac+
	projectIndexer := map[string]cache.IndexFunc{
		projectByNameIndex: func(obj interface{}) ([]string, error) {
			p, ok := obj.(*v3.Project)
			if !ok {
				return []string{}, nil
			}

			return []string{p.Spec.DisplayName}, nil
		},
	}
	mgmt.Management.Projects("").Controller().Informer().AddIndexers(projectIndexer)
	//Auhtor: Zac-
	store := &projectStore{
		Store:              schema.Store,
		projectLister:      mgmt.Management.Projects("").Controller().Lister(),
		roleTemplateLister: mgmt.Management.RoleTemplates("").Controller().Lister(),
		scaledContext:      mgmt,
		clusterLister:      mgmt.Management.Clusters("").Controller().Lister(),
		//Auhtor: Zac+
		projectIndexer:     mgmt.Management.Projects("").Controller().Informer().GetIndexer(),
		//Auhtor: Zac-
	}
	schema.Store = store
}

func (s *projectStore) Create(apiContext *types.APIContext, schema *types.Schema, data map[string]interface{}) (map[string]interface{}, error) {
	//Author: Zac+
	err := s.validateDisplayName(data, "")
	if err != nil {
		return nil, err
	}
	//Author: Zac-
	annotation, err := s.createProjectAnnotation()
	if err != nil {
		return nil, err
	}

	if err := s.validateResourceQuota(apiContext, data, ""); err != nil {
		return nil, err
	}

	values.PutValue(data, annotation, "annotations", roleTemplatesRequired)

	//return s.Store.Create(apiContext, schema, data) Author: Zac+
	created, err := s.Store.Create(apiContext, schema, data)
	if err == nil {
		name, _ := created[client.ProjectFieldName].(string)
		clusterId, _ := created[client.ProjectFieldClusterID].(string)
		id, _ := created["id"].(string)
		go func() {
			harbor.SyncAddProject(apiContext, name, clusterId)
			_, n := ref.Parse(id)
			harbor.SyncAddUser(apiContext, n, name)
			harbor.SyncAddProjectMember(apiContext, name, n, "", clusterId)
		}()
	}
	return created, err
	//Author: Zac -
}

func (s *projectStore) Update(apiContext *types.APIContext, schema *types.Schema, data map[string]interface{}, id string) (map[string]interface{}, error) {
	//Author: Zac+
	err := s.validateDisplayName(data, apiContext.ID)
	if err != nil {
		return nil, err
	}
	//Author: Zac-
	if err := s.validateResourceQuota(apiContext, data, id); err != nil {
		return nil, err
	}

	return s.Store.Update(apiContext, schema, data, id)
}

func (s *projectStore) Delete(apiContext *types.APIContext, schema *types.Schema, id string) (map[string]interface{}, error) {
	parts := strings.Split(id, ":")

	proj, err := s.projectLister.Get(parts[0], parts[len(parts)-1])
	if err != nil {
		return nil, err
	}
	if proj.Labels["authz.management.cattle.io/system-project"] == "true" {
		return nil, httperror.NewAPIError(httperror.MethodNotAllowed, "System Project cannot be deleted")
	}
	//return s.Store.Delete(apiContext, schema, id) Author: Zac+
	deleted, err := s.Store.Delete(apiContext, schema, id)
	if err == nil {
		name, _ := deleted[client.ProjectFieldName].(string)
		clusterId, _ := deleted[client.ProjectFieldClusterID].(string)
		id, _ := deleted["id"].(string)
		go func() {
			harbor.SyncDeleteProject(apiContext, name, clusterId)
			_, n := ref.Parse(id)
			harbor.SyncRemoveUser(apiContext, n)
		}()
	}
	return deleted, err
	//Author: Zac-
}

func (s *projectStore) createProjectAnnotation() (string, error) {
	rt, err := s.roleTemplateLister.List("", labels.NewSelector())
	if err != nil {
		return "", err
	}

	annoMap := make(map[string][]string)

	for _, role := range rt {
		if role.ProjectCreatorDefault && !role.Locked {
			annoMap["required"] = append(annoMap["required"], role.Name)
		}
	}

	d, err := json.Marshal(annoMap)
	if err != nil {
		return "", err
	}

	return string(d), nil
}

func (s *projectStore) validateResourceQuota(apiContext *types.APIContext, data map[string]interface{}, id string) error {
	quotaO, quotaOk := data[quotaField]
	if quotaO == nil {
		quotaOk = false
	}
	nsQuotaO, namespaceQuotaOk := data[namespaceQuotaField]
	if nsQuotaO == nil {
		namespaceQuotaOk = false
	}
	if quotaOk != namespaceQuotaOk {
		if quotaOk {
			return httperror.NewFieldAPIError(httperror.MissingRequired, namespaceQuotaField, "")
		}
		return httperror.NewFieldAPIError(httperror.MissingRequired, quotaField, "")
	} else if !quotaOk {
		return nil
	}

	var nsQuota mgmtclient.NamespaceResourceQuota
	if err := convert.ToObj(nsQuotaO, &nsQuota); err != nil {
		return err
	}
	var projectQuota mgmtclient.ProjectResourceQuota
	if err := convert.ToObj(quotaO, &projectQuota); err != nil {
		return err
	}

	projectQuotaLimit, err := limitToLimit(projectQuota.Limit)
	if err != nil {
		return err
	}
	nsQuotaLimit, err := limitToLimit(nsQuota.Limit)
	if err != nil {
		return err
	}

	// limits in namespace default quota should include all limits defined in the project quota
	projectQuotaLimitMap, err := convert.EncodeToMap(projectQuotaLimit)
	if err != nil {
		return err
	}

	nsQuotaLimitMap, err := convert.EncodeToMap(nsQuotaLimit)
	if err != nil {
		return err
	}
	if len(nsQuotaLimitMap) != len(projectQuotaLimitMap) {
		return httperror.NewFieldAPIError(httperror.MissingRequired, namespaceQuotaField, fmt.Sprintf("does not have all fields defined on a %s", quotaField))
	}

	for k := range projectQuotaLimitMap {
		if _, ok := nsQuotaLimitMap[k]; !ok {
			return httperror.NewFieldAPIError(httperror.MissingRequired, namespaceQuotaField, fmt.Sprintf("misses %s defined on a %s", k, quotaField))
		}
	}
	return s.isQuotaFit(apiContext, nsQuotaLimit, projectQuotaLimit, id)
}

func (s *projectStore) isQuotaFit(apiContext *types.APIContext, nsQuotaLimit *v3.ResourceQuotaLimit,
	projectQuotaLimit *v3.ResourceQuotaLimit, id string) error {
	// check that namespace default quota is within project quota
	isFit, msg, err := resourcequota.IsQuotaFit(nsQuotaLimit, []*v3.ResourceQuotaLimit{}, projectQuotaLimit)
	if err != nil {
		return err
	}
	if !isFit {
		return httperror.NewFieldAPIError(httperror.MaxLimitExceeded, namespaceQuotaField, fmt.Sprintf("exceeds %s on fields: %s",
			quotaField, msg))
	}

	if id == "" {
		return nil
	}

	var project mgmtclient.Project
	if err := access.ByID(apiContext, &mgmtschema.Version, mgmtclient.ProjectType, id, &project); err != nil {
		return err
	}

	// check if fields were added or removed
	// and update project's namespaces accordingly
	defaultQuotaLimitMap, err := convert.EncodeToMap(nsQuotaLimit)
	if err != nil {
		return err
	}

	usedQuotaLimitMap := map[string]interface{}{}
	if project.ResourceQuota != nil && project.ResourceQuota.UsedLimit != nil {
		usedQuotaLimitMap, err = convert.EncodeToMap(project.ResourceQuota.UsedLimit)
		if err != nil {
			return err
		}
	}

	limitToAdd := map[string]interface{}{}
	limitToRemove := map[string]interface{}{}
	for key, value := range defaultQuotaLimitMap {
		if _, ok := usedQuotaLimitMap[key]; !ok {
			limitToAdd[key] = value
		}
	}

	for key, value := range usedQuotaLimitMap {
		if _, ok := defaultQuotaLimitMap[key]; !ok {
			limitToRemove[key] = value
		}
	}

	// check that used quota is not bigger than the project quota
	for key := range limitToRemove {
		delete(usedQuotaLimitMap, key)
	}

	var usedLimitToCheck mgmtclient.ResourceQuotaLimit
	err = convert.ToObj(usedQuotaLimitMap, &usedLimitToCheck)
	if err != nil {
		return err
	}

	usedQuotaLimit, err := limitToLimit(&usedLimitToCheck)
	if err != nil {
		return err
	}
	isFit, msg, err = resourcequota.IsQuotaFit(usedQuotaLimit, []*v3.ResourceQuotaLimit{}, projectQuotaLimit)
	if err != nil {
		return err
	}
	if !isFit {
		return httperror.NewFieldAPIError(httperror.MaxLimitExceeded, quotaField, fmt.Sprintf("is below the used limit on fields: %s",
			msg))
	}

	if len(limitToAdd) == 0 && len(limitToRemove) == 0 {
		return nil
	}

	// check if default quota is enough to set on namespaces
	toAppend := &mgmtclient.ResourceQuotaLimit{}
	if err := mapstructure.Decode(limitToAdd, toAppend); err != nil {
		return err
	}
	converted, err := limitToLimit(toAppend)
	if err != nil {
		return err
	}
	mu := resourcequota.GetProjectLock(id)
	mu.Lock()
	defer mu.Unlock()

	namespacesCount, err := s.getNamespacesCount(apiContext, project)
	if err != nil {
		return err
	}
	var nsLimits []*v3.ResourceQuotaLimit
	for i := 0; i < namespacesCount; i++ {
		nsLimits = append(nsLimits, converted)
	}

	isFit, msg, err = resourcequota.IsQuotaFit(&v3.ResourceQuotaLimit{}, nsLimits, projectQuotaLimit)
	if err != nil {
		return err
	}
	if !isFit {
		return httperror.NewFieldAPIError(httperror.MaxLimitExceeded, namespaceQuotaField,
			fmt.Sprintf("exceeds project limit on fields %s when applied to all namespaces in a project",
				msg))
	}

	return nil
}

func (s *projectStore) getNamespacesCount(apiContext *types.APIContext, project mgmtclient.Project) (int, error) {
	cluster, err := s.clusterLister.Get("", project.ClusterID)
	if err != nil {
		return 0, err
	}

	kubeConfig, err := clustermanager.ToRESTConfig(cluster, s.scaledContext)
	if kubeConfig == nil || err != nil {
		return 0, err
	}

	clusterContext, err := config.NewUserContext(s.scaledContext, *kubeConfig, cluster.Name)
	if err != nil {
		return 0, err
	}
	namespaces, err := clusterContext.Core.Namespaces("").List(metav1.ListOptions{})
	if err != nil {
		return 0, err
	}
	count := 0
	for _, n := range namespaces.Items {
		if n.Annotations == nil {
			continue
		}
		if n.Annotations["field.cattle.io/projectId"] == project.ID {
			count++
		}
	}

	return count, nil
}

func limitToLimit(from *mgmtclient.ResourceQuotaLimit) (*v3.ResourceQuotaLimit, error) {
	var to v3.ResourceQuotaLimit
	err := convert.ToObj(from, &to)
	return &to, err
}
//Author: Zac+
func (s *projectStore) validateDisplayName(data map[string]interface{}, id string) error {
	name, _ := data[client.ProjectFieldName].(string)
	clusterId, _ := data[client.ProjectFieldClusterID].(string)
	projects, err := s.projectIndexer.ByIndex(projectByNameIndex, name)
	if err == nil {
		exist := false
		for _, v := range projects {
			if project, ok := v.(*v3.Project); ok && project != nil && project.Spec.ClusterName == clusterId {
				if id != "" {
					_, n := ref.Parse(id)
					p, err := s.projectLister.Get(clusterId, n)
					if err == nil && p.Spec.DisplayName == name {
						break
					}
				}
				exist = true
				break
			}
		}
		if exist {
			return errors.New("dubplicate project name")
		}
	}
	return nil
}
//Author: Zac-