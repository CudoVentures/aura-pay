VERSION := $(shell echo $(shell git describe --tags) | sed 's/^v//')
COMMIT  := $(shell git log -1 --format='%H')

export GO111MODULE = on

###############################################################################
###                                   All                                   ###
###############################################################################

all: build

###############################################################################
###                                Build flags                              ###
###############################################################################

LD_FLAGS = -X github.com/CudoVentures/aura-pay/cmd.Version=$(VERSION) \
	-X github.com/CudoVentures/aura-pay/cmd.Commit=$(COMMIT)
BUILD_FLAGS :=  -ldflags '$(LD_FLAGS)'


###############################################################################
###                                  Build                                  ###
###############################################################################

build: go.sum
ifeq ($(OS),Windows_NT)
	@echo "building cudos-markets-pay binary..."
	@go build -mod=readonly $(BUILD_FLAGS) -o build/cudos-markets-pay.exe ./cmd/cudos-markets-pay
else
	@echo "building cudos-markets-pay binary..."
	@go build -mod=readonly $(BUILD_FLAGS) -o build/cudos-markets-pay ./cmd/cudos-markets-pay
endif
.PHONY: build

###############################################################################
###                                 Install                                 ###
###############################################################################

install: go.sum
	@echo "installing cudos-markets-pay binary..."
	@go install -mod=readonly $(BUILD_FLAGS) ./cmd/cudos-markets-pay
.PHONY: install
