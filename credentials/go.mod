module github.com/vemilyus/borg-collective/credentials

go 1.24.1

// 99designs forked ages ago, keybase's is up-to-date
replace github.com/99designs/go-keychain => github.com/keybase/go-keychain v0.0.1

// currently used version is really out of date
replace github.com/godbus/dbus => github.com/godbus/dbus/v5 v5.1.0

require (
	filippo.io/age v1.2.1
	github.com/99designs/keyring v1.2.2
	github.com/awnumar/memguard v0.22.5
	github.com/google/uuid v1.6.0
	github.com/grpc-ecosystem/go-grpc-middleware/v2 v2.3.1
	github.com/integrii/flaggy v1.5.2
	github.com/pelletier/go-toml/v2 v2.2.4
	github.com/pkg/errors v0.9.1
	github.com/rs/zerolog v1.34.0
	github.com/stretchr/testify v1.10.0
	golang.org/x/crypto v0.37.0
	golang.org/x/term v0.31.0
	google.golang.org/grpc v1.72.0
	google.golang.org/protobuf v1.36.6
)

require (
	github.com/99designs/go-keychain v0.0.0 // indirect
	github.com/awnumar/memcall v0.4.0 // indirect
	github.com/danieljoos/wincred v1.2.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dvsekhvalnov/jose2go v1.8.0 // indirect
	github.com/godbus/dbus v4.1.0+incompatible // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/gsterjov/go-libsecret v0.0.0-20161001094733-a6f4afe4910c // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mtibben/percent v0.2.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	golang.org/x/net v0.39.0 // indirect
	golang.org/x/sys v0.32.0 // indirect
	golang.org/x/text v0.24.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250425173222-7b384671a197 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
