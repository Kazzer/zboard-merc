.LICENSE := https://gist.github.com/Kazzer/ce54e48904287facce7561c1876c3fa6
.DEFAULT_GOAL := all
.PHONY: all
.SECONDEXPANSION:

DOCKER_BINARY ?= $(shell command -v docker)
GIT_BINARY ?= $(shell command -v git)

COMPOSE_GO = @$(if $(DOCKER_UID_GID),DOCKER_UID_GID=$(DOCKER_UID_GID) )$(DOCKER_BINARY) compose run --rm $(if $(1),$(1) )$(if $(RUN_ARGS),$(RUN_ARGS) )zboard $(2)

ifeq ($(shell uname -s),Linux)
	DOCKER_UID_GID ?= $(shell id -u):$(shell id -g)
endif

GOOS ?= $(shell uname -s | tr '[:upper:]' '[:lower:]')
GOARCH ?= $(shell uname -m | sed -e 's/^x86_64$$/amd64/' -e 's/^aarch64$$/arm64/')

prefix ?= /usr/local
config_prefix ?= /etc
exec_prefix ?= $(prefix)
bindir ?= $(exec_prefix)/bin
modulesloaddir ?= $(prefix)/lib/modules-load.d
systemduserdir ?= $(prefix)/lib/systemd/user
udevdir ?= $(prefix)/lib/udev
udevhwdbdir ?= $(config_prefix)/udev/hwdb.d

INSTALL ?= install
INSTALL_DATA ?= $(INSTALL) -m 644
INSTALL_PROGRAM ?= $(INSTALL)

COMMANDS := zboard zengine zprobe
DESCRIPTION_zboard := ZBoard Agent Command Line
DESCRIPTION_zengine := ZBoard Keyboard Agent
DESCRIPTION_zprobe := ZBoard Keyboard Probe and Capture Tool
DISTRIBUTION = $(shell $(GIT_BINARY) ls-files)
EXECUTABLES_linux := $(COMMANDS)
EXECUTABLES_windows := $(addsuffix .exe,$(COMMANDS) zengine-tray)
GO_SOURCES := $(shell find . -type f -name '*.go')
RESOURCES := $(foreach command,$(COMMANDS),cmd/$(command)/rsrc_windows_amd64.syso cmd/$(command)/rsrc_windows_arm64.syso)
VERSION := $(file <internal/version/VERSION)

all: $(addprefix bin/$(GOOS)-$(GOARCH)/,$(EXECUTABLES_$(GOOS)) $(if $(filter linux,$(GOOS)),zengine.service)) ## Builds the executables for your system into bin/OS-ARCH; specify GOOS=... GOARCH=... to override
	$()

define build_target
.NOTINTERMEDIATE: bin/%/$(1)
bin/%/$(1): $$(GO_SOURCES) $$$$(if $$$$(filter .exe,$$$$(suffix $$$$@)),cmd/$$$$(firstword $$$$(subst -, ,$$$$(basename $$$$(notdir $$$$@))))/rsrc_windows_$$$$(lastword $$$$(subst -, ,$$$$*)).syso)
	@mkdir -p $$(@D)
	$$(call COMPOSE_GO,-T -e GOOS=$$(firstword $$(subst -, ,$$*)) -e GOARCH=$$(lastword $$(subst -, ,$$*)) -e CGO_ENABLED=0,build -trimpath -ldflags='-s -w$$(if $$(filter zengine-tray.exe,$$(notdir $$@)), -H=windowsgui)' -o ./$$@ ./cmd/$$(firstword $$(subst -, ,$$(basename $$(notdir $$@)))))

endef
$(foreach binary,$(COMMANDS),$(eval $(call build_target,$(binary))))
$(foreach binary,$(addsuffix .exe,$(COMMANDS) zengine-tray),$(eval $(call build_target,$(binary))))

.NOTINTERMEDIATE: bin/linux-%/zengine.service
bin/linux-%/zengine.service: packaging/linux/zengine.service
	@mkdir -p $(@D)
	@sed -e 's#^ExecStart=.*#ExecStart=$(bindir)/zengine#' $< >$@

define windows_rsrc_target
.NOTINTERMEDIATE: cmd/%/rsrc_windows_$(1).syso
cmd/%/rsrc_windows_$(1).syso: internal/version/VERSION tools/winres/main.go internal/tray/icon.go
	$(call COMPOSE_GO,-T,run ./tools/winres -arch $(1) -name $$* -desc '$$(DESCRIPTION_$$*)' -o ./$$@)

endef
$(foreach arch,amd64 arm64,$(eval $(call windows_rsrc_target,$(arch))))

.PHONY: bundle
bundle: bundle/$(GOOS)-$(GOARCH) ## Assembles the bundle for your system: executables, installer, and examples; specify GOOS=... GOARCH=... to override
	$()

.NOTINTERMEDIATE: bundle/%
bundle/%: $$(addprefix bin/$$*/,$$(EXECUTABLES_$$(firstword $$(subst -, ,$$*)))) $$(wildcard packaging/$$(firstword $$(subst -, ,$$*))/*) examples/config.yaml $$(wildcard examples/mods/*) ## Assembles the bundle for a specific OS/ARCH (specified as bundle/OS-ARCH)
	@mkdir -p $@
	@cp $(addprefix bin/$*/,$(EXECUTABLES_$(firstword $(subst -, ,$*)))) $(wildcard packaging/$(firstword $(subst -, ,$*))/*) examples/config.yaml $@
	@cp -R examples/mods/ $@
	@touch $@

.PHONY: check
check: ## Runs lint, typecheck, and tests for Linux and Windows
	$(call COMPOSE_GO,,fmt ./...)
	$(call COMPOSE_GO,,vet ./...)
	$(call COMPOSE_GO,-e GOOS=windows,vet ./...)
	$(call COMPOSE_GO,,test ./...)

.PHONY: clean
clean: mostlyclean ## Deletes all resources that are normally created by building the codebase
	@rm -f $(RESOURCES)
	$(eval override CONTAINERS := $(shell $(DOCKER_BINARY) container list --all --filter 'label=service=zboard' --format '{{ .ID }}'))
	$(if $(CONTAINERS),@$(DOCKER_BINARY) container rm --force $(CONTAINERS))

.PHONY: deps
deps: ## Ensures the required dependencies are present
	$(if $(DOCKER_BINARY),,$(error Specify the path to docker using DOCKER_BINARY to continue))
	$(if $(GIT_BINARY),,$(error Specify the path to git using GIT_BINARY to continue))
	$(if $(filter $(MAKECMDGOALS),$@),$(info DOCKER_BINARY := $(DOCKER_BINARY)))
	$(if $(filter $(MAKECMDGOALS),$@),$(info GIT_BINARY := $(GIT_BINARY)))

.PHONY: dist
dist: zboard-merc-$(VERSION).tar.xz ## Creates the source distribution
	$()

.PHONY: distclean
distclean: clean ## Deletes all resources that are created by configuring or building the codebase
	$(eval override IMAGES := $(shell $(DOCKER_BINARY) image list --all --filter 'label=service=zboard' --format '{{ .ID }}'))
	$(if $(IMAGES),@$(DOCKER_BINARY) image rm --force $(IMAGES))
	@$(GIT_BINARY) clean -fXd

docs/keys.md: $(wildcard tools/keysdoc/* internal/hidrd/*.go internal/keys/*.go internal/merc/*.go internal/profile/*.go)
	$(call COMPOSE_GO,-T,run ./tools/keysdoc -o ./$@)

.PHONY: install
install: all installdirs ## Installs executables into bindir; udev rules, modules-load, and systemd user unit into prefix/lib; hwdb into udevhwdbdir
	@$(INSTALL_PROGRAM) $(addprefix bin/$(GOOS)-$(GOARCH)/,$(EXECUTABLES_$(GOOS))) $(DESTDIR)$(bindir)
	@$(INSTALL_DATA) packaging/linux/70-zboard.rules $(DESTDIR)$(udevdir)/rules.d
	@$(INSTALL_DATA) packaging/linux/70-zboard.hwdb $(DESTDIR)$(udevhwdbdir)
	@$(INSTALL_DATA) packaging/linux/zboard-uinput.conf $(DESTDIR)$(modulesloaddir)
	@$(INSTALL_DATA) bin/$(GOOS)-$(GOARCH)/zengine.service $(DESTDIR)$(systemduserdir)

.PHONY: installcheck
installcheck: ## Runs the installed executables and checks that each reports this version
	@$(DESTDIR)$(bindir)/zengine --version | grep -qx 'zengine $(VERSION)'
	@$(DESTDIR)$(bindir)/zboard version | grep -qx 'zboard $(VERSION)'
	@$(DESTDIR)$(bindir)/zprobe --version | grep -qx 'zprobe $(VERSION)'

.PHONY: installdirs
installdirs: ## Creates directories for installation, if necessary
	@mkdir -p $(DESTDIR)$(bindir) $(DESTDIR)$(udevdir)/rules.d $(DESTDIR)$(udevhwdbdir) $(DESTDIR)$(modulesloaddir) $(DESTDIR)$(systemduserdir)

.PHONY: maintainer-clean
maintainer-clean: distclean ## Deletes all resources that are not obtained from the codebase
	@$(GIT_BINARY) clean -fxdf

.PHONY: mostlyclean
mostlyclean: ## Deletes most resources that can be regenerated
	@rm -rf bin bundle zboard-merc-*.tar.xz SHA256SUMS

.PHONY: release
release: zboard-merc-$(VERSION)-$(GOOS)-$(GOARCH).tar.xz ## Creates the release artifact for your system: the bundle as zboard-merc-VERSION-OS-ARCH.tar.xz; specify GOOS=... GOARCH=... to override
	$()

.PHONY: run-interactive
run-interactive: deps ## Runs an interactive shell in the build container
	@$(if $(DOCKER_UID_GID),DOCKER_UID_GID=$(DOCKER_UID_GID) )$(DOCKER_BINARY) compose run --rm --entrypoint /bin/sh $(if $(RUN_ARGS),$(RUN_ARGS) )zboard

.PHONY: uninstall
uninstall: ## Uninstall the executables from the system
	@rm -f $(addprefix $(DESTDIR)$(bindir)/,$(EXECUTABLES_$(GOOS))) $(DESTDIR)$(udevdir)/rules.d/70-zboard.rules $(DESTDIR)$(udevhwdbdir)/70-zboard.hwdb $(DESTDIR)$(modulesloaddir)/zboard-uinput.conf $(DESTDIR)$(systemduserdir)/zengine.service

.PHONY: usage
usage: ## Displays this message
	@grep -E '^[A-Za-z0-9_/\.%-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort -t: -k1,1V | sed -e 's/^\([^:]*\):.*## /\x1b[36m\1\x1b[0m|/' | awk -F '|' '{for(i=1;i<=NF;i++) if(length($$i)>w[i]) w[i]=length($$i); l[NR]=$$0; if(NF>f) f=NF} END {for(n=1;n<=NR;n++) {split(l[n],a,"|"); for(i=1;i<=f;i++) if(i<f) {printf "%-*s  ",w[i],a[i]} else {print a[i]}}}'

.PHONY: version
version: ## Displays the current version of this codebase
	@echo $(VERSION)

zboard-merc-$(VERSION).tar.xz: docs/keys.md $$(DISTRIBUTION)
	@tar --create --xz --file $@ --transform 's,^,zboard-merc-$(VERSION)/,' $(DISTRIBUTION)

zboard-merc-$(VERSION)-%.tar.xz: bundle/%
	@tar --create --xz --file $@ --directory bundle --transform 's,^,zboard-merc-$(VERSION)-,' $*
