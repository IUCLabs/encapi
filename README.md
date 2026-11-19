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

## Schema migrations

The SQL lives in [`migrations/`](migrations), is embedded in the `encapi`
binary, and is applied at startup before the server listens — there is no
mirrored copy in the deployment to drift out of sync.

The runner is [`golib/pg`](https://github.com/IUCLabs/golib)'s
`pg.NewMigrated`: encapi owns the SQL, the shared library owns the mechanics.
Each start takes `pg_advisory_lock` on a key derived from the lock name
`encapi-migrations`, creates `schema_migrations` (`version`, `applied_at`) if
missing, and applies every embedded file whose filename is not yet recorded — in
lexical (version) order, each file's SQL and its tracking row in one transaction
— then unlocks. Replicas starting together queue on the lock and then find
nothing to do.

A file the live database already satisfies but `schema_migrations` does not
record is re-run, which is how the pre-migration schema is adopted: migrations
are `IF NOT EXISTS`-guarded, so the re-run is a no-op that only lands the
tracking row. Add schema changes as a new numbered file; never edit an applied
one.

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

Reads are open. Writes require `Authorization: Bearer <token>`, where the
token is either the static `ENCAPI_WRITE_TOKEN` or a Vault identity JWT from
`ENCAPI_OIDC_ISSUER`:

- aud `encapi` (`ENCAPI_OIDC_AUDIENCE`): full write access, like the static token.
- aud `encapi-testing` (`ENCAPI_OIDC_TESTING_AUDIENCE`): only
  `PUT /api/v1/nodes/{certname}/environment` with `{"environment":"testing"}`
  and `DELETE /api/v1/nodes/{certname}/environment/testing`; any other
  environment or write route returns 403.

An unreachable issuer returns 503 for JWTs; the static token keeps working.

```
GET    /healthz
GET    /api/v1/nodes                      list nodes
GET    /api/v1/nodes/{certname}           get node
PUT    /api/v1/nodes/{certname}           upsert node        (token)
PUT    /api/v1/nodes/{certname}/environment  set only the environment, {"environment":"…"}; 404 if unknown (token, or testing JWT for "testing")
DELETE /api/v1/nodes/{certname}/environment/testing  restore the environment held before testing; 409 if not in testing or none recorded (token or testing JWT)
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
| `ENCAPI_WRITE_TOKEN`   | *(unset)*     | static bearer token for writes |
| `ENCAPI_OIDC_ISSUER`   | *(unset)*     | OIDC issuer URL for JWT writes (e.g. `<vault>/v1/identity/oidc`); unset = JWTs disabled |
| `ENCAPI_OIDC_AUDIENCE` | `encapi`      | JWT audience granting full write access |
| `ENCAPI_OIDC_TESTING_AUDIENCE` | `encapi-testing` | JWT audience allowed only to set a node's environment to `testing` |
| `ENCAPI_DISTRO_API_URL`| *(unset)*     | optional kickstart/distro API for provisioning params |

With neither `ENCAPI_WRITE_TOKEN` nor `ENCAPI_OIDC_ISSUER` set, writes are refused (read-only).

## CLI

```bash
export ENCAPI_URL=https://encapi.example.com
export ENCAPI_WRITE_TOKEN=…            # static token or Vault identity JWT; only needed for writes

encapi-cli status set production
encapi-cli role set roles::infra::storage::minio --param 'replicas=4' --param 'epel="9"'
encapi-cli node set host2100.int.example.net --role roles::infra::storage::minio --env production
encapi-cli node set-env host2100.int.example.net testing
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
