# borg-collective

> [![build status](https://github.com/vemilyus/borg-collective/actions/workflows/build.yml/badge.svg)][actions-url]
> [![Latest Release](https://img.shields.io/github/v/release/vemilyus/borg-collective)][release-url]

[actions-url]: https://github.com/vemilyus/borg-collective/actions

[release-url]: https://github.com/vemilyus/borg-collective/releases/latest

Helps to setup and control as many Borg backups as possible with minimal configuration.

## Features/Goals

- Automatically configuring backups for Docker containers using labels
- Connecting directly to Docker daemon to get the required information
- Enabling secure configuration and retrieval of encryption credentials for Borg
- Running as non-privileged user with necessary capabilities configured for Borg

## Non-Goals

- Running inside a container

## Docs

[Go here](./docs/index.md)

## Tools

### [`borg-collective`](borg-collective)

Configures any backups as detected or specified in configuration files on an individual host.

### [`credstore`](./credentials/cmd/store)

Securely manages and provides secure values over the network.

### [`cred`](./credentials/cmd/cli)

Interacts with `credstore` on the individual hosts. Intended for use by `borg-collective` to retrieve
encryption keys for borg backups as needed.

`cred` and `credstore` are tightly coupled and communicate via gRPC to keep network overhead
to a minimum. And also because any HTTP based API would end up looking like RPC anyway.

## Target platforms

- linux/amd64
- linux/arm64
- linux/armv7l
