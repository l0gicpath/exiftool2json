# Usage

```shell
$ go build
$ ./exiftool2json --help
```

Running `./exiftool2json` will default to binding on localhost, port 3000 and will expect exiftool to be in 
your PATH.

## Testing

To test out the service
```
curl -o - http://localhost:3000/tags | python3 -mjson.tool
```

Hitting root URL `/` will issue a redirect, which you can ask cURL to follow by adding `-L` flag

```
curl -L -o - http://localhost:3000 | python3 -mjson.tool
```

# Implementation Details

Since the idea is to stream arbitrary sized XML converted into JSON to a client, we need to chunk the
JSON output to have a valid stream of JSON objects. This means we need to chunk the corresponding XML
structures, the way to do this using Golang's `encoding/xml` is by tapping into the XML token stream ourselves.

We start a synched io pipe using Golang's `io.Pipe` and wire the command (the XML data generator) Stdout to
the write end of that pipe, while the read end of the pipe goes to `xml.Decoder`.

The command is started in its own Goroutine which we spawn ourselves, that's to avoid any unexpected
behaviour from simply offloading that to Go's `Command.Start`. In either case, both offload to `os.Process`
but we can guarantee a synched io state by blocking in a Goroutine waiting for Run to finish.

The pipe only ever writes more as more is read from the other end. So the request's main task is to read
the XML token stream, transform it to JSON, flush it to the client and repeat until we reach the end of
the XML data structure we're reading `</taginfo>`. Then we break the loop.

Ideally we should check for the StartElement of that `taginfo` but I feel that's unnecessary. We know the
data shape beforehand and we know taginfo encloses our tables and it's the tables and their nested tag structures
that we're interested in. As soon as we spot one, we start transforming and flushing to the client.