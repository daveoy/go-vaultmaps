FROM golang:1.18.3
ENV GOOS linux
RUN mkdir /app
ADD . /app
WORKDIR /app
RUN go get -v "k8s.io/api/core/v1" \
	 "k8s.io/apimachinery/pkg/apis/meta/v1" \
	 "k8s.io/apimachinery/pkg/runtime/serializer/json" \
	 "filippo.io/age"
RUN go build -o vaultmaps . && chmod +x vaultmaps
ENTRYPOINT ["/app/vaultmaps"]