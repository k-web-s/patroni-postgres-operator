FROM scratch

ARG TARGETARCH

LABEL org.opencontainers.image.authors "Richard Kojedzinszky <richard@kojedz.in>"

COPY patroni-postgres-operator.${TARGETARCH} /patroni-postgres-operator
COPY upgrade.${TARGETARCH} /upgrade

USER 19282

ENTRYPOINT ["/patroni-postgres-operator"]
