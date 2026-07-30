# encapi

A Postgres-backed **External Node Classifier (ENC)** for Puppet, written in Go.
It replaces Cobbler as the source of truth for *which role a host runs* while
keeping Puppet's node-classification contract byte-compatible.

## Components

| Binary          | What it is                                                            |
|-----------------|-----------------------------------------------------------------------|
| `encapi`        | HTTP server: serves ENC documents + a read/write API. Postgres-backed. |
| `encapi-cli`    | CLI over the API. `encapi-cli classify <host>` is the Puppet ENC.      |
| `encapi-enc`    | Thin wrapper (shipped in the RPM) for Puppet's `external_nodes`.       |

The Terraform provider lives in a sibling repo:
[`terraform-provider-encapi`](https://git.example.com/example/terraform-provider-encapi).

## Data model

- **status** — a Puppet environment (Cobbler's "status": testing, production…).
- **role** — a class assignment target (`roles::infra::storage::vault`) with
  inheritable `default_params`.
- **node** — a host (certname) pinned to one role + one environment, with
  optional per-node `params` (which win over the role defaults).

Foreign keys guarantee a node can only reference a role/status that exists, and
a role/status in use cannot be deleted.

## ENC output

`encapi` renders two shapes from the same data:

- **`GET /api/v1/nodes/{certname}/enc`** — the reshaped document Puppet's
  `exec` terminus consumes: `classes` as a list, `environment` dropped when it
  is `testing`, and `parameters` carrying `enc_role` (list) + `enc_env`. This is
  what `encapi-cli classify` prints.
- **`GET /cblr/svc/op/puppet/hostname/{certname}`** — the cobbler-wire form
  (`classes` as a map, `environment` always present), so the legacy
  `enc_direct_facts.rb` fact can be repointed with only a URL change.

See [`docs/cutover.md`](docs/cutover.md) for the migration from Cobbler.

## API

Reads are open. Writes require `Authorization: Bearer $ENCAPI_WRITE_TOKEN`
(set the token in the server's environment; manage it in Vault).

```
GET    /healthz
GET    /api/v1/nodes                      list nodes
GET    /api/v1/nodes/{certname}           get node
PUT    /api/v1/nodes/{certname}           upsert node        (token)
DELETE /api/v1/nodes/{certname}           delete node        (token)
GET    /api/v1/nodes/{certname}/enc       reshaped ENC (YAML)
GET    /cblr/svc/op/puppet/hostname/{h}   cobbler-wire ENC (YAML)
GET    /api/v1/roles       ... /roles/{name}      (PUT/DELETE token)
GET    /api/v1/statuses    ... /statuses/{name}   (PUT/DELETE token)
```

## Configuration (server env)

| Var                    | Default       | Purpose                                  |
|------------------------|---------------|------------------------------------------|
| `LISTEN_ADDR`          | `:8000`       | listen address                           |
| `DBHOST/DBPORT/DBUSER/DBPASS/DBNAME/DBSSL` | localhost/5432/encapi/encapi/encapi/disable | Postgres |
| `ENCAPI_WRITE_TOKEN`   | *(unset)*     | bearer token for writes; unset = read-only |
| `ENCAPI_DISTRO_API_URL`| *(unset)*     | optional kickstart/distro API for provisioning params |

## CLI

```bash
export ENCAPI_URL=https://encapi.example.com
export ENCAPI_WRITE_TOKEN=…            # only needed for writes

encapi-cli status set production
encapi-cli role set roles::infra::storage::minio --param 'replicas=4' --param 'epel="9"'
encapi-cli node set host2100.int.example.net --role roles::infra::storage::minio --env production
encapi-cli classify host2100.int.example.net

# one-shot migration from the live Cobbler estate:
encapi-cli import-cobbler --dry-run
encapi-cli import-cobbler
```

## Local development

```bash
docker compose up -d          # postgres + encapi
make test                     # full suite (Postgres via testcontainers)
make test-short               # skip container-backed DB tests
```

## Releases

- **`encapi` server image** — tagging `vX.Y.Z` builds and pushes
  `artifactapi.example.com/docker-internal/encapi:{tag,latest}` (distroless).
- **`encapi-cli` RPM** — the same tag builds an RPM (nfpm) and publishes it to
  the ArtifactAPI `rpm-internal` repo. Installs `encapi-cli`, the `encapi-enc`
  Puppet wrapper, and `/etc/encapi/enc.conf`.
