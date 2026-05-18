VERSION := $(shell ./hack/version)
COMMIT := $(shell git rev-parse HEAD)
MODULE := $(shell grep '^module' go.mod | awk '{print $$2}')
BUILD_META :=
BUILD_META += -X=$(MODULE)/pkg/version.Version=$(VERSION)
BUILD_META += -X=$(MODULE)/pkg/version.Commit=$(COMMIT)
PREFIX ?= /usr

SHELL := $(shell command -v bash;)
BASHINSTALLDIR=${PREFIX}/share/bash-completion/completions
ZSHINSTALLDIR=${PREFIX}/share/zsh/site-functions
FISHINSTALLDIR=${PREFIX}/share/fish/vendor_completions.d

SELINUXOPT ?= $(shell test -x /usr/sbin/selinuxenabled && selinuxenabled && echo -Z)

LDFLAGS := -ldflags "$(BUILD_META)"

all: build completions

clean:
	if [[ -e .git ]]; then git clean -dfX; fi

.PHONY: completions
completions: build
	rm -rf completions
	mkdir -p completions/{bash,zsh,fish,powershell}
	./leaktk completion bash > completions/bash/leaktk
	./leaktk completion zsh > completions/zsh/_leaktk
	./leaktk completion fish > completions/fish/leaktk.fish
	./leaktk completion powershell > completions/powershell/leaktk.ps1

vet:
	go vet ./...

.PHONY: lint
lint: vet
	golangci-lint run

build:
	CGO_ENABLED=0 go build $(LDFLAGS)

import:
	goimports -local $(MODULE) -l -w .
	go mod tidy

format: import
	go fmt ./...
	find . -type f \( -name '*.md' -or -name '*.go' -or -name '*.yaml' -or -name Makefile -or -path './hack/*' \) | xargs sed -i 's/[ \t]*$$//g'

test: format vet lint
	go test -race $(MODULE) ./...

failfast:
	go test -failfast github.com/leaktk/leaktk ./...

install:
	install ${SELINUXOPT} -d -m 0755 $(DESTDIR)$(PREFIX)/bin
	install ${SELINUXOPT} -m 0755 leaktk $(DESTDIR)$(PREFIX)/bin

.PHONY: install.completions
install.completions:
	install ${SELINUXOPT} -d -m 0755 $(DESTDIR)${BASHINSTALLDIR}
	install ${SELINUXOPT} -m 0644 completions/bash/leaktk $(DESTDIR)${BASHINSTALLDIR}
	install ${SELINUXOPT} -d -m 0755 $(DESTDIR)${ZSHINSTALLDIR}
	install ${SELINUXOPT} -m 0644 completions/zsh/_leaktk $(DESTDIR)${ZSHINSTALLDIR}
	install ${SELINUXOPT} -d -m 0755 $(DESTDIR)${FISHINSTALLDIR}
	install ${SELINUXOPT} -m 0644 completions/fish/leaktk.fish $(DESTDIR)${FISHINSTALLDIR}

security-report:
	trivy fs .

update:
	go get -u ./...
	go mod tidy

.PHONY: validate.completions
validate.completions: SHELL:=/usr/bin/env bash
validate.completions: completions
	. completions/bash/leaktk
	if [ -x /bin/zsh ]; then /bin/zsh completions/zsh/_leaktk; fi
	if [ -x /bin/fish ]; then /bin/fish completions/fish/leaktk.fish; fi
