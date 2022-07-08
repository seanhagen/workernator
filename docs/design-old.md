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

#### Running Jobs


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
2022-07-07 12:34:54 [JOB] Starting job 'Fibonacci'
2022-07-07 12:34:55 [FIB] Calculating the value of the 3rd value in the Fibonacci sequence
2022-07-07 12:34:55 [FIB] Using lookup; value is '1'.
2022-07-07 12:34:55 [RESULT] 1
2022-07-07 12:34:56 [JOB] Complete

Job finished, no more output, exiting tail!

```

The tail command will exit after outputting all the lines if the job has
finished.

### Security 

 - TLS setup (version, cipher suites, etc.)


