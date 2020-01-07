package harbor

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/rancher/norman/types"
	"github.com/rancher/rancher/pkg/auth/tokens"
	"github.com/rancher/rancher/pkg/randomtoken"
	"github.com/rancher/rancher/pkg/settings"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/types/config"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"strings"
	"sync"
)

type registry struct {
	URL         string  `json:"url"`
	ClusterName string  `json:"clusterName"`
	Client      *Client `json:"-"`
}

var tokenKeyIndex = "authn.management.cattle.io/token-key-index"
var cachedRegistryClients = make(map[string]*Client)
var lock = &sync.Mutex{}
var mgmt *config.ScaledContext

func Setup(mt *config.ScaledContext) {
	mgmt = mt
}

func prepare() ([]*registry, error) {
	syncRegistries := settings.SyncRegistries.Get()
	var registries []*registry

	err := json.Unmarshal([]byte(syncRegistries), &registries)
	if err != nil {
		return nil, err
	}

	lock.Lock()
	defer lock.Unlock()
	for _, v := range registries {
		if _, ok := cachedRegistryClients[v.URL]; !ok {
			httpClient := &http.Client{}
			if strings.HasPrefix(v.URL, "https://") {
				tr := &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				}
				httpClient.Transport = tr
			}
			client := New(v.URL, httpClient)
			cachedRegistryClients[v.URL] = client
		}
		v.Client = cachedRegistryClients[v.URL]
	}
	return registries, nil
}

func SyncAddUser(apiContext *types.APIContext, username, displayName string) {
	clients, err := prepare()
	if err != nil {
		logrus.Warningf("prepare registries error: %v", err)
		return
	}
	if len(clients) == 0 {
		return
	}
	token, err := randomtoken.Generate()
	if err != nil {
		token = "cYUgvESa0^spLLtQ"
	}
	if displayName == "" {
		displayName = username
	}
	username = strings.ToLower(username)
	user := &User{
		Username: username,
		Email:    fmt.Sprintf("%s@rancher.placeholder", username),
		RealName: displayName,
		Password: strings.ToUpper(token[:1]) + token[1:16],
	}
	content, err := json.Marshal(user)
	if err != nil {
		logrus.Warningf("json marshal error: %v", err)
		return
	}
	for _, v := range clients {
		go func() {
			err = v.Client.createUser(getAuth(apiContext), content)
			if err != nil {
				logrus.Warningf("sync add harbor user[%s] in [%s] error: %v", user.Username, v.URL, err)
			}
		}()
	}
}

func SyncRemoveUser(apiContext *types.APIContext, username string) {
	clients, err := prepare()
	if err != nil {
		logrus.Warningf("prepare registries error: %v", err)
		return
	}
	if len(clients) == 0 {
		return
	}
	username = strings.ToLower(username)
	for _, v := range clients {
		go func() {
			err = v.Client.deleteUser(getAuth(apiContext), username)
			if err != nil {
				logrus.Warningf("sync remove harbor user[%s] in [%s] error: %v", username, v.URL, err)
			}
		}()
	}
}

func SyncAddProject(apiContext *types.APIContext, projectName, clusterName string) {
	projectName = strings.ToLower(projectName)
	project := &Project{
		ProjectName: projectName,
		Metadata: Metadata{
			Public: "false",
		},
	}
	content, err := json.Marshal(project)
	if err != nil {
		logrus.Warningf("json marshal error: %v", err)
		return
	}
	filterCluster(clusterName, func(client *Client) {
		err := client.createProject(getAuth(apiContext), content)
		if err != nil {
			logrus.Warningf("sync add harbor project[%s] in [%s] error: %v", projectName, client.Host, err)
		}
	})
}

func SyncDeleteProject(apiContext *types.APIContext, projectName, clusterName string) {
	projectName = strings.ToLower(projectName)
	filterCluster(clusterName, func(client *Client) {
		err := client.deleteProject(getAuth(apiContext), projectName)
		if err != nil {
			logrus.Warningf("sync delete harbor project[%s] in [%s] error: %v", projectName, client.Host, err)
		}
	})
}

func SyncAddProjectMember(apiContext *types.APIContext, projectName, username, role, clusterName string) {
	projectName = strings.ToLower(projectName)
	username = strings.ToLower(username)
	filterCluster(clusterName, func(client *Client) {
		err := client.addProjectMember(getAuth(apiContext), strings.ToLower(projectName), username, convertRole(role))
		if err != nil {
			logrus.Warningf("sync add harbor project[%s] member[%s] in [%s] error: %v", projectName, username, client.Host, err)
		}
	})
}

func SyncUpdateProjectMember(apiContext *types.APIContext, projectName, username, role, clusterName string) {
	projectName = strings.ToLower(projectName)
	username = strings.ToLower(username)
	filterCluster(clusterName, func(client *Client) {
		err := client.modifyProjectMemberRole(getAuth(apiContext), strings.ToLower(projectName), username, convertRole(role))
		if err != nil {
			logrus.Warningf("sync update harbor project[%s] member[%s] in [%s] error: %v", projectName, username, client.Host, err)
		}
	})
}

func SyncDeleteProjectMember(apiContext *types.APIContext, projectName, username, clusterName string) {
	projectName = strings.ToLower(projectName)
	username = strings.ToLower(username)
	filterCluster(clusterName, func(client *Client) {
		err := client.deleteProjectMember(getAuth(apiContext), strings.ToLower(projectName), username)
		if err != nil {
			logrus.Warningf("sync delete harbor project[%s] member[%s] in [%s] error: %v", projectName, username, client.Host, err)
		}
	})
}

func filterCluster(clusterName string, callback func(*Client)) {
	clients, err := prepare()
	if err != nil {
		logrus.Warningf("prepare registries error: %v", err)
		return
	}
	if len(clients) == 0 {
		return
	}
	for _, v := range clients {
		if v.ClusterName != clusterName {
			continue
		}
		go callback(v.Client)
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

func getAuth(apiContext *types.APIContext) Auth {
	auth := &Auth{}
	if mgmt == nil {
		return *auth
	}
	tokenAuthValue := tokens.GetTokenAuthFromRequest(apiContext.Request)
	if tokenAuthValue == "" {
		return *auth
	}

	tokenName, tokenKey := tokens.SplitTokenParts(tokenAuthValue)
	if tokenName == "" || tokenKey == "" {
		return *auth
	}

	lookupUsingClient := false
	objs, err := mgmt.Management.Tokens("").Controller().Informer().GetIndexer().ByIndex(tokenKeyIndex, tokenKey)
	if err != nil {
		if apierrors.IsNotFound(err) {
			lookupUsingClient = true
		} else {
			return *auth
		}
	} else if len(objs) == 0 {
		lookupUsingClient = true
	}
	storedToken := &v3.Token{}
	if lookupUsingClient {
		storedToken, err = mgmt.Management.Tokens("").Get(tokenName, metav1.GetOptions{})
		if err != nil {
			return *auth
		}
	} else {
		storedToken = objs[0].(*v3.Token)
	}

	auth.Token = tokenAuthValue
	auth.User = storedToken.UserPrincipal.LoginName
	return *auth
}
