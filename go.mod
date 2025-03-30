module github.com/hwcer/updater

go 1.23

toolchain go1.23.4

replace github.com/hwcer/cosgo v1.1.0 => ../cosgo

require (
	github.com/hwcer/cosgo v1.1.0
	go.mongodb.org/mongo-driver v1.17.1
)

require (
<<<<<<< Updated upstream
	github.com/golang/snappy v0.0.4 // indirect
	github.com/klauspost/compress v1.17.11 // indirect
=======
	github.com/golang/snappy v1.0.0 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
>>>>>>> Stashed changes
	github.com/montanaflynn/stats v0.7.1 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.1.2 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/youmark/pkcs8 v0.0.0-20240726163527-a2c0da244d78 // indirect
<<<<<<< Updated upstream
	golang.org/x/crypto v0.29.0 // indirect
	golang.org/x/sync v0.9.0 // indirect
	golang.org/x/text v0.20.0 // indirect
=======
	golang.org/x/crypto v0.36.0 // indirect
	golang.org/x/net v0.21.0 // indirect
	golang.org/x/sync v0.12.0 // indirect
	golang.org/x/sys v0.31.0 // indirect
	golang.org/x/text v0.23.0 // indirect
>>>>>>> Stashed changes
)
