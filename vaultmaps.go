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
	"os/exec"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
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

func getVaultToken(githubToken string, httpclient http.Client, vaultAddress string) string {
	// format our data
	authRequestBody := &AuthRequestBody{
		Token: githubToken,
	}
	// encode in json
	jsonBody, jsonError := json.Marshal(authRequestBody)
	if jsonError != nil {
		panic(jsonError)
	}
	// set up reqeust
	tokenReq, _ := http.NewRequest(http.MethodPut, vaultAddress+"/v1/auth/github/login", bytes.NewBuffer(jsonBody))
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
	return tokenResponseJSON.Auth.ClientToken
}

func getSecretValues(vaultToken string, httpclient http.Client, vaultAddress string, secretPath string) string {
	// set up request for secrets
	secretReq, _ := http.NewRequest(http.MethodGet, vaultAddress+"/v1/"+secretPath, bytes.NewBuffer([]byte{}))
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
	// extract secret to a map
	return secretResponseJSON.Data.Data.HelmSecretValues
}

func secretsToYaml(helmSecretValues string) []byte {
	mySecrets := make(map[string]interface{})
	for _, value := range strings.Split(helmSecretValues, ",") {
		splitty := strings.Split(value, "=")
		splittwo := strings.Split(splitty[0], ".")
		var myvalue string
		ptr := mySecrets
		for index, val := range splittwo {
			if _, ok := ptr[val]; !ok {
				ptr[val] = map[string]interface{}{}
			}
			if index != len(splittwo)-1 {
				// Advance the map pointer deeper into the map.
				ptr = ptr[val].(map[string]interface{})
			} else {
				myvalue = val
			}
		}
		// should have made all the nodes along the way by now
		ptr[myvalue] = splitty[1]
	}
	myYaml, yErr := yaml.Marshal(mySecrets)
	if yErr != nil {
		fmt.Println("yaml error:", yErr)
	}
	return myYaml
}

func getServiceName(secretPath string) string {
	s := strings.Split(secretPath, "/")
	for _, pathPart := range s {
		if strings.Contains(pathPart, "service") {
			return strings.Split(pathPart, "-")[1]
		}
	}
	return "dy"
}

func encryptSecrets(path string) {
	publicKey := "age1mn59crksy4luzytctr39u492jcdfvq005p4nunryqas03ckvp9tsx4ew40"
	app := "sops"
	args := []string{
		"--age=" + publicKey,
		"--encrypt",
		"--encrypted-regex",
		"^(data|password)$",
		path,
	}

	cmd := exec.Command(app, args...)
	cmd.Stdout, _ = os.OpenFile(path, os.O_RDWR, 0666)
	fmt.Printf("cmd: %v\n", cmd)
	cmd.Run()
}

func main() {
	// get cli args
	var vaultAddress = flag.String("vault-address", LookupEnvOrString("VAULT_ADDRESS", "https://vault.ps.thmulti.com:8200"), "vault address")
	var githubToken = flag.String("github-token", LookupEnvOrString("GITHUB_TOKEN", "fake-token"), "your github token")
	var secretPath = flag.String("secret-path", LookupEnvOrString("SECRET_PATH", "fake-path"), "secret path")
	var outputPath = flag.String("output-path", LookupEnvOrString("OUTPUT_PATH", "."), "path to output file, default is .")
	flag.Parse()
	log.Printf("app.config %v\n", getConfig(flag.CommandLine))
	// set up client
	httpclient := &http.Client{
		Timeout: 0,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	// get vault token
	vaultToken := getVaultToken(*githubToken, *httpclient, *vaultAddress)
	// get secrets
	helmSecretValues := getSecretValues(vaultToken, *httpclient, *vaultAddress, *secretPath)
	// format yaml
	myYaml := secretsToYaml(helmSecretValues)
	// get service name
	serviceName := getServiceName(*secretPath)
	// create our configmap
	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      strings.ToLower(serviceName) + "-secure",
			Namespace: strings.ToLower(serviceName),
		},
		Data: map[string]string{
			"values.yaml": string(myYaml),
		},
	}
	// write it out
	myFile, err := os.Create(*outputPath + "/" + strings.ToLower(serviceName) + "-secure.yaml")
	if err != nil {
		panic(err)
	}
	yamlOut := serializer.NewYAMLSerializer(serializer.DefaultMetaFactory, nil, nil)
	serializerError := yamlOut.Encode(cm, myFile)
	if serializerError != nil {
		panic(serializerError)
	}
	encryptSecrets(myFile.Name())
	// party
}
