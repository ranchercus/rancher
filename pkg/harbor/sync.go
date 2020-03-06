package harbor

import (
	"errors"
	"fmt"
	"github.com/rancher/norman/types"
	"github.com/rancher/rancher/pkg/randomtoken"
	"github.com/rancher/rancher/pkg/settings"
	"github.com/sirupsen/logrus"
	"strconv"
	"strings"
)

type Role int

const (
	UserAPIURI           = "/api/users"
	ProjectAPIURL        = "/api/projects"
	ProjectMembersAPIURL = "/api/projects/%d/members"

	ADMIN_ROLE    Role = 1
	DEVELPER_ROLE Role = 2
	GUEST_ROLE    Role = 3
)

var tokenKeyIndex = "authn.management.cattle.io/token-key-index"

func SyncAddUser(apiContext *types.APIContext, username, displayName string) {
	if username == "" {
		return
	}
	token, err := randomtoken.Generate()
	if err != nil {
		token = "cYUgvESa0^spLLtQ"
	}
	if displayName == "" {
		displayName = username
	}

	user := map[string]string{
		"username": username,
		"email":    fmt.Sprintf("%s@rancher.placeholder", username),
		"realname": displayName,
		"password": strings.ToUpper(token[:1]) + token[1:16],
	}

	for _, setting := range settings.GetClusterSettings() {
		if setting == nil || setting.Spec.RegistrySetting.Host == "" {
			continue
		}
		harborApiContext := &ApiContext{
			APIContext:  apiContext,
			ClusterName: setting.Name,
		}

		err := GetProxyClient().Post(harborApiContext, UserAPIURI, user)
		if err != nil {
			logrus.Warningf("sync adding harbor user[%s] in [%s] error: %v", username, setting.Name, err)
		}
	}
}

func SyncRemoveUser(apiContext *types.APIContext, username string) {
	if username == "" {
		return
	}

	for _, setting := range settings.GetClusterSettings() {
		if setting == nil || setting.Spec.RegistrySetting.Host == "" {
			continue
		}
		harborApiContext := &ApiContext{
			APIContext:  apiContext,
			ClusterName: setting.Name,
		}
		var users []map[string]interface{}
		err := GetProxyClient().Get(harborApiContext, fmt.Sprintf("%s?username=%s", UserAPIURI, username), &users)
		if err != nil || len(users) != 1 {
			logrus.Warningf("sync removing harbor user[%s] in [%s] error: %v, can't find user", username, setting.Name, err)
			continue
		}
		userId := users[0]["user_id"]
		err = GetProxyClient().Delete(harborApiContext, fmt.Sprintf("%s/%v", UserAPIURI, userId))

		if err != nil {
			logrus.Warningf("sync removing harbor user[%s] in [%s] error: %v", username, setting.Name, err)
		}
	}
}

func SyncAddProject(apiContext *types.APIContext, projectName, clusterName string) {
	if projectName == "" {
		return
	}
	projectName = strings.ToLower(projectName)
	project := map[string]interface{}{
		"project_name": projectName,
		"metadata": map[string]string{
			"public": "false",
		},
	}
	harborApiContext := &ApiContext{
		APIContext:  apiContext,
		ClusterName: clusterName,
	}
	err := GetProxyClient().Post(harborApiContext, ProjectAPIURL, project)
	if err != nil {
		logrus.Warningf("sync adding harbor project[%s] in [%s] error: %v", projectName, clusterName, err)
	}
}

func SyncDeleteProject(apiContext *types.APIContext, projectName, clusterName string) {
	if projectName == "" {
		return
	}
	projectName = strings.ToLower(projectName)
	harborApiContext := &ApiContext{
		APIContext:  apiContext,
		ClusterName: clusterName,
	}

	id, err := getProjectIdByName(harborApiContext, projectName, clusterName)
	if err != nil {
		logrus.Warningf("sync removing harbor error: %v", err)
		return
	}
	err = GetProxyClient().Delete(harborApiContext, fmt.Sprintf("%s/%d", ProjectAPIURL, id))
	if err != nil {
		logrus.Warningf("sync removing harbor project[%s] in [%s] error: %v", projectName, clusterName, err)
	}
}

func SyncAddProjectMember(apiContext *types.APIContext, projectName, username, role, clusterName string) {
	if projectName == "" || username == "" {
		return
	}
	projectName = strings.ToLower(projectName)
	username = strings.ToLower(username)

	harborApiContext := &ApiContext{
		APIContext:  apiContext,
		ClusterName: clusterName,
	}

	projectId, err := getProjectIdByName(harborApiContext, projectName, clusterName)
	if err != nil {
		logrus.Warningf("sync adding harbor project member[%s] error: %v", username, err)
		return
	}

	data := map[string]interface{}{
		"role_id": convertRole(role),
		"member_user": map[string]string{
			"username": username,
		},
	}

	err = GetProxyClient().Post(harborApiContext, fmt.Sprintf(ProjectMembersAPIURL, projectId), data)
	if err != nil {
		logrus.Warningf("sync add harbor project[%s] member[%s] in [%s] error: %v", projectName, username, clusterName, err)
	}
}

func SyncUpdateProjectMember(apiContext *types.APIContext, projectName, username, role, clusterName string) {
	if projectName == "" || username == "" {
		return
	}
	projectName = strings.ToLower(projectName)
	username = strings.ToLower(username)

	harborApiContext := &ApiContext{
		APIContext:  apiContext,
		ClusterName: clusterName,
	}
	projectId, err := getProjectIdByName(harborApiContext, projectName, clusterName)
	if err != nil {
		logrus.Warningf("sync updating harbor project member[%s] error: %v", username, err)
		return
	}

	userId, err := getProjectMember(harborApiContext, projectId, username, clusterName)
	if err != nil {
		logrus.Warningf("sync updating harbor project member[%s] error: %v", username, err)
		return
	}

	data := map[string]Role{
		"role_id": convertRole(role),
	}

	err = GetProxyClient().Put(harborApiContext, fmt.Sprintf(ProjectMembersAPIURL+"/%d", projectId, userId), data)
	if err != nil {
		logrus.Warningf("sync updating harbor project[%s] member[%s] in [%s] error: %v", projectName, username, clusterName, err)
	}
}

func SyncDeleteProjectMember(apiContext *types.APIContext, projectName, username, clusterName string) {
	if projectName == "" || username == "" {
		return
	}
	projectName = strings.ToLower(projectName)
	username = strings.ToLower(username)

	harborApiContext := &ApiContext{
		APIContext:  apiContext,
		ClusterName: clusterName,
	}
	projectId, err := getProjectIdByName(harborApiContext, projectName, clusterName)
	if err != nil {
		logrus.Warningf("sync removing harbor project member[%s] error: %v", username, err)
		return
	}

	userId, err := getProjectMember(harborApiContext, projectId, username, clusterName)
	if err != nil {
		logrus.Warningf("sync removing harbor project member[%s] error: %v", username, err)
		return
	}

	err = GetProxyClient().Delete(harborApiContext, fmt.Sprintf(ProjectMembersAPIURL+"/%d", projectId, userId))
	if err != nil {
		logrus.Warningf("sync removing harbor project[%s] member[%s] in [%s] error: %v", projectName, username, clusterName, err)
	}
}

func convertRole(role string) Role {
	switch role {
	case "project-owner":
		return ADMIN_ROLE
	case "project-member":
		return DEVELPER_ROLE
	case "workloads-manage":
		return DEVELPER_ROLE
	default:
		return GUEST_ROLE
	}
}

func getProjectIdByName(apiContext *ApiContext, projectName, clusterName string) (int, error) {
	projectName = strings.ToLower(projectName)
	var projects []map[string]interface{}

	err := GetProxyClient().Get(apiContext, fmt.Sprintf("%s?name=%s", ProjectAPIURL, projectName), &projects)
	if err != nil || len(projects) != 1 {
		return 0, errors.New(fmt.Sprintf("can't get harbor project[%s] in [%s] error: %v", projectName, clusterName, err))
	}
	projectId := projects[0]["project_id"]

	id, err := strconv.Atoi(fmt.Sprintf("%v", projectId))
	if err != nil {
		return 0, errors.New(fmt.Sprintf("can't get harbor project[%s] in [%s] error: %v", projectName, clusterName, err))
	}
	return id, nil
}

func getProjectMember(apiContext *ApiContext, projectId int, username string, clusterName string) (int, error) {
	var members []map[string]interface{}

	err := GetProxyClient().Get(apiContext, fmt.Sprintf(ProjectMembersAPIURL+"?entityname=%s", projectId, username), &members)
	if err != nil || len(members) != 1 {
		return 0, errors.New(fmt.Sprintf("can't get harbor project_id[%d] member[%s] in [%s] error: %v", projectId, username, clusterName, err))
	}

	memberId := members[0]["id"]
	id, err := strconv.Atoi(fmt.Sprintf("%v", memberId))
	if err != nil {
		return 0, errors.New(fmt.Sprintf("can't get harbor project_id[%s] member[%s] in [%s] error: %v", projectId, username, clusterName, err))
	}
	return id, nil
}
