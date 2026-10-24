# Кодогенерация из proto/. Версии зафиксированы: шапка сгенерированных файлов
# содержит версии protoc и плагинов, а CI сравнивает результат побайтно.
PROTOC_VERSION             := 36.2
PROTOC_GEN_GO_VERSION      := v1.36.11
PROTOC_GEN_GO_GRPC_VERSION := v1.5.1

PROTO_DIR   := proto
PROTO_FILES := $(shell find $(PROTO_DIR) -name '*.proto')

# $(1) — модуль сервиса, $(2) — каталог для кода внутри модуля
define gen
	rm -rf $(1)/$(2)
	protoc -I $(PROTO_DIR) \
		--go_out=$(1) --go_opt=module=$(1) --go_opt=Mproduct/v1/product.proto=$(1)/$(2) \
		--go-grpc_out=$(1) --go-grpc_opt=module=$(1) --go-grpc_opt=Mproduct/v1/product.proto=$(1)/$(2) \
		$(PROTO_FILES)
endef

.PHONY: proto proto-tools proto-check

## proto-tools: установить плагины нужных версий
proto-tools:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VERSION)

## proto: сгенерировать код в оба сервиса (сервер — product-service, клиент — order-service)
proto:
	@protoc --version | grep -q ' $(PROTOC_VERSION)$$' || { echo "нужен protoc $(PROTOC_VERSION), установлен: $$(protoc --version)"; exit 1; }
	$(call gen,order-service,internal/gen/productv1)
	$(call gen,product-service,gen/productv1)

## proto-check: сгенерированный код совпадает с .proto (для CI)
proto-check: proto
	@git diff --exit-code -- order-service/internal/gen product-service/gen || { echo '.proto и сгенерированный код разошлись: запусти make proto и закоммить'; exit 1; }
	@test -z "$$(git status --porcelain -- order-service/internal/gen product-service/gen)" || { echo 'есть незакоммиченные сгенерированные файлы'; git status --porcelain -- order-service/internal/gen product-service/gen; exit 1; }
