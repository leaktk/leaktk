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
	declare -A outfiles=([bash]=%s [zsh]=_%s [fish]=%s.fish [powershell]=%s.ps1);\
	for shell in $${!outfiles[*]}; do \
		mkdir -p "completions/$$shell"; \
		outfile=$$(printf "completions/$$shell/$${outfiles[$$shell]}" leaktk); \
		./leaktk completion $$shell >| $$outfile; \
	done

vet:
	go vet ./...

.PHONY: lint
lint: vet
	golangci-lint run

build: import
	go build $(LDFLAGS)

import:
	goimports -local $(MODULE) -l -w .
	go mod tidy

format: import
	go fmt ./...

test: format vet lint
	go test -race $(MODULE) ./...

failfast:
	go test -failfast github.com/leaktk/leaktk ./...

install:
	install ./leaktk $(DESTDIR)$(PREFIX)/bin/leaktk

.PHONY: install.completions
install.completions:
	install ${SELINUXOPT} -d -m 755 $(DESTDIR)${BASHINSTALLDIR}
	install ${SELINUXOPT} -m 644 completions/bash/leaktk $(DESTDIR)${BASHINSTALLDIR}
	install ${SELINUXOPT} -d -m 755 $(DESTDIR)${ZSHINSTALLDIR}
	install ${SELINUXOPT} -m 644 completions/zsh/_leaktk $(DESTDIR)${ZSHINSTALLDIR}
	install ${SELINUXOPT} -d -m 755 $(DESTDIR)${FISHINSTALLDIR}
	install ${SELINUXOPT} -m 644 completions/fish/leaktk.fish $(DESTDIR)${FISHINSTALLDIR}

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
