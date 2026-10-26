# syntax=docker/dockerfile:1

# 1. Frontend
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# 2. Static binary with the frontend embedded
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/lastro ./cmd/lastro

# 3. Final image: just the binary
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/lastro /lastro
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/lastro"]
CMD ["serve"]
