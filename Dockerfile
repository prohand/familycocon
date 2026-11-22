# SPDX-License-Identifier: GPL-3.0-or-later
#
# Image de FamilyCocon : binaire statique seul (pas de shell, pas d'outil),
# utilisateur non root, amd64 et arm64 (Raspberry Pi).
#
# La compilation tourne TOUJOURS sur l'architecture de la machine de build
# ($BUILDPLATFORM) et produit le binaire de l'architecture visée
# ($TARGETARCH) : pas d'émulation, quelques secondes par architecture.

# --- construction -------------------------------------------------------------
FROM --platform=$BUILDPLATFORM golang:1.27.4-bookworm AS construction
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS=linux
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -buildvcs=false \
      -ldflags "-s -w -buildid= -X familycocon/internal/version.Version=${VERSION} -X familycocon/internal/version.Commit=${COMMIT}" \
      -o /familycocon ./cmd/familycocon
# Dossier des données préparé ici : l'image finale n'a pas de shell pour le créer.
RUN mkdir -p /out/donnees/sauvegardes

# --- image finale ---------------------------------------------------------------
FROM scratch
# Certificats : pour envoyer les notifications Web Push (HTTPS sortant).
COPY --from=construction /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=construction /familycocon /familycocon
COPY --from=construction /src/LICENSE /licences/
COPY --from=construction /src/web/statique/fonts/OFL.txt /licences/nunito-OFL.txt
# Le volume reprend ce propriétaire à sa création (non root, uid 65532).
COPY --from=construction --chown=65532:65532 /out/donnees /donnees
ARG VERSION=dev
LABEL org.opencontainers.image.title="familycocon" \
      org.opencontainers.image.description="Organisation familiale auto-hébergée" \
      org.opencontainers.image.source="https://github.com/prohand/familycocon" \
      org.opencontainers.image.licenses="GPL-3.0-or-later" \
      org.opencontainers.image.version="${VERSION}"
USER 65532:65532
ENV FC_DONNEES=/donnees \
    FC_ADRESSE=:8080 \
    FC_FORMAT_LOGS=json \
    GOMEMLIMIT=48MiB
VOLUME ["/donnees"]
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD ["/familycocon", "-sante"]
ENTRYPOINT ["/familycocon"]
