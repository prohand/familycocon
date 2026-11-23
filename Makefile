# Makefile de FamilyCocon
#
#   make lancer    lance le serveur en local (http://localhost:8080, données dans ./donnees)
#   make test      tests (avec détection des accès concurrents)
#   make verif     formatage, go vet, code SQL généré à jour
#   make sql       régénère le code d'accès à la base (sqlc) après un changement de requête
#   make binaires  binaires statiques linux/amd64 et linux/arm64 dans dist/
#   make image     image Docker locale (architecture de la machine)

export GOTOOLCHAIN := go1.27.4
# sqlc : surchargeable (make verif SQLC=~/outils/sqlc), comme le fait la CI.
SQLC ?= go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null)
LDFLAGS := -s -w -buildid= -X familycocon/internal/version.Version=$(VERSION) -X familycocon/internal/version.Commit=$(COMMIT)

.PHONY: lancer test verif sql binaires image

lancer:
	FC_URL_PUBLIQUE=http://localhost:8080 FC_FORMAT_LOGS=texte go run ./cmd/familycocon

test:
	go test -race ./...

verif:
	@test -z "$$(gofmt -l .)" || (echo "fichiers mal formatés :"; gofmt -l .; exit 1)
	go vet ./...
	$(SQLC) diff

sql:
	$(SQLC) generate

binaires:
	@for arch in amd64 arm64; do \
	  CGO_ENABLED=0 GOOS=linux GOARCH=$$arch go build -trimpath -buildvcs=false -ldflags "$(LDFLAGS)" \
	    -o dist/familycocon-linux-$$arch ./cmd/familycocon || exit 1; \
	done
	@ls -lh dist/

image:
	docker build --build-arg VERSION=$(VERSION) -t familycocon:$(VERSION) .
