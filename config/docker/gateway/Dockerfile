FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --chown=0:0 ./gateway-ctrl /bin/

USER 65532:65532

ENTRYPOINT ["/bin/gateway"]
