SUDACHI_DICT_VERSION ?= 20260723
SUDACHI_DICT_SHA256 ?= b6e835f63440f97474c2da45d80950f73746e632e40bbfc168b4041729135e1f
STATICCHECK_VERSION ?= 2026.2.1
DICT_DIR := .cache/sudachi
DICT := $(DICT_DIR)/system_core.dic
DICT_URL := https://d2ej7fkh96fzlu.cloudfront.net/sudachidict/sudachi-dictionary-$(SUDACHI_DICT_VERSION)-core.zip

.PHONY: build test test-dict vet lint cover

build:
	go build -o bin/natural-japanese ./cmd/natural-japanese

test:
	go test -race ./...

$(DICT):
	mkdir -p $(DICT_DIR)
	curl -fsSL -o $(DICT_DIR)/dict.zip $(DICT_URL)
	echo "$(SUDACHI_DICT_SHA256)  $(DICT_DIR)/dict.zip" | shasum -a 256 -c -
	unzip -o -j $(DICT_DIR)/dict.zip '*.dic' -d $(DICT_DIR)
	rm $(DICT_DIR)/dict.zip

test-dict: $(DICT)
	SUDACHIN_DICT=$(abspath $(DICT)) go test -race ./...

vet:
	go vet ./...

lint:
	go run honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION) ./...

cover: $(DICT)
	SUDACHIN_DICT=$(abspath $(DICT)) go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1
