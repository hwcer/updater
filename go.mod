module github.com/hwcer/updater

go 1.18

replace (
	github.com/hwcer/cosgo v0.0.2 => ../cosgo
	github.com/hwcer/logger v0.0.2 => ../logger
)

require (
	github.com/hwcer/cosgo v0.0.2
	github.com/hwcer/logger v0.0.2
	go.mongodb.org/mongo-driver v1.11.4
)

require (
	github.com/golang/snappy v0.0.4 // indirect
	github.com/klauspost/compress v1.16.4 // indirect
	github.com/montanaflynn/stats v0.7.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.1.2 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/youmark/pkcs8 v0.0.0-20201027041543-1326539a0a0a // indirect
	golang.org/x/crypto v0.8.0 // indirect
	golang.org/x/sync v0.1.0 // indirect
	golang.org/x/text v0.9.0 // indirect
)
