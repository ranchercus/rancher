package harbor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rancher/norman/httperror"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
)

const (
	UserAPIURI           = "/api/users"
	ProjectAPIURL        = "/api/projects"
	ProjectMembersAPIURL = "/api/projects/%d/members"
)

type Auth struct {
	User  string
	Token string
}

type Client struct {
	Host        string
	ClusterName string
	HTTPClient  *http.Client
}

func New(host string, httpClient *http.Client) *Client {
	if host[len(host)-1:] == "/" {
		host = host[:len(host)-1]
	}
	c := &Client{
		Host:       host,
		HTTPClient: httpClient,
	}
	return c
}

func (c *Client) getUser(auth Auth, username string) (*User, error) {
	userListURL, err := url.Parse(c.Host + UserAPIURI + "?username=" + username)
	if err != nil {
		return nil, err
	}
	req, _ := http.NewRequest(http.MethodGet, userListURL.String(), nil)
	req.SetBasicAuth(auth.User, auth.Token)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	err = checkHTTPError(resp, "delete harbor user")
	if err != nil {
		return nil, err
	}
	respContent, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var users []User
	err = json.Unmarshal(respContent, &users)
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, nil
	}
	user := users[0]
	return &user, nil
}

func (c *Client) createUser(auth Auth, content []byte) error {
	userURL, err := url.Parse(c.Host + UserAPIURI)
	if err != nil {
		return err
	}

	req, _ := http.NewRequest(http.MethodPost, userURL.String(), bytes.NewReader(content))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(auth.User, auth.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return checkHTTPError(resp, "create harbor user")
}

func (c *Client) deleteUser(auth Auth, username string) error {
	user, err := c.getUser(auth, username)
	if err != nil {
		return err
	}
	if user == nil {
		return nil
	}
	userURL, err := url.Parse(fmt.Sprintf("%s%s/%d", c.Host, UserAPIURI, user.UserId))
	if err != nil {
		return err
	}

	req, _ := http.NewRequest(http.MethodDelete, userURL.String(), nil)
	req.SetBasicAuth(auth.User, auth.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return checkHTTPError(resp, "delete harbor user")
}

func (c *Client) createProject(auth Auth, content []byte) error {
	projectURL, err := url.Parse(c.Host + ProjectAPIURL)
	if err != nil {
		return err
	}
	req, _ := http.NewRequest(http.MethodPost, projectURL.String(), bytes.NewReader(content))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(auth.User, auth.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return checkHTTPError(resp, "create harbor project")
}

func (c *Client) deleteProject(auth Auth, projectName string) error {
	project, err := c.getProject(auth, projectName)
	if err != nil {
		return err
	}
	if project == nil {
		return nil
	}
	projectURL, err := url.Parse(fmt.Sprintf("%s%s/%d", c.Host, ProjectAPIURL, project.ProjectId))
	if err != nil {
		return err
	}

	req, _ := http.NewRequest(http.MethodDelete, projectURL.String(), nil)
	req.SetBasicAuth(auth.User, auth.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return checkHTTPError(resp, "delete harbor project")
}

func (c *Client) getProject(auth Auth, name string) (*Project, error) {
	projectListURL, err := url.Parse(c.Host + ProjectAPIURL + "?name=" + name)
	if err != nil {
		return nil, err
	}
	req, _ := http.NewRequest(http.MethodGet, projectListURL.String(), nil)
	req.SetBasicAuth(auth.User, auth.Token)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	err = checkHTTPError(resp, "get harbor projects")
	if err != nil {
		return nil, err
	}
	respContent, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var projects []Project
	err = json.Unmarshal(respContent, &projects)
	if err != nil {
		return nil, err
	}
	if len(projects) == 0 {
		return nil, nil
	}
	project := projects[0]
	return &project, nil
}

func (c *Client) getProjectMember(auth Auth, name, username string) (*ProjectMember, error) {
	project, err := c.getProject(auth, name)
	if err != nil {
		return nil, err
	}
	projectURL, err := url.Parse(fmt.Sprintf(c.Host+ProjectMembersAPIURL+"?entityname=%s", project.ProjectId, username))
	if err != nil {
		return nil, err
	}
	req, _ := http.NewRequest(http.MethodGet, projectURL.String(), nil)
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(auth.User, auth.Token)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	respContent, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var members []ProjectMember
	err = json.Unmarshal(respContent, &members)
	if err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return nil, nil
	}
	member := members[0]
	return &member, nil
}

func (c *Client) addProjectMember(auth Auth, projectName string, username string, role Role) error {
	project, err := c.getProject(auth, projectName)
	if err != nil {
		return err
	}
	if project == nil {
		return errors.New(fmt.Sprintf("Can't find harbor project, name=%s", projectName))
	}
	data := map[string]interface{}{
		"role_id": role,
		"member_user": map[string]string{
			"username": username,
		},
	}
	projectURL, err := url.Parse(fmt.Sprintf(c.Host+ProjectMembersAPIURL, project.ProjectId))
	if err != nil {
		return err
	}
	content, err := json.Marshal(data)
	if err != nil {
		return err
	}

	req, _ := http.NewRequest(http.MethodPost, projectURL.String(), bytes.NewReader(content))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(auth.User, auth.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return checkHTTPError(resp, "add harbor project member")
}

func (c *Client) updateProjectMember(auth Auth, projectName string, username string, role *Role, op projectMemberOp) error {
	project, err := c.getProject(auth, projectName)
	if err != nil {
		return err
	}
	if project == nil {
		return errors.New(fmt.Sprintf("Can't find harbor project, name=%s", projectName))
	}
	user, err := c.getProjectMember(auth, projectName, username)
	if err != nil {
		return err
	}
	var method string
	var body io.Reader
	if op == project_member_op_update {
		if user == nil {
			return errors.New(fmt.Sprintf("Can't find harbor user, name=%s", username))
		}
		method = http.MethodPut
		data := map[string]Role{
			"role_id": *role,
		}
		content, err := json.Marshal(data)
		if err != nil {
			return err
		}
		body = bytes.NewReader(content)
	} else if op == project_member_op_delete {
		if user == nil {
			return nil
		}
		method = http.MethodDelete
		body = nil
	}

	projectURL, err := url.Parse(fmt.Sprintf(c.Host+ProjectMembersAPIURL+"/%d", project.ProjectId, user.Id))
	if err != nil {
		return err
	}

	req, _ := http.NewRequest(method, projectURL.String(), body)
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(auth.User, auth.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return checkHTTPError(resp, "update/delete harbor project member role")
}

func (c *Client) modifyProjectMemberRole(auth Auth, projectName string, username string, role Role) error {
	return c.updateProjectMember(auth, projectName, username, &role, project_member_op_update)
}

func (c *Client) deleteProjectMember(auth Auth, projectName string, username string) error {
	return c.updateProjectMember(auth, projectName, username, nil, project_member_op_delete)
}

func checkHTTPError(resp *http.Response, event string) error {
	if resp == nil {
		return nil
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		data, _ := ioutil.ReadAll(resp.Body)
		return httperror.NewAPIErrorLong(resp.StatusCode, fmt.Sprintf("%s got %d", event, resp.StatusCode), string(data))
	}
	return nil
}
