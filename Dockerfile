FROM golang:1.15 as build

WORKDIR /go/src/github.com/webdevops/azure-msi-operator

# Get deps (cached)
COPY ./go.mod /go/src/github.com/webdevops/azure-msi-operator
COPY ./go.sum /go/src/github.com/webdevops/azure-msi-operator
COPY ./Makefile /go/src/github.com/webdevops/azure-msi-operator
RUN make dependencies

# Compile
COPY ./ /go/src/github.com/webdevops/azure-msi-operator
RUN make lint
RUN make build
RUN ./azure-msi-operator --help

#############################################
# FINAL IMAGE
#############################################
FROM gcr.io/distroless/static

ENV LOG_JSON=1

COPY --from=build /go/src/github.com/webdevops/azure-msi-operator/azure-msi-operator /
USER 1000
ENTRYPOINT ["/azure-msi-operator"]
