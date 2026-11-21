# Cutover: Cobbler ENC → encapi

Today Cobbler is Puppet's ENC. **Two** consumers hit
`http://cobbler.int.example.net/cblr/svc/op/puppet/hostname/<host>`:

1. **The exec ENC** — `puppet-prod` sets, in
   `site/profiles/manifests/puppet/server.pp`:
   ```
   node_terminus  = exec
   external_nodes = /opt/cobbler-enc/cobbler-enc
   ```
   `/opt/cobbler-enc/cobbler-enc` (templated from
   `site/profiles/templates/puppet/server/cobbler-enc.erb`) fetches Cobbler and
   reshapes it (list-form classes, drops `environment` when `testing`, adds
   `enc_role`/`enc_env`).

2. **The `enc_role`/`enc_env` facts** — `modules/libs/lib/facter/enc_direct_facts.rb`,
   a fact on *every* node, fetches the same Cobbler endpoint (cached 7d at
   `/var/cache/puppet_enc.yaml`) and derives `enc_role = classes.keys.first`,
   `enc_env = environment`. Those facts drive `enc_role_tier1/2/3` +
   `enc_role_path`, i.e. the whole Hiera hierarchy.

encapi serves drop-in replacements for **both**.

## 1. Seed encapi from the current estate

Create the statuses, roles, and nodes derived from each host's Cobbler ENC
through the write API (`PUT /api/v1/{statuses,roles,nodes}/…`) or the
Terraform provider.

## 2. Repoint the exec ENC (`profiles::puppet::cobbler_enc`)

[`encapic`](https://github.com/IUCLabs/encapic) is the ENC client on every
puppet master:

```
node_terminus  = exec
external_nodes = /usr/bin/encapic-enc
```

`encapic <certname>` fetches the cobbler-wire document from encapi and
reshapes it exactly as `cobbler-enc` did.

## 3. Repoint the `enc_direct_facts.rb` fact

Change only the base URL in `enc_direct_facts.rb`:

```ruby
uri = URI("https://encapi.example.com/cblr/svc/op/puppet/hostname/#{...}")
```

encapi's `/cblr/svc/op/puppet/hostname/<host>` returns the cobbler-wire shape
(`classes` as a map, `environment` always present), so `classes.keys.first` and
`environment` still resolve. No other fact logic changes.

## 4. Verify before flipping

For a sample of hosts, diff the old and new output:

```bash
diff <(curl -s "http://cobbler.int.example.net/cblr/svc/op/puppet/hostname/$H") \
     <(curl -s "https://encapi.example.com/cblr/svc/op/puppet/hostname/$H")

diff <(/opt/cobbler-enc/cobbler-enc "$H") \
     <(encapic "$H")
```

The `classes`/`environment`/`enc_role`/`enc_env` fields must match. (Cobbler's
provisioning params — `epel`, `tree`, `from_cobbler`, `operatingsystemrelease` —
are intentionally dropped: they are unused by the Puppet manifests. If a future
need arises, wire `ENCAPI_DISTRO_API_URL` to the kickstart-replacement API and
they reappear under `parameters`.)

## 5. Decommission

Once masters and agents are repointed and a Puppet run is clean, retire
`profiles::puppet::cobbler_enc` and Cobbler's ENC role. Cobbler can keep doing
provisioning/kickstart; only its ENC duty moves to encapi.
