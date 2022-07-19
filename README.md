# Workernator

Very very simple worker manager built in Go.

Runs arbitrary Linux processes as jobs, where the output of the job is captured
and saved to a file. This output can be streamed to clients as binary data,
allowing jobs to output whatever they want as output.

## Setup 

First step: get you some [asdf](https://asdf-vm.com/), it's what we'll be using
to install some tools at specific versions. You will need the following plugins,
each of which you can install with `asdf plugin add <name>`:

 - mage
 - golangci-lint
 - direnv
 - upx

What are each of these being used for?

 - [mage](https://magefile.org/) is what we'll be using instead of a Makefile or
   shell scripts.
 - [golangci-lint](https://golangci-lint.run/) is the linter we all know & love
 - [direnv](https://direnv.net/) is potentially going to get used to store some environment variables.
 - [upx](https://upx.github.io/) is a handy tool for shrinking binary sizes

## Development

### GRPC

The GRPC protobuf definitions are kept in [proto](./proto). If these are
updated, run `mage grpc` to rebuild the auto-generated code.

### Testing

You should be able to just use `go test ./...` to run the test suite. 
