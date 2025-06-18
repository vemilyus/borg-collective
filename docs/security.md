# Borg Collective Security

## Introduction

## credstore

The `credstore` process and underlying system are generally regarded as untrustworthy.

Any credentials managed by `credstore` are protected from direct tampering by 
being encrypted at rest and their metadata is verified using an HMAC. Any attempt
to tamper with metadata or values will lead to a failure to read those values.

Once unlocked the vault retains the master passphrase in hardened memory, which is
guarded against extraction and manipulation by a Boojum. See the following references:

- [encrypting secrets in memory](https://spacetime.dev/encrypting-secrets-in-memory)
- [memory retention attacks](https://spacetime.dev/memory-retention-attacks)

Before the vault is unlocked the data encryption prevents all forms of reading the
encrypted credentials, short of breaking the encryption ([age][age-encryption]) itself.

[age-encryption]: https://github.com/FiloSottile/age

Age is utilized to encrypt the data at-rest using an identity that is encrypted using
the master passphrase. To prevent leakage that identity is only loaded and decrypted
as needed for concrete access actions.

Sensitive data in transit (e.g. credentials in RPC calls) is always encrypted in prod
mode and afterward explicitly zeroed out where possible.

### Covered scenarios

- Unauthorized memory access
  - vault data is not kept in volatile memory and core credentials are kept in hardened
    memory protected via Boojum and other measures provided by the kernel
- Reboot attacks to exploit vulnerable components
  - all secrets are encrypted at-rest and vault must be unlocked using passphrase before
    any values can be retrieved via `cred`
- Execution of `cred` on compromised host using compromised credentials
  - `credstore` doesn't allow access via `cred` from the same host in prod mode
- Using credentials of compromised client to access all other secrets
  - secrets record the remote IP address of first access and can then 
    only be accessed from that host
- Sniffing of network packets
  - TLS is required in prod mode
- Extraction of new secrets after specifying malicious recovery key
  - The recovery key is verified using an HMAC before each use

## cred

## borgd

The system `borgd` runs on is generally regarded as trusted, just like the 
Borg client process. Should that system become compromised the configured 
repository should also be considered compromised.

`borgd` doesn't store any credentials, unless otherwise configured. For the 
recommended scenario where credentials are retrieved using `cred` refer to
the previous section. Otherwise, credentials may be provided using the config
file.

Please see [Borg Security][borg-security] as `borgd` delegates many critical 
operations to Borg.

[borg-security]: https://borgbackup.readthedocs.io/en/stable/internals/security.html
