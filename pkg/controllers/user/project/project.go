package project

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"github.com/rancher/norman/types"
	"github.com/rancher/rancher/pkg/pipeline/utils"
	"github.com/rancher/rancher/pkg/randomtoken"
	"github.com/rancher/rancher/pkg/ref"
	"github.com/rancher/rancher/pkg/settings"
	"github.com/rancher/rancher/pkg/ticker"
	"github.com/rancher/types/apis/core/v1"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/types/config"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kubernetes/pkg/credentialprovider"
	"strings"
	"time"
)

const projectIDFieldLabel = "field.cattle.io/projectId"
const registryName = "system-default-registry"
const userByUsernameIndex = "auth.management.cattle.io/user-by-username"

type projectLifecycle struct {
	secrets       v1.SecretInterface
	secretLister  v1.SecretLister
	projectLister v3.ProjectLister
	users         v3.UserInterface
	userIndexer   cache.Indexer
	ctx           context.Context
}

func Register(ctx context.Context, cluster *config.UserContext) {
	clusterSecretsClient := cluster.Core.Secrets("")
	usersClient := cluster.Management.Management.Users("")
	userInformer := cluster.Management.Management.Users("").Controller().Informer()
	userIndexers := map[string]cache.IndexFunc{
		userByUsernameIndex: func(obj interface{}) ([]string, error) {
			u, ok := obj.(*v3.User)
			if !ok {
				return []string{}, nil
			}

			return []string{u.Username}, nil
		},
	}
	userInformer.AddIndexers(userIndexers)
	lifecycle := &projectLifecycle{
		secrets:       clusterSecretsClient,
		secretLister:  cluster.Core.Secrets("").Controller().Lister(),
		projectLister: cluster.Management.Management.Projects("").Controller().Lister(),
		users:         usersClient,
		userIndexer:   userInformer.GetIndexer(),
		ctx:           ctx,
	}

	cluster.Management.Management.Projects("").AddLifecycle(ctx, "project-lifecycle", lifecycle)

	sdrHandler := &systemDefaultRegistryHandler{
		users:       usersClient,
		userIndexer: userInformer.GetIndexer(),
		ctx:         ctx,
	}
	cluster.Management.Core.Secrets("").AddHandler(ctx, "system-default-registry-handler", sdrHandler.sync)
	go lifecycle.checkAndAdjust(1*time.Minute, cluster.ClusterName)
}

func (p *projectLifecycle) Create(obj *v3.Project) (runtime.Object, error) {
	if settings.SystemDefaultRegistry.Get() != "" {
		return nil, p.syncDefaultRegistryCredential(obj)
	}
	return nil, nil
}

func (p *projectLifecycle) Remove(obj *v3.Project) (runtime.Object, error) {
	err := p.secrets.DeleteNamespaced(obj.Name, fmt.Sprintf("%s-%s", registryName, obj.Name), &metav1.DeleteOptions{})
	logrus.Warningf("warning delete system default docker registry secret got - %v", err)
	users, err := p.userIndexer.ByIndex(userByUsernameIndex, obj.Name)
	if err != nil {
		return nil, err
	}
	for _, obj := range users {
		user := obj.(*v3.User)
		err := p.users.Delete(user.Name, &metav1.DeleteOptions{})
		logrus.Warningf("warning delete system default user got - %v", err)
	}
	return nil, nil
}

func (p *projectLifecycle) Updated(obj *v3.Project) (runtime.Object, error) {
	return nil, nil
}

func (p *projectLifecycle) syncDefaultRegistryCredential(obj *v3.Project) error {
	projectId := fmt.Sprintf("%s:%s", obj.Namespace, obj.Name)
	token, err := randomtoken.Generate()
	if err != nil {
		logrus.Warningf("warning generate random token got - %v, use default instead", err)
		token = utils.PipelineSecretDefaultToken
	}

	defaultDockerCredential, err := p.getDefaultRegistryCredential(projectId, token, settings.SystemDefaultRegistry.Get())
	if err != nil {
		return err
	}
	if _, err := p.secrets.Create(defaultDockerCredential); err != nil && !apierrors.IsAlreadyExists(err) {
		return errors.Wrapf(err, "Error create credential for default pipeline registry")
	}
	users, err := p.userIndexer.ByIndex(userByUsernameIndex, obj.Name)
	if err == nil && len(users) > 0 {
		user := users[0].(*v3.User)
		projectUser, err := p.getProjectUser(user.ObjectMeta.Name, obj.Name, token, obj.Spec.DisplayName)
		if err != nil {
			return err
		}
		if _, err := p.users.Update(projectUser); err != nil {
			return errors.Wrapf(err, "Error update user for default pipeline registry")
		}
		return nil
	} else {
		userId := types.GenerateName("user")
		projectUser, err := p.getProjectUser(userId, obj.Name, token, obj.Spec.DisplayName)
		if err != nil {
			return err
		}
		if _, err := p.users.Create(projectUser); err != nil && !apierrors.IsAlreadyExists(err) {
			return errors.Wrapf(err, "Error create user for default pipeline registry")
		}
	}
	return nil
}

func (p *projectLifecycle) getDefaultRegistryCredential(projectID string, token string, hostname string) (*corev1.Secret, error) {
	_, projectName := ref.Parse(projectID)
	config := credentialprovider.DockerConfigJson{
		Auths: credentialprovider.DockerConfig{
			hostname: credentialprovider.DockerConfigEntry{
				Username: projectName,
				Password: token,
				Email:    "",
			},
		},
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", registryName, projectName),
			Namespace: projectName,
			Annotations: map[string]string{
				projectIDFieldLabel: projectID,
			},
		},
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: configJSON,
		},
		Type: corev1.SecretTypeDockerConfigJson,
	}, nil
}

func (p *projectLifecycle) getProjectUser(userId, projectName, token, displayName string) (*v3.User, error) {
	enabled := true
	pwd, err := hashPasswordString(token)
	if err != nil {
		return nil, err
	}
	return &v3.User{
		ObjectMeta: metav1.ObjectMeta{
			Name:   userId,
			Labels: map[string]string{"cattle.io/creator": "norman"},
		},
		TypeMeta: metav1.TypeMeta{
			APIVersion: "management.cattle.io/v3",
			Kind:       "User",
		},
		DisplayName:        displayName,
		Description:        "user for system default docker registry",
		Username:           projectName,
		Password:           pwd,
		MustChangePassword: false,
		Enabled:            &enabled,
		Me:                 false,
	}, nil
}

func (p *projectLifecycle) checkAndAdjust(syncInterval time.Duration, clusterName string) {
	if settings.SystemDefaultRegistry.Get() == "" {
		return
	}
	for range ticker.Context(p.ctx, syncInterval) {
		projects, err := p.projectLister.List(clusterName, labels.NewSelector())
		if err != nil {
			errors.Wrapf(err, "Error get projects for default pipeline registry")
		}
		for _, project := range projects {
			secret, err := p.secretLister.Get(project.Name, fmt.Sprintf("%s-%s", registryName, project.Name))
			if err != nil && !apierrors.IsNotFound(err) {
				errors.Wrapf(err, "Error check for default pipeline registry")
				continue
			}
			if secret == nil {
				err = p.syncDefaultRegistryCredential(project)
				errors.Wrapf(err, "Error adjust for default pipeline registry")
			}
		}
	}
}

func hashPasswordString(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", errors.Wrap(err, "problem encrypting password")
	}
	return string(hash), nil
}

type systemDefaultRegistryHandler struct {
	users       v3.UserInterface
	userIndexer cache.Indexer
	ctx         context.Context
}

func (h *systemDefaultRegistryHandler) sync(key string, obj *corev1.Secret) (runtime.Object, error) {
	if obj != nil && obj.DeletionTimestamp != nil {
		secretName := ""
		splits := strings.Split(key, "/")
		if len(splits) == 2 {
			secretName = splits[1]
		}
		if strings.HasPrefix(secretName, registryName) {
			username := strings.Replace(secretName, fmt.Sprintf("%s-", registryName), "", 1)
			users, err := h.userIndexer.ByIndex(userByUsernameIndex, username)
			if err != nil {
				return nil, err
			}
			for _, obj := range users {
				user := obj.(*v3.User)
				err := h.users.Delete(user.Name, &metav1.DeleteOptions{})
				logrus.Warningf("warning delete system default user got - %v", err)
			}
		}
	}
	return obj, nil
}
