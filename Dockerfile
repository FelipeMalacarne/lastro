# syntax=docker/dockerfile:1

# 1. Frontend
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# 2. Static server binary with the frontend embedded (the lastro client is not in the image)
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/lastrod ./cmd/lastrod

# 3. Final image: just the binary
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/lastrod /lastrod
# Numeric UID and GID of distroless's nonroot user: with runAsNonRoot, Kubernetes refuses
# to start a container whose user is a name, because it cannot verify it is not root.
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/lastrod"]
CMD ["serve"]
