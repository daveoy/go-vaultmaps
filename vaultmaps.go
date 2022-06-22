// set VAULT_ADDR and VAULT_TOKEN at runtime to minimize  VCS issues here
package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	serializer "k8s.io/apimachinery/pkg/runtime/serializer/json"
)

type AuthRequestBody struct {
	Token string `json:"token"`
}

type AuthResponse struct {
	LeaseID       string      `json:"lease_id"`
	Renewable     bool        `json:"renewable"`
	LeaseDuration int         `json:"lease_duration"`
	Data          interface{} `json:"data"`
	Warnings      interface{} `json:"warnings"`
	Auth          struct {
		ClientToken string   `json:"client_token"`
		Accessor    string   `json:"accessor"`
		Policies    []string `json:"policies"`
		Metadata    struct {
			Username string `json:"username"`
			Org      string `json:"org"`
		} `json:"metadata"`
	} `json:"auth"`
}

type SecretResponse struct {
	RequestID     string `json:"request_id"`
	LeaseID       string `json:"lease_id"`
	Renewable     bool   `json:"renewable"`
	LeaseDuration int    `json:"lease_duration"`
	Data          struct {
		Data struct {
			HelmSecretValues string `json:"helmSecretValues"`
		} `json:"data"`
		Metadata struct {
			CreatedTime    time.Time   `json:"created_time"`
			CustomMetadata interface{} `json:"custom_metadata"`
			DeletionTime   string      `json:"deletion_time"`
			Destroyed      bool        `json:"destroyed"`
			Version        int         `json:"version"`
		} `json:"metadata"`
	} `json:"data"`
	WrapInfo interface{} `json:"wrap_info"`
	Warnings interface{} `json:"warnings"`
	Auth     interface{} `json:"auth"`
}

func LookupEnvOrString(key string, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return defaultVal
}

func getConfig(fs *flag.FlagSet) []string {
	cfg := make([]string, 0, 10)
	fs.VisitAll(func(f *flag.Flag) {
		cfg = append(cfg, fmt.Sprintf("%s:%q", f.Name, f.Value.String()))
	})

	return cfg
}

func main() {
	// get cli args
	var vaultAddress = flag.String("vault-address", LookupEnvOrString("VAULT_ADDRESS", "https://vault.ps.thmulti.com:8200"), "vault address")
	var githubToken = flag.String("github-token", LookupEnvOrString("GITHUB_TOKEN", "fake-token"), "your github token")
	var secretPath = flag.String("secret-path", LookupEnvOrString("SECRET_PATH", "fake-path"), "secret path")
	var outputPath = flag.String("output-path", LookupEnvOrString("OUTPUT_PATH", "."), "path to output file, default is .")
	flag.Parse()
	log.Printf("app.config %v\n", getConfig(flag.CommandLine))
	// get a vault token
	// set up client
	httpclient := &http.Client{
		Timeout: 0,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	// format our data
	authRequestBody := &AuthRequestBody{
		Token: *githubToken,
	}
	// encode in json
	jsonBody, jsonError := json.Marshal(authRequestBody)
	if jsonError != nil {
		panic(jsonError)
	}
	// set up reqeust
	tokenReq, _ := http.NewRequest(http.MethodPut, *vaultAddress+"/v1/auth/github/login", bytes.NewBuffer(jsonBody))
	tokenReq.Header.Set("x-vault-request", "true")
	// send request for token
	tokenResp, tokenRespError := httpclient.Do(tokenReq)
	if tokenRespError != nil {
		panic(tokenRespError)
	}
	defer tokenResp.Body.Close()
	tokenRespBody, tokenRespBodyError := ioutil.ReadAll(tokenResp.Body) // response body is []byte
	if tokenRespBodyError != nil {
		panic(tokenRespBodyError)
	}
	// set up struct to unpack into
	var tokenResponseJSON AuthResponse
	// unpack
	json.Unmarshal(tokenRespBody, &tokenResponseJSON)
	// extract token
	vaultToken := tokenResponseJSON.Auth.ClientToken
	// set up request for secrets
	secretReq, _ := http.NewRequest(http.MethodGet, *vaultAddress+"/v1/"+*secretPath, bytes.NewBuffer(jsonBody))
	secretReq.Header.Set("x-vault-token", vaultToken)
	secretReq.Header.Set("x-vault-request", "true")
	//	log.Printf("logging into %s with token %s to retreive key at %s", *vaultAddress, vaultToken, *secretPath)
	secretResp, secretRespError := httpclient.Do(secretReq)
	if secretRespError != nil {
		panic(secretRespError)
	}
	defer secretResp.Body.Close()
	secretRespBody, secretRespBodyError := ioutil.ReadAll(secretResp.Body) // response body is []byte
	if secretRespBodyError != nil {
		panic(secretRespBodyError)
	}
	// set up struct to unpack into
	var secretResponseJSON SecretResponse
	// unpack
	json.Unmarshal(secretRespBody, &secretResponseJSON)
	// extract secret to yaml
	myYaml := make(map[string]string)
	for _, val := range strings.Split(secretResponseJSON.Data.Data.HelmSecretValues, ",") {
		values := strings.Split(val, "=")
		myYaml[values[0]] = values[1]
	}
	s := strings.Split(*secretPath, "/")
	var serviceName string
	for _, pathPart := range s {
		if strings.Contains(pathPart, "service") {
			serviceName = strings.Split(pathPart, "-")[1]
		}
	}
	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      strings.ToLower(serviceName) + "-secure",
			Namespace: strings.ToLower(serviceName),
		},
		Data: myYaml,
	}
	myFile, err := os.Create(*outputPath + "/" + strings.ToLower(serviceName) + ".yaml")
	if err != nil {
		panic(err)
	}
	yamlOut := serializer.NewYAMLSerializer(serializer.DefaultMetaFactory, nil, nil)
	serializerError := yamlOut.Encode(cm, myFile)
	if serializerError != nil {
		panic(serializerError)
	}
}
