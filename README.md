# Go Peerflix Server

Streaming torrent client with Web UI

## Installation and usage

In the main folder of the (cloned) repo:

```
$ go build
$ ./go-peerflix-server --help
Usage of ./go-peerflix-server:
  -listen-address string
        Address to listen on for HTTP requests (default "0.0.0.0:8080")
  -root-dir string
        Root directory of the application (default ".")
  -storage-dir string
        Where to store existing torrents and downloaded data (default "torrent")
  -upload
        Whether or not to upload data
```

## Planned features

 - live stats
 - nice overview page (maybe extract info using [guessit](https://github.com/guessit-io/guessit))
 - ...


## License

This project is released under the GPL version 3 or (at your discretion) any later version.
