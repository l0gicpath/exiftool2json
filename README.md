# Implementation thoughts

Parsing facilities available in Go treat data as one lump, the idea I wanted was to begin streaming midst 
the XML parsing.
In theory, this can be achived using Golang decoders except the stream can be broken at any point
which will yield incomplete results decoders will simply parse as-much-up-to the current memory buffer it's reading
from.

The second issue is decoding from one format, piping that into another encoder and streaming the final result.

To solve this, I decided to use the decoder tokenizer to delimite my own XML knowing where my boundaries are. I use
Golang's io.Pipe, wire the pipe input (writer) to the command Stdout, and wire the pipe output (reader) into
the xml decoder.
Traverse the tokens, looking for my starting elements in that case, a `table` element. Then parse that one
particular table element, converting it into a series of `tags` and streaming them one at a time as they come in.

This is done by linking the ResponseWriter to the json encoder. As we encode, we stream.

It's not pretty, but it's memory footprint is negliable compared to other solutions when I stress tested this
and the receiving client immediately starts receiving data as soon as we parse the first xml element.