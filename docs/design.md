---
authors: Sean Hagen (sean.hagen@gmail.com)
state: draft
---

# RFD 78 - Workernator

## What 

A simple, bare-bones worker manager, with a core library, GRPC API, and CLI client.

## Why 

To have a worker manager that can use cGroups & namespaces to manage compute
resources and namespaces for PID, networking, & mount isolation.

## Details 

### Library

    Worker library with methods to start/stop/query status and get the output of a job.
    Library should be able to stream the output of a running job.
        Output should be from start of process execution.
        Multiple concurrent clients should be supported.
    Add resource control for CPU, Memory and Disk IO per job using cgroups.
    Add resource isolation for using PID, mount, and networking namespaces.

Right off the bat: this isn't meant to be a generic job-running library. If that
was what was needed, we could use something like Faktory to host a job
server. There are also cloud services that provide similar
functionality. Instead, the library is designed to know what jobs it's able to
run; including what arguments are required. The main reason for this is that
this way we get to take full advantage of the type system.

Other job-runner services ( such as Faktory ) are built to allow *any* kind of
job to get run they have to rely on things like `interface{}/any` or JSON
strings to provide data to a job when it starts. This is not great; the
conversion and manual type checking add effort better spent elsewhere, and
additionally adds a potential bug-prone section to the code.

Also, if we were just going to send JSON-in-a-string then why use GRPC?

As an additional side-benefit, this means that the API ( both the external GRPC
API and the code-library API ) should provide enough information about what jobs
are available and what arguments they take. This should help devs rely on their
IDE auto-completion and documentation tooling rather than a browser -- at least
that's the hope!

#### Running Jobs

The system will take care of setting up namespaces & cgroups for each job. It
uses these to give each job it's own container to run in. This allows the system
to isolate a running job from the rest of the system it runs on, but also allows
workernator to control how much CPU & memory a job has while running.

This is accomplished by calling `/proc/self/exe` in the fashion detailed [in
this stand-alone article](https://www.infoq.com/articles/build-a-container-golang/) and
[this series of
articles](https://medium.com/@teddyking/linux-namespaces-850489d3ccf). 

The way this works is a multi-step process:

 - when the library is asked to start a job, it creates an `*exec.Cmd` that has
   all the settings configured so that the command uses new namespaces for
   everything
 - this command runs `/proc/self/exe ns` with additional arguments to control
   which job gets launched
 - this sub-command sets up the rest of the container, it sets a hostname, sets
   up cgroups, and handles `proc` and pivoting the root -- then runs
   `/proc/self/exe job` to launch the actual job
 - finally, at this point the job will actually run

Additionally, when setting up the initial `*exec.Cmd` it does the following, and
saves each piece of data into a `Job` object:
  * generate a unique alpha-numeric ID for the job
  * sets STDOUT and STDERR to be `io.Writer`'s that write each line to a buffer
    that can be viewed using the 'tail' functionality
  * captures the PID of the process
  * stores the arguments 
  * captures the timestamp of when the job started

The unique alpha-numeric ID is used when stopping a job, getting the status of a
job, or tailing the output logs of a job. The PID is stored as a backup measure;
if a job fails to stop, or is somehow left hanging when the manager is stopped
the PID can be used to properly kill the process manually.

#### Job Output

The library will be responsible for managing the output of each job that is
started. While a job is still "alive", the output will be stored in an in-memory
buffer. Once the job has stopped (regardless of whether it completed
successfully, it was stopped, or it died due to an error), the data in that
in-memory buffer will get flushed to a file on disk. The file will be encoded
using a simple text format such as JSON or CSV. 

However, none of this is exposed outside of the library; the library will only
expose a method something like this:

```go
ReadJobLog(ctx context.Context, id string, output chan OutputLine)
```

Where `OutputLine` is the following type:

```go
type OutputLine struct {
  Line string
  Timestamp time.Time
}
```

The `ReadJobLog` method checks first to see if the ID it was given matches a
currently running job. If it finds one, it copies each `OutputLine` from the
in-memory buffer and sends it to the `output` channel.

If there is no running job with the given ID, it checks to see if there is a
finished job with that ID. If there is, it opens the file and parses each line
into a `OutputLine` before sending it to the `output` channel.

If there is also no finished job with the given ID, the method will return
simply return. The library will have other functions that can be used to check
if a given job ID is valid, and should be used before calling `ReadJobLog` to
validate the ID.

If there *are* logs to be sent, `ReadJobLog` will send each output line in
order. Once it has finished, it will close the channel.

#### Stopping Jobs

As part of starting a job, the library will keep the `*exec.Cmd` pointer around
in a lookup table. The library will remove the pointer from the lookup table
once the job stops running ( whether it completes successfully or not ).

If a user makes a request to stop a job, the library will use the lookup table
to see if it's a job it knows about. If it is, it will use
`cmd.ProcessState.Exited()` to see if the job is still running. If it is, it'll
use `cmd.Process.Kill()` to kill the job.

#### Tailing & Concurrent Clients

For all the other currently defined routes, the communication is a more
traditional request & response. The 'tail' route is different, as it streams the
output back to the client -- including output lines from before the client first
connected.

The caller of `ReadJobLog` can use the `context.Context` it passes in to cancel the
reading of job logs, such as when a GRPC API client disconnects.

In the GRPC service, we should end up with something similar to this:

```go
func (g *grpcService) Tail(strm pb.Tail_WorkerServer) (*pb.TailJobResponse, error ) {
	ctx := strm.Context()
	if err := g.manager.JobExists(ctx, req.GetId()); err != nil {
		// if the Id doesn't match any current or historical job, return an error
		return nil, err
	}

	out := make(chan lib.OutputLine)

	go g.manager.ReadJobLog(ctx, req.GetId(), out)

	for line := <-out {
		strm.Send(convertLineToPB(line))
	}

	strm.Close()
}
```


### API

    GRPC API to start/stop/get status/stream output of a running process.
    Use mTLS authentication and verify client certificate. Set up strong set of cipher suites for TLS and good crypto setup for certificates. Do not use any other authentication protocols on top of mTLS.
    Use a simple authorization scheme.



I'm not going to go over the entire protobuf definition here, rather let's go
over some of the design choices. If you want to follow along, you can check out
the [workernator.proto](/proto/workernator.proto) file where all this is
defined.

#### Job Type

As part of the definition of a job, each job has a 'type'. This type defines 
what the job does, as well as what arguments it expects. 

In addition to the three pre-defined jobs ( "Fibonacci", "Expression Evaluator",
and "Wait Then Send" ), there is also a '0-th' job type: `Noop`. This is
because in Go, the default value for a variable with type `JobType` is 0. Rather
than have this be the value for an "actual" job, instead this is assigned to a
job that does nothing and doesn't print anything. This way, a configuration,
programmer, or simple clumsy-fingered mistake won't start the wrong job.

#### Job Request Messages

There are two potential messages that each of `Stop`, `Status`, and `Tail` could
have used:

 - a generic `Id` message that simply contained the job ID, OR
 - a method-specific message that contains the job ID
 
The first variation is a bit nicer; instead of three different message types
that contain the same data you just have one. So you'd get this:

```
service Service {
  rpc Start(JobStartRequest) returns (Job){}
  rpc Stop(JobId) returns (Job){}
  rpc Status(JobId) returns (Job) {}
  rpc Tail(JobId) returns (stream TailJobResponse){}
}

```
 
However, there is a somewhat large drawback to this. 

For example, what happens if we want to add a timeout field to the request we
send to `Stop`? Or if we want `Status` to additionally return all of the current
log lines for the job? Maybe we want to be able to have `Tail` only start from
the most recent message and then continue from there -- or to only show the last
N log lines.

Each of these would require one of two things. Either the `JobId` message gets
overloaded to the point of being nearly useless -- or each method gets its own
message type. 

This is the route I chose, as I can see lots of potential functionality
requiring expanding each of the request messages for `Stop`, `Status`, and
`Tail`.

#### The Arguments Message Type

Not a lot to say about this one, but just in case you were curious: this message
type is here so that there's no chance that the `args` field in the `Job`
message type and the `args` field in the `JobStartRequest` message type don't
accidentally diverge.

#### Separate Folders for .proto & Generated Code

This one is mostly a personal preference thing, but I prefer to keep the
protobuf definition files separate from the code generated from those
files. This is mostly so that if there's a need to generate code for other
languages that there's already a clear pattern as to how that should work.

### CLI Client 

Calling the CLI client without any arguments will print out a help message that
describes the basics of how to use the client.

```
# workernator 
Workernator is a job-runner library, server, and CLI client used for
long-running tasks you don't want to run as part of your core service.

This is the CLI client application, which allows you to start jobs,
stop jobs, get the status of jobs, and tail the output of any job.

Usage:
  workernator [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  jobs        A brief description of your command

Flags:
  -h, --help     help for workernator
  -t, --toggle   Help message for toggle

Use "workernator [command] --help" for more information about a command.
```

#### Flags VS Config

There are a few options that have to be set for pretty much any invocation of
the 'jobs' sub-commands.

As of right now, the values that have to be set:

 - the host of the server
 - the path to the mTLS certificate for authentication

These are set via flags on the 'jobs' command and all it's sub-commands.

#### Starting, Stopping, & Getting Job Status

Starting a job is fairly straightforward; you give the name of the job, as well
as any arguments to the CLI client. 

The base command for jobs will list out the available job commands:

```
# workernator jobs
This sub-command provides the ability to manage jobs, including
starting, stopping, getting the status, and viewing the output.

Usage:
  workernator jobs [command]

Available Commands:
  start       Start a job in the server
  status      Get the status of a job
  stop        Stop a running job
  tail        View the output of a command

Flags:
  -h, --help   help for jobs

Use "workernator jobs [command] --help" for more information about a command.
```

Calling `workernator jobs start` will print out the jobs that can be started:

```
# workernator jobs start
-- truncated ---
Available Commands:
  eval        Evaluate a mathmatical formula
  fib         Calculate the value of a position in the Fibonacci sequence
  noop        A job that does nothing
  wait        Wait for a set number of seconds before sending an empty HTTP POST request to a URL
-- truncated ---
```

To start a job, call `workernator jobs start <name>`, where `<name>` is one of
the sub-commands listed. Each job has it's own set of arguments; the help for
each one will describe what's required. For example, starting the Fibonacci job
looks like this:

```
# workernator jobs start fib 3
Started 'Fibonacci' job, ID: XE38YM
Use 'workernator jobs tail XE38YM' to see the output of this job,
'workernator jobs stop XE38YM' to stop the job.
```

Failing to provide the correct arguments will cause the client to return an
error:

```
# workernator jobs start fib
Error: accepts 1 arg(s), received 0
Usage:
  workernator jobs start fib position [flags]

Flags:
  -h, --help   help for fib

Error: accepts 1 arg(s), received 0
exit status 1

```

Getting the status of a job is a bit easier, all you need is the ID of the job:

```
# workernator jobs status XE38YM
Getting info for job XE38YM... DONE!

Job XE38YM: Fibonacci
Arguments:
  Number:   3
Status:     Complete
Started:    2022-07-07 16:34:03
Finished:   2022-07-07 16:34:04
Duration:   1 second
```

And tailing the output is similarly easy:

```
# workernator jobs tail XE38YM
[JOB] Starting job 'Fibonacci'
[FIB] Calculating the value of the 3rd value in the Fibonacci sequence
[FIB] Using lookup; value is '1'.
[RESULT] 1
[JOB] Complete

Job finished, no more output, exiting tail!

```

The tail command will exit after outputting all the lines if the job has
finished.

### Security 

 - TLS setup (version, cipher suites, etc.)


