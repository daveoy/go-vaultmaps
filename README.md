# get configmaps into clusters from vault

this repo contains some go code to pull secrets from a predefined path in vault and output a kubernetes configmap object for application directly to a kubernetes cluster or for encryption and storage in version control to keep history of versioned objects that  contain secrets.

## how to use this software
git clone this repo, cd into `go-vaultmaps`:

usage:
```
Usage of vaultmaps:
  -github-token string
    	your github token (default "fake-token")
  -output-path string
    	path to output file, default is . (default ".")
  -secret-path string
    	secret path (default "fake-path")
  -vault-address string
    	vault address (default "https://vault.ps.thmulti.com:8200")
```
you can also set commandline flags with environment variables:
```
VAULT_ADDR="<https:// address for vault>" 
SECRET_PATH="<vault path>" 
OUTPUT_PATH="<path>" 
GITHUB_TOKEN="<token>"
```

## how to build with this repo
git clone this repo, cd into go-vaultmaps, then:

```
go get -v "k8s.io/api/core/v1" \
	 "k8s.io/apimachinery/pkg/apis/meta/v1" \
	 "k8s.io/apimachinery/pkg/runtime/serializer/json"
go build -o vaultmaps . && chmod +x vaultmaps
```

## docker
if you haven't got go set up you can always  git clone this repo, then 
```
docker build . -t vaultmaps
```
in this case you'll need to use the -output-path option like so
```
docker run -v `pwd`:/tmp vaultmaps -vault-address="https://vault.ps.thmulti.com:8200" -github-token="<your-token>" -secret-path=github/data/mpc-film/secured/service-multisiteQueue/bglr-stage -output-path=/tmp
```