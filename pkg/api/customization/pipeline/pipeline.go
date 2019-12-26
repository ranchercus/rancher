package pipeline

import (
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"github.com/rancher/norman/api/access"
	"github.com/rancher/norman/httperror"
	"github.com/rancher/norman/types"
	"github.com/rancher/rancher/pkg/pipeline/providers"
	"github.com/rancher/rancher/pkg/pipeline/remote"
	"github.com/rancher/rancher/pkg/pipeline/remote/model"
	"github.com/rancher/rancher/pkg/pipeline/utils"
	"github.com/rancher/rancher/pkg/ref"
	"github.com/rancher/types/apis/project.cattle.io/v3"
	"github.com/rancher/types/client/project/v3"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"net/http"
	"strings"
)

const (
	actionRun        = "run"
	actionPushConfig = "pushconfig"
	// Author: Zac +
	actionSub = "sub"
	// Author: Zac -
	linkConfigs  = "configs"
	linkYaml     = "yaml"
	linkBranches = "branches"
	queryBranch  = "branch"
	queryConfigs = "configs"
)

type Handler struct {
	PipelineLister             v3.PipelineLister
	PipelineExecutions         v3.PipelineExecutionInterface
	SourceCodeCredentialLister v3.SourceCodeCredentialLister
	SourceCodeCredentials      v3.SourceCodeCredentialInterface
	//Author: Zac +
	PipelineInterface v3.PipelineInterface
	//Author: Zac -
}

func Formatter(apiContext *types.APIContext, resource *types.RawResource) {
	//Author: Zac +
	revision := map[string]interface{}{
		"id": resource.ID,
	}
	if err := apiContext.AccessControl.CanDo(v3.GroupName, v3.PipelineExecutionResource.Name, "update", apiContext, revision, apiContext.Schema); err == nil {
		resource.AddAction(apiContext, actionRun)
	}
	if err := apiContext.AccessControl.CanDo(v3.GroupName, v3.PipelineResource.Name, "update", apiContext, revision, apiContext.Schema); err == nil {
		resource.AddAction(apiContext, actionPushConfig)
		resource.AddAction(apiContext, actionSub)
	}
	//Author: Zac -
	resource.Links[linkConfigs] = apiContext.URLBuilder.Link(linkConfigs, resource)
	resource.Links[linkYaml] = apiContext.URLBuilder.Link(linkYaml, resource)
	resource.Links[linkBranches] = apiContext.URLBuilder.Link(linkBranches, resource)
}

func (h *Handler) LinkHandler(apiContext *types.APIContext, next types.RequestHandler) error {
	if apiContext.Link == linkYaml {
		if apiContext.Method == http.MethodPut {
			return h.updatePipelineConfigYaml(apiContext)
		}
		return h.getPipelineConfigYAML(apiContext)
	} else if apiContext.Link == linkConfigs {
		return h.getPipelineConfigJSON(apiContext)
	} else if apiContext.Link == linkBranches {
		return h.getBranches(apiContext)
	}

	return httperror.NewAPIError(httperror.NotFound, "Link not found")
}

func (h *Handler) ActionHandler(actionName string, action *types.Action, apiContext *types.APIContext) error {
	switch actionName {
	case actionRun:
		return h.run(apiContext)
	case actionPushConfig:
		return h.pushConfig(apiContext)
	// Author: Zac +
	case actionSub:
		return h.subPipeline(apiContext)
		// Author: Zac -
	}
	return httperror.NewAPIError(httperror.InvalidAction, "unsupported action")
}

func (h *Handler) run(apiContext *types.APIContext) error {
	ns, name := ref.Parse(apiContext.ID)
	pipeline, err := h.PipelineLister.Get(ns, name)
	if err != nil {
		return err
	}
	runPipelineInput := v3.RunPipelineInput{}
	requestBytes, err := ioutil.ReadAll(apiContext.Request.Body)
	if err != nil {
		return err
	}
	if string(requestBytes) != "" {
		if err := json.Unmarshal(requestBytes, &runPipelineInput); err != nil {
			return err
		}
	}

	branch := runPipelineInput.Branch
	if branch == "" {
		return httperror.NewAPIError(httperror.InvalidBodyContent, "Error branch is not specified for the pipeline to run")
	}

	userName := apiContext.Request.Header.Get("Impersonate-User")
	pipelineConfig, err := providers.GetPipelineConfigByBranch(h.SourceCodeCredentials, h.SourceCodeCredentialLister, pipeline, branch)
	if err != nil {
		return err
	}

	if pipelineConfig == nil {
		return fmt.Errorf("find no pipeline config to run in the branch")
	}

	info, err := h.getBuildInfoByBranch(pipeline, branch)
	if err != nil {
		return err
	}
	info.TriggerType = utils.TriggerTypeUser
	info.TriggerUserName = userName
	// Author: Zac +
	info.RunCallbackScript = runPipelineInput.RunCallbackScript
	info.RunCodeScanner = runPipelineInput.RunCodeScanner
	// Author: Zac -
	execution, err := utils.GenerateExecution(h.PipelineExecutions, pipeline, pipelineConfig, info)
	if err != nil {
		return err
	}

	if execution == nil {
		return errors.New("condition is not match, no build is triggered")
	}

	data := map[string]interface{}{}
	if err := access.ByID(apiContext, apiContext.Version, client.PipelineExecutionType, ref.Ref(execution), &data); err != nil {
		return err
	}

	apiContext.WriteResponse(http.StatusOK, data)
	return err
}

func (h *Handler) pushConfig(apiContext *types.APIContext) error {
	ns, name := ref.Parse(apiContext.ID)
	pipeline, err := h.PipelineLister.Get(ns, name)
	if err != nil {
		return err
	}

	pushConfigInput := v3.PushPipelineConfigInput{}
	requestBytes, err := ioutil.ReadAll(apiContext.Request.Body)
	if err != nil {
		return err
	}
	if string(requestBytes) != "" {
		if err := json.Unmarshal(requestBytes, &pushConfigInput); err != nil {
			return err
		}
	}

	//use current user's auth to do the push
	userName := apiContext.Request.Header.Get("Impersonate-User")
	creds, err := h.SourceCodeCredentialLister.List(userName, labels.Everything())
	if err != nil {
		return err
	}
	sourceCodeType := model.GithubType
	var credential *v3.SourceCodeCredential
	for _, cred := range creds {
		if cred.Spec.ProjectName == pipeline.Spec.ProjectName && !cred.Status.Logout {
			sourceCodeType = cred.Spec.SourceCodeType
			credential = cred
			break
		}
	}

	_, projID := ref.Parse(pipeline.Spec.ProjectName)
	scpConfig, err := providers.GetSourceCodeProviderConfig(sourceCodeType, projID)
	if err != nil {
		return err
	}
	remote, err := remote.New(scpConfig)
	if err != nil {
		return err
	}
	accessToken, err := utils.EnsureAccessToken(h.SourceCodeCredentials, remote, credential)
	if err != nil {
		return err
	}

	for branch, config := range pushConfigInput.Configs {
		content, err := utils.PipelineConfigToYaml(&config)
		if err != nil {
			return err
		}
		//Author: Zac+
		if err := remote.SetPipelineFileInRepo(pipeline.Spec.RepositoryURL, branch, accessToken, content, pipeline.Spec.SubPath); err != nil {
			//Author: Zac-
			if apierr, ok := err.(*httperror.APIError); ok && apierr.Code.Status == http.StatusNotFound {
				//github returns 404 for unauth request to prevent leakage of private repos
				return httperror.NewAPIError(httperror.Unauthorized, "current git account is unauthorized for the action")
			}
			return err
		}
	}

	data := map[string]interface{}{}
	if err := access.ByID(apiContext, apiContext.Version, apiContext.Type, apiContext.ID, &data); err != nil {
		return err
	}

	apiContext.WriteResponse(http.StatusOK, data)
	return nil
}

func (h *Handler) getBuildInfoByBranch(pipeline *v3.Pipeline, branch string) (*model.BuildInfo, error) {
	credentialName := pipeline.Spec.SourceCodeCredentialName
	repoURL := pipeline.Spec.RepositoryURL
	var scpConfig interface{}
	var credential *v3.SourceCodeCredential
	var err error
	if credentialName != "" {
		ns, name := ref.Parse(credentialName)
		credential, err = h.SourceCodeCredentialLister.Get(ns, name)
		if err != nil {
			return nil, err
		}
		sourceCodeType := credential.Spec.SourceCodeType
		_, projID := ref.Parse(pipeline.Spec.ProjectName)
		scpConfig, err = providers.GetSourceCodeProviderConfig(sourceCodeType, projID)
		if err != nil {
			return nil, err
		}
	}
	remote, err := remote.New(scpConfig)
	if err != nil {
		return nil, err
	}
	accessToken, err := utils.EnsureAccessToken(h.SourceCodeCredentials, remote, credential)
	if err != nil {
		return nil, err
	}
	info, err := remote.GetHeadInfo(repoURL, branch, accessToken)
	if err != nil {
		return nil, err
	}
	return info, nil

}

func (h *Handler) getBranches(apiContext *types.APIContext) error {
	ns, name := ref.Parse(apiContext.ID)
	pipeline, err := h.PipelineLister.Get(ns, name)
	if err != nil {
		return err
	}

	var scpConfig interface{}
	var cred *v3.SourceCodeCredential
	if pipeline.Spec.SourceCodeCredentialName != "" {
		ns, name = ref.Parse(pipeline.Spec.SourceCodeCredentialName)
		cred, err = h.SourceCodeCredentialLister.Get(ns, name)
		if err != nil {
			return err
		}
		sourceCodeType := cred.Spec.SourceCodeType
		_, projID := ref.Parse(pipeline.Spec.ProjectName)
		scpConfig, err = providers.GetSourceCodeProviderConfig(sourceCodeType, projID)
		if err != nil {
			return err
		}
	}
	remote, err := remote.New(scpConfig)
	if err != nil {
		return err
	}
	accessKey, err := utils.EnsureAccessToken(h.SourceCodeCredentials, remote, cred)
	if err != nil {
		return err
	}

	branches, err := remote.GetBranches(pipeline.Spec.RepositoryURL, accessKey)
	if err != nil {
		return err
	}
	bytes, err := json.Marshal(branches)
	if err != nil {
		return err
	}
	apiContext.Response.Write(bytes)
	return nil
}

func (h *Handler) getPipelineConfigJSON(apiContext *types.APIContext) error {
	ns, name := ref.Parse(apiContext.ID)
	pipeline, err := h.PipelineLister.Get(ns, name)
	if err != nil {
		return err
	}
	branch := apiContext.Request.URL.Query().Get(queryBranch)

	m, err := h.getPipelineConfigs(pipeline, branch)
	if err != nil {
		return err
	}
	bytes, err := json.Marshal(m)
	if err != nil {
		return err
	}
	apiContext.Response.Write(bytes)
	return nil
}

func (h *Handler) getPipelineConfigYAML(apiContext *types.APIContext) error {
	yamlMap := map[string]interface{}{}
	m := map[string]*v3.PipelineConfig{}

	branch := apiContext.Request.URL.Query().Get(queryBranch)
	configs := apiContext.Request.URL.Query().Get(queryConfigs)
	if configs != "" {
		err := json.Unmarshal([]byte(configs), &m)
		if err != nil {
			return err
		}
		for b, config := range m {
			if config == nil {
				yamlMap[b] = nil
				continue
			}
			content, err := utils.PipelineConfigToYaml(config)
			if err != nil {
				return err
			}
			yamlMap[b] = string(content)
		}
	} else {
		ns, name := ref.Parse(apiContext.ID)
		pipeline, err := h.PipelineLister.Get(ns, name)
		if err != nil {
			return err
		}
		m, err = h.getPipelineConfigs(pipeline, branch)
		if err != nil {
			return err
		}
	}

	if branch != "" {
		config := m[branch]
		if config == nil {
			return nil
		}
		content, err := utils.PipelineConfigToYaml(config)
		if err != nil {
			return err
		}
		apiContext.Response.Write(content)
		return nil
	}

	for b, config := range m {
		if config == nil {
			yamlMap[b] = nil
			continue
		}
		content, err := utils.PipelineConfigToYaml(config)
		if err != nil {
			return err
		}
		yamlMap[b] = string(content)
	}

	bytes, err := json.Marshal(yamlMap)
	if err != nil {
		return err
	}
	apiContext.Response.Write(bytes)
	return nil
}
func (h *Handler) updatePipelineConfigYaml(apiContext *types.APIContext) error {
	branch := apiContext.Request.URL.Query().Get(queryBranch)
	if branch == "" {
		return httperror.NewAPIError(httperror.InvalidOption, "Branch is not specified")
	}

	ns, name := ref.Parse(apiContext.ID)
	pipeline, err := h.PipelineLister.Get(ns, name)
	if err != nil {
		return err
	}

	content, err := ioutil.ReadAll(apiContext.Request.Body)
	if err != nil {
		return err
	}
	//check yaml
	config := &v3.PipelineConfig{}
	if err := yaml.Unmarshal(content, config); err != nil {
		return err
	}

	//use current user's auth to do the push
	userName := apiContext.Request.Header.Get("Impersonate-User")
	creds, err := h.SourceCodeCredentialLister.List(userName, labels.Everything())
	if err != nil {
		return err
	}
	sourceCodeType := model.GithubType
	var credential *v3.SourceCodeCredential
	for _, cred := range creds {
		if cred.Spec.ProjectName == pipeline.Spec.ProjectName && !cred.Status.Logout {
			sourceCodeType = cred.Spec.SourceCodeType
			credential = cred
		}
	}

	_, projID := ref.Parse(pipeline.Spec.ProjectName)
	scpConfig, err := providers.GetSourceCodeProviderConfig(sourceCodeType, projID)
	if err != nil {
		return err
	}
	remote, err := remote.New(scpConfig)
	if err != nil {
		return err
	}
	accessToken, err := utils.EnsureAccessToken(h.SourceCodeCredentials, remote, credential)
	if err != nil {
		return err
	}

	//Author: Zac+
	if err := remote.SetPipelineFileInRepo(pipeline.Spec.RepositoryURL, branch, accessToken, content, pipeline.Spec.SubPath); err != nil {
		//Author: Zac-
		if apierr, ok := err.(*httperror.APIError); ok && apierr.Code.Status == http.StatusNotFound {
			//github returns 404 for unauth request to prevent leakage of private repos
			return httperror.NewAPIError(httperror.Unauthorized, "current git account is unauthorized for the action")
		}
		return err
	}

	return nil
}

func (h *Handler) getPipelineConfigs(pipeline *v3.Pipeline, branch string) (map[string]*v3.PipelineConfig, error) {
	var scpConfig interface{}
	var cred *v3.SourceCodeCredential
	var err error
	if pipeline.Spec.SourceCodeCredentialName != "" {
		ns, name := ref.Parse(pipeline.Spec.SourceCodeCredentialName)
		cred, err = h.SourceCodeCredentialLister.Get(ns, name)
		if err != nil {
			return nil, err
		}
		sourceCodeType := cred.Spec.SourceCodeType
		_, projID := ref.Parse(pipeline.Spec.ProjectName)
		scpConfig, err = providers.GetSourceCodeProviderConfig(sourceCodeType, projID)
		if err != nil {
			return nil, err
		}
	}

	remote, err := remote.New(scpConfig)
	if err != nil {
		return nil, err
	}
	accessToken, err := utils.EnsureAccessToken(h.SourceCodeCredentials, remote, cred)
	if err != nil {
		return nil, err
	}

	m := map[string]*v3.PipelineConfig{}

	if branch != "" {
		//Author: Zac+
		content, err := remote.GetPipelineFileInRepo(pipeline.Spec.RepositoryURL, branch, accessToken, pipeline.Spec.SubPath)
		//Author: Zac-
		if err != nil {
			return nil, err
		}
		if content != nil {
			spec, err := utils.PipelineConfigFromYaml(content)
			if err != nil {
				return nil, errors.Wrapf(err, "Error fetching pipeline config in Branch '%s'", branch)
			}
			m[branch] = spec
		} else {
			m[branch] = nil
		}

	} else {
		branches, err := remote.GetBranches(pipeline.Spec.RepositoryURL, accessToken)
		if err != nil {
			return nil, err
		}
		for _, b := range branches {
			//Author: Zac+
			content, err := remote.GetPipelineFileInRepo(pipeline.Spec.RepositoryURL, b, accessToken, pipeline.Spec.SubPath)
			//Author: Zac-
			if err != nil {
				return nil, err
			}
			if content != nil {
				spec, err := utils.PipelineConfigFromYaml(content)
				if err != nil {
					return nil, errors.Wrapf(err, "Error fetching pipeline config in Branch '%s'", b)
				}
				m[b] = spec
			} else {
				m[b] = nil
			}
		}
	}

	return m, nil
}

// Author: Zac +
func (h *Handler) subPipeline(apiContext *types.APIContext) error {
	ns, name := ref.Parse(apiContext.ID)
	pipeline, err := h.PipelineLister.Get(ns, name)
	if err != nil {
		return err
	}
	subPipelineInput := v3.SubPipelineInput{}
	requestBytes, err := ioutil.ReadAll(apiContext.Request.Body)
	if err != nil {
		return err
	}
	if string(requestBytes) != "" {
		if err := json.Unmarshal(requestBytes, &subPipelineInput); err != nil {
			return err
		}
	}
	userName := apiContext.Request.Header.Get("Impersonate-User")
	subPath := strings.TrimSpace(subPipelineInput.SubPath)
	if len(subPath) > 0 {
		if subPath[:1] == "/" {
			subPath = subPath[1:]
		}
	} else {
		apiContext.Response.Write([]byte(pipeline.Name))
		return nil
	}
	contextPath := strings.TrimSpace(subPipelineInput.ContextPath)
	if len(contextPath) > 0 && contextPath[:1] == "/" {
		contextPath = contextPath[1:]
	}

	requirement1, err := labels.NewRequirement("cattle.io/parent-pipeline", selection.Equals, []string{pipeline.Name})
	if err != nil {
		return err
	}
	requirement2, err := labels.NewRequirement("cattle.io/pipeline-sub-path", selection.Equals, []string{subPath})
	if err != nil {
		return err
	}
	selector := labels.NewSelector()
	selector = selector.Add(*requirement1, *requirement2)
	pipelines, err := h.PipelineLister.List(ns, selector)
	if err != nil {
		return err
	}
	if len(pipelines) != 0 {
		apiContext.Response.Write([]byte(pipelines[0].Name))
		return nil
	}

	subPipeline := &v3.Pipeline{}
	pipelineId := types.GenerateName("pipeline")
	subPipeline.Name = pipelineId
	subPipeline.Namespace = pipeline.Namespace
	subPipeline.Labels = map[string]string{
		"cattle.io/creator":           "norman",
		"cattle.io/parent-pipeline":   pipeline.Name,
		"cattle.io/pipeline-sub-path": subPath,
	}
	subPipeline.Annotations = map[string]string{
		"field.cattle.io/creatorId":                            userName,
		"lifecycle.cattle.io/create.pipeline-controller_local": "true",
	}
	subPipeline.ClusterName = pipeline.ClusterName
	subPipeline.Spec.ProjectName = pipeline.Spec.ProjectName
	subPipeline.Spec.DisplayName = pipeline.Spec.DisplayName
	subPipeline.Spec.TriggerWebhookPush = false
	subPipeline.Spec.TriggerWebhookPr = false
	subPipeline.Spec.TriggerWebhookTag = false
	subPipeline.Spec.RepositoryURL = pipeline.Spec.RepositoryURL
	subPipeline.Spec.SourceCodeCredentialName = pipeline.Spec.SourceCodeCredentialName
	subPipeline.Spec.SubPath = subPath
	subPipeline.Spec.ContextPath = contextPath
	_, err = h.PipelineInterface.Create(subPipeline)
	if err != nil {
		return err
	}
	apiContext.Response.Write([]byte(pipelineId))
	return err
}

// Author: Zac -
