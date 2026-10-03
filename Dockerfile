FROM golang:1.27.1 AS build
ARG VERSION=dev
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /aka-webgui ./cmd/aka-webgui
# ブラウザ向けの自己署名証明書を保存するディレクトリ。ボリュームの初期の所有者を nonroot にするため、ここで作る。
RUN mkdir -p /out/data/tls

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /aka-webgui /aka-webgui
COPY --from=build --chown=nonroot:nonroot /out/data /data
ENTRYPOINT ["/aka-webgui"]
CMD ["serve"]
