# Workernator

Very very simple worker manager built in Go.

Runs arbitrary Linux processes as jobs, where the output of the job is captured
and saved to a file. This output can be streamed to clients as binary data,
allowing jobs to output whatever they want as output.

## Setup 

First step: get you some [asdf](https://asdf-vm.com/), it's what we'll be using
to pin some tooling versions. You will need the following plugins, each of which
you can install with `asdf plugin add <name>`:

 - mage
 - golangci-lint
 - grpcurl
 - direnv
 - upx

What are each of these being used for?

 - [Mage](https://magefile.org/) is what we'll be using instead of a Makefile or
   shell scripts.
 - [golangci-lint](https://golangci-lint.run/) is the linter we all know & love
 - [grpcurl](https://github.com/fullstorydev/grpcurl) gives us `curl` for GRPC,
   useful for testing
 - [Direnv](https://direnv.net/) is used to store some environment variables.
 - [upx](https://upx.github.io/) is a handy tool for shrinking binary sizes

## Development

### GRPC

The GRPC protobuf definitions are kept in [proto](./proto). If these are
updated, run `mage grpc` to rebuild the auto-generated code.

### Testing

You should be able to just use `go test ./...` to run the test suite. However,
because there are some paths that have to be correct for the tests to run, I
recommend you run `go test ./...` from the root of the repository.
