SHELL := /bin/bash # Use bash syntax

# Set up variables
GO111MODULE=on

GO_CMD_FLAGS=-tags codegen

AWS_SERVICE:=$(shell echo $(SERVICE))

TEMPLATE_CONTROLLER:=./bin/controller-bootstrap

ROOT_DIR:=$(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))
CODE_GEN_DIR:=${ROOT_DIR}/../code-generator
CONTROLLER_DIR:=${ROOT_DIR}/../${AWS_SERVICE}-controller
EXISTING_CONTROLLER=true

# Provide the latest versions of aws-sdk-go releases and runtime releases
# todo: provide user-friendly option to specify the version via command line input
#AWS_SDK_GO_VERSION=$(shell curl -H "Accept: application/vnd.github.v3+json" \
#                                 https://api.github.com/repos/aws/aws-sdk-go/releases/latest | jq -r '.tag_name')
#RUNTIME_VERSION=$(shell curl -H "Accept: application/vnd.github.v3+json" \
#								  https://api.github.com/repos/aws-controllers-k8s/runtime/releases/latest | jq -r '.tag_name')

AWS_SDK_GO_VERSION:=$(shell curl -sL https://github.com/aws/aws-sdk-go/releases/latest | grep -oE 'v+[0-9]+\.[0-9]+\.[0-9]+' | head -n 1 )

RUNTIME_VERSION:=$(shell curl -sL https://github.com/aws-controllers-k8s/runtime/releases/latest | grep -oE 'v+[0-9]+\.[0-9]+\.[0-9]+' | head -n 1 )

build:
	@go build ${GO_CMD_FLAGS} -o ${TEMPLATE_CONTROLLER} ./cmd/controller-bootstrap/*.go

bootstrap-controller: build
	@${TEMPLATE_CONTROLLER} generate -o ${ROOT_DIR}/../${AWS_SERVICE}-controller -v ${AWS_SDK_GO_VERSION} -r ${RUNTIME_VERSION} -e=false -- ${AWS_SERVICE}

update-controller: build
	@${TEMPLATE_CONTROLLER} generate -o ${ROOT_DIR}/../${AWS_SERVICE}-controller -v ${AWS_SDK_GO_VERSION} -r ${RUNTIME_VERSION} -e=${EXISTING_CONTROLLER} -- ${AWS_SERVICE}

initialize-controller:
	@if [ -f ${CONTROLLER_DIR}/cmd/controller/main.go ]; then \
	  make update-controller; \
	  echo "controller exists"; \
	else \
	  make bootstrap-controller; \
	  echo "controller doesn't exist"; \
	fi

generate-controller: bootstrap-controller
	@export SERVICE=${AWS_SERVICE}
	@echo "build controller attempt #1"
	@cd ${CODE_GEN_DIR} && make -i build-controller >/dev/null 2>/dev/null
	@echo "missing go.sum entry, running go mod tidy"
	@cd ${CONTROLLER_DIR} && go mod tidy
	@echo "build controller attempt #2"
	@cd ${CODE_GEN_DIR} && make -i build-controller >/dev/null 2>/dev/null
	@echo "go.sum outdated, running go mod tidy"
	@cd ${CONTROLLER_DIR} && go mod tidy
	@echo "final build controller attempt"
	@cd ${CODE_GEN_DIR} && make build-controller
	@echo "look inside ${AWS_SERVICE}-controller/generator.yaml for further instructions"

clean-controller-dir:
	@cd ${CONTROLLER_DIR}
	@rm -rf ${CONTROLLER_DIR}/..?* ${CONTROLLER_DIR}/.[!.]* ${CONTROLLER_DIR}/*
