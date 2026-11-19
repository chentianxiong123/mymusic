module mymusic-mygo

go 1.27.3

tool github.com/egoist/mygo/cmd/mygo

require (
	github.com/ProtonMail/go-crypto v1.4.1 // indirect
	github.com/cloudflare/circl v1.6.2 // indirect
	github.com/dlclark/regexp2/v2 v2.11.0 // indirect
	github.com/ebitengine/purego v0.11.1 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	github.com/google/pprof v0.0.0-20230207041349-798e818bf904 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hajimehoshi/go-mp3 v0.3.4 // indirect
	github.com/huin/goupnp v1.3.0 // indirect
	github.com/klauspost/compress v1.18.6 // indirect
	github.com/ncruces/go-sqlite3 v0.35.6 // indirect
	github.com/ncruces/go-sqlite3-wasm/v6 v6.3.35304 // indirect
	github.com/ncruces/julianday v1.0.0 // indirect
	github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e // indirect
	github.com/tphakala/go-aac v0.7.1 // indirect
	github.com/tphakala/simd v1.11.0 // indirect
	github.com/ulikunitz/xz v0.5.15 // indirect
	go.digitalxero.dev/rpm v0.3.4 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/image v0.46.0 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

require (
	github.com/egoist/mygo v0.9.0
	github.com/grafana/sobek v0.0.0-20261116133901-d070e0f1f200
	github.com/lsongdev/miservice-go v0.0.0-20260317060819-b3e39634cd06
	homecast v0.0.0-00010101000000-000000000000
)

replace homecast => ../go

replace github.com/egoist/mygo => github.com/egoist/mygo v0.9.1-0.20261115172007-ea96ba60e107
