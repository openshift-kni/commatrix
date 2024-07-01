FORMAT ?= csv
CLUSTER_ENV ?= baremetal
DEST_DIR ?= .
DEPLOYMENT ?= mno
GO_SRC := cmd/main.go
OC_VERSION_TAG := 4.15.0-202402082307

EXECUTABLE := commatrix-gen

.DEFAULT_GOAL := run

build:
	go build -o $(EXECUTABLE) $(GO_SRC) 

oc:
ifeq (, $(shell which oc))
	@{ \
	set -e ;\
	curl -LO https://github.com/openshift/oc/archive/refs/tags/openshift-clients-$(OC_VERSION_TAG).tar.gz ;\
	tar -xf openshift-clients-$(OC_VERSION_TAG).tar.gz ;\
	cd $(PWD)/oc-openshift-clients-$(OC_VERSION_TAG) ;\
	make oc ;\
	mv oc $(GOBIN)/oc ;\
	chmod u+x $(GOBIN)/oc ;\
	rm -rf $(PWD)/oc-openshift-clients-$(OC_VERSION_TAG) ;\
	rm $(PWD)/openshift-clients-$(OC_VERSION_TAG).tar.gz ;\
	}
endif

generate: oc build
	rm -rf $(DEST_DIR)/communication-matrix
	mkdir -p $(DEST_DIR)/communication-matrix
	./$(EXECUTABLE) -format=$(FORMAT) -env=$(CLUSTER_ENV) -destDir=$(DEST_DIR)/communication-matrix -deployment=$(DEPLOYMENT) -customEntriesPath=$(CUSTOM_ENTRIES_PATH) -customEntriesFormat=$(CUSTOM_ENTRIES_FORMAT)

apply-firewall: build
	./$(EXECUTABLE) -applyFirewall -env=$(CLUSTER_ENV) -destDir=$(DEST_DIR)/communication-matrix -deployment=$(DEPLOYMENT)

clean:
	@rm -f $(EXECUTABLE)
