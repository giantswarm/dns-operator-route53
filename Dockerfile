# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /

ARG TARGETOS
ARG TARGETARCH
COPY dns-operator-route53-${TARGETOS}-${TARGETARCH} /dns-operator-route53

USER nonroot:nonroot

ENTRYPOINT ["/dns-operator-route53"]
