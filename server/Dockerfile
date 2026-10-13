# ---- Build stage: compile the Go binary ----
FROM golang:1.27-alpine AS build
WORKDIR /src

# Download dependencies first so this layer is cached between builds.
COPY go.mod go.sum* ./
RUN go mod download

# Copy the source and build a static binary.
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server .

# ---- Runtime stage: tiny image with just the binary ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /server
ENV PORT=8080
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/server"]
