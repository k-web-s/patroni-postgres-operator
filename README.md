# patroni-postgres-operator

Still in **BETA**, use with care.

A Kubernetes operator for Postgresql clusters managed by [Patroni](https://patroni.readthedocs.io/). Can do major Postgresql version upgrades without significant downtime.

Uses [postgres-patroni](https://github.com/rkojedzinszky/postgres-patroni) images, which supports Postgresql versions 13 and 15 only.

## Deploy the Operator

To quickly deploy the operator, run:

```shell
$ kubectl apply -k https://github.com/k-web-s/patroni-postgres-operator/config/default/
```

## Create a patronipostgres instance

The following minimal object creates a PatroniPostgres instance with one node:

```yaml
apiVersion: kwebs.cloud/v1alpha1
kind: PatroniPostgres
metadata:
  name: patroni-postgres
spec:
  version: 15
  volumeSize: 5Gi
  nodes:
  - storageClassName: default
```

The number of `node` definitions specifies the cluster size. At least each node definition must have a `storageClassName` attribute. See full [reference](api/v1alpha1/patronipostgres_types.go). The operator will create a service with the same name as the object, which can be used to access the patronipostgres cluster. No users/databases are created. Superuser credentials are stored in a secret with the same name as the object. Superuser username is `postgres`, and the password can be obtained by:

```shell
$ kubectl --context=pi-kubernetes -n db get secret patroni-postgres --template '{{index .data "superuser-password"}}' | base64 -d
```

Check more [samples](config/samples/).

## Scaling the cluster

Adding new nodes is just as easy as extending `nodes` array. Removing also works, howewer, only removing nodes from the end of the array is supported. Changing a `storageClassName` in a node definition is not supported.
