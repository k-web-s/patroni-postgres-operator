FROM scratch

ARG TARGETARCH

LABEL org.opencontainers.image.authors "Richard Kojedzinszky <richard@kojedz.in>"

COPY patroni-postgres-operator.${TARGETARCH} /patroni-postgres-operator

USER 19282

ENTRYPOINT ["/patroni-postgres-operator"]
