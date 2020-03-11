//Author: Zac+
package harbor

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/rancher/norman/types"
	"github.com/rancher/rancher/pkg/auth/tokens"
	"github.com/rancher/rancher/pkg/ticker"
	v3 "github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/types/config"
	"github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sync"
	"time"
)

const tokenKeyIndex = "authn.management.cattle.io/token-key-index"

type ProxyClient struct {
	clusterSetting v3.ClusterSettingInterface
	token          v3.TokenInterface
	cachedClient   *sync.Map
	ctx            context.Context
}

type Error struct {
	Code    int
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("http error: code %d, message %s", e.Code, e.Message)
}

type ApiContext struct {
	*types.APIContext
	ClusterName string
}

type HttpClient struct {
	http.Client
	resourceVersion string
}

type Transport struct {
	transport http.RoundTripper
	host      string
	scheme    string
}

func NewTransport(transport http.RoundTripper, host, scheme string) *Transport {
	return &Transport{
		transport: transport,
		host:      host,
		scheme:    scheme,
	}
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Host = t.host
	req.URL.Scheme = t.scheme
	resp, err := t.transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	return resp, err
}

var proxyClient *ProxyClient
var locker = &sync.Mutex{}

func NewHarborProxy(ctx context.Context, mgmt *config.ScaledContext) *ProxyClient {
	locker.Lock()
	defer locker.Unlock()

	if proxyClient != nil {
		return proxyClient
	}

	proxyClient = &ProxyClient{
		clusterSetting: mgmt.Management.ClusterSettings(""),
		token:          mgmt.Management.Tokens(""),
		cachedClient:   &sync.Map{},
		ctx:            ctx,
	}
	err := proxyClient.sync()
	if err != nil {
		logrus.Error("sync harbor proxy client error", err)
	}
	go func() {
		for range ticker.Context(proxyClient.ctx, 60*time.Second) {
			err := proxyClient.sync()
			if err != nil {
				logrus.Error("sync harbor proxy client error", err)
			}
		}
	}()
	return proxyClient
}

func GetProxyClient() *ProxyClient {
	for proxyClient == nil {
		time.Sleep(1 * time.Second)
	}
	return proxyClient
}

func (c *ProxyClient) sync() error {
	list, err := c.clusterSetting.List(v1.ListOptions{})

	if err != nil {
		return err
	}
	c.cachedClient.Range(func(k interface{}, v interface{}) bool {
		exist := false
		for _, s := range list.Items {
			if s.Name == k.(string) && s.Spec.RegistrySetting.Host != "" {
				exist = true
				break
			}
		}
		if !exist {
			c.cachedClient.Delete(k)
		}
		return true
	})
	for _, s := range list.Items {
		httpClientInterface, ok := c.cachedClient.Load(s.Name)
		if ok && httpClientInterface.(*HttpClient).resourceVersion == s.ResourceVersion {
			continue
		}

		if registry := s.Spec.RegistrySetting; registry.Host != "" {
			httpClient := &HttpClient{
				resourceVersion: s.ResourceVersion,
			}
			tr := &http.Transport{}
			if !registry.Insecure {
				if registry.Cert == "" {
					tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
				}
				httpClient.Transport = NewTransport(tr, registry.Host, "https")
			} else {
				httpClient.Transport = NewTransport(tr, registry.Host, "http")
			}
			c.cachedClient.Store(s.Name, httpClient)
		}
	}
	return nil
}

func (c *ProxyClient) getClient(clusterName string) *HttpClient {
	if clusterName == "" {
		return nil
	}
	cachedClient, ok := c.cachedClient.Load(clusterName)
	if ok {
		if client, ok := cachedClient.(*HttpClient); ok {
			return client
		}
	}
	return nil
}

func (c *ProxyClient) Get(apiContext *ApiContext, url string, v ...interface{}) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	data, err := c.do(apiContext, req)
	if err != nil {
		return err
	}

	if len(v) == 0 {
		return nil
	}

	return json.Unmarshal(data, v[0])
}

func (c *ProxyClient) Post(apiContext *ApiContext, url string, v ...interface{}) error {
	var reader io.Reader
	if len(v) > 0 {
		data, err := json.Marshal(v[0])
		if err != nil {
			return err
		}

		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(http.MethodPost, url, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	_, err = c.do(apiContext, req)
	return err
}

func (c *ProxyClient) Put(apiContext *ApiContext, url string, v ...interface{}) error {
	var reader io.Reader
	if len(v) > 0 {
		data := []byte{}
		data, err := json.Marshal(v[0])
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(http.MethodPut, url, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	_, err = c.do(apiContext, req)
	return err
}

func (c *ProxyClient) Delete(apiContext *ApiContext, url string) error {
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	_, err = c.do(apiContext, req)
	return err
}

func (c *ProxyClient) do(apiContext *ApiContext, req *http.Request) ([]byte, error) {
	httpClient := c.getClient(apiContext.ClusterName)
	if httpClient == nil {
		return nil, &Error{
			Code:    404,
			Message: "Not found client",
		}
	}
	c.auth(apiContext, req)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &Error{
			Code:    resp.StatusCode,
			Message: string(data),
		}
	}

	return data, nil
}

func (c *ProxyClient) auth(apiContext *ApiContext, req *http.Request) {
	tokenAuthValue := tokens.GetTokenAuthFromRequest(apiContext.Request)
	if len(tokenAuthValue) == 0 {
		return
	}
	tokenName, tokenKey := tokens.SplitTokenParts(tokenAuthValue)
	if tokenName == "" || tokenKey == "" {
		return
	}
	lookupUsingClient := false
	objs, err := c.token.Controller().Informer().GetIndexer().ByIndex(tokenKeyIndex, tokenKey)
	if err != nil {
		if apierrors.IsNotFound(err) {
			lookupUsingClient = true
		} else {
			logrus.Error("Can't get token from index", err)
			return
		}
	} else if len(objs) == 0 {
		lookupUsingClient = true
	}
	storedToken := &v3.Token{}
	if lookupUsingClient {
		storedToken, err = c.token.Get(tokenName, metav1.GetOptions{})
		if err != nil {
			logrus.Error("Can't get token from store", err)
			return
		}
	} else {
		storedToken = objs[0].(*v3.Token)
	}

	req.SetBasicAuth(storedToken.UserPrincipal.LoginName, tokenAuthValue)
}
//Author: Zac-