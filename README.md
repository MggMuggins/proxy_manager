# `proxy_manager`
A simple tool for managing a bunch of daemons to start proxies. A simple proxy
file format defines which local ports are mapped to which remotes (and ports),
and the tool will ensure that a daemon is running for each proxy mapping.

## Usage
```sh
# Reads standard tcp mappings from 'proxies.list'
proxy_manager
# Use -h to see more options
proxy_manager -h
```

### File format
See `proxies.list` in this repo for an example
```
# Comments start with a '#'

# Repeat this line for different hosts and ports
<local_port>:<remote>:<remote_port>  # Trailing comments are supported
```
`<remote>` can be an IP address or dns name.

## Details
Standard tcp mappings are managed using `socat(1)`. Encrypted tunnels use ssh.
Note that ssh tunnels should be authenticated using keys, and require that the
keys are stored unencrypted OR are present in the `ssh-agent`.

