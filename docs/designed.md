---
authors: Sean Hagen (sean.hagen@gmail.com)
state: draft
---


# RFD 78 - Workernator


## What

A simple, bare-bones worker manager, with a core library, GRPC API, and CLI
client.


## Why

To have a worker manager that can use cGroups & namespaces to manage compute
resources and namespaces for PID, networking, & mount isolation.


## Details

Right off the bat: this isn't meant to be a generic job-running service. If that
was what was needed, we could use something like Faktory to host a job
server. There are also cloud services that provide similar
functionality. Instead, the library is designed to know what jobs it's able to
run; including what arguments are required. The main reason for this is that
this way we get to take full advantage of the type system.

Other job-runner services are built to allow *any* kind of job arguments, which
means often relying on `interface{}/any` or JSON strings to provide data to a job
when it starts. This is not great; the conversion and manual type checking add
effort better spent elsewhere, and additionally adds a potential bug-prone
section to the code.

Also, if we were just going to send JSON-in-a-string &#x2013; why use GRPC? Let's take
advantage of all that type checking goodness.

As an additional side-benefit, this means that the API ( both the external GRPC
API and the code-library API ) should provide enough information about what jobs
are available and what arguments they take. This should help devs rely on their
IDE auto-completion and documentation tooling rather than a browser &#x2013; at least
that's the hope!

> One note before we dive in: don't treat the names of types, functions, or
> concepts in this document as "final". I tend to iterate & change names as I work
> as the "purpose" of a type or function becomes more clear as the code gets
> fleshed out.


### Library

The library is made up of two parts; the job manager, and the jobs.

The job manager is the workhorse; it is what launches jobs when a user makes a
request, keeps track of running jobs, and all the other functionality we
require.

There are a few things that will be required for the library: a way to **register**
a job, a way to **start** a job, a way to **stop** a job, a way to get the **status** of a
job, and lastly a way to **tail the output** of a job.

This is all done with the following types:

    type JobData struct {
      Name string
      Arguments []string
    }
    
    type Interactor interface {
      Log(string) // prints the string with timestamp & tag
      Result(any) // prints the result, with timestamp & tag
      Error(err) // prints out the error, with timestamp & tag
    }
    
    type JobFunction func(JobData,Interactor) error
    
    type Job struct {
      Name string
      Run  JobFunction
    }


#### Registering Jobs

The library will provide a function that can be used to register a job function:

    RegisterJob(job Job) error

The `Job` struct should contain the name of the job that will be used later when
starting a job; the `JobFunction` function is the "worker" ( cause it does the
actual work! ).


#### Starting Jobs

There are a few steps involved in launching a job.

The first step is running the same `workernator` binary, but with different
arguments. This is done automatically by `workernator`, don't worry! This is done
to set up the namespaces so that we've got some resource isolation.

When the binary runs, it is now in "namespaced" mode. The next step is setting
up the cgroups, and handling the rest of the setup ( mounting `proc`, pivoting the
root file system, etc ). The last part of this step is running the `workernator`
binary one last time.

Now the binary is ready to run the job worker function. All the arguments are
passed to the function via the command line arguments, but the job function
doesn't need to worry about that. Part of the code that wraps up the function
will take care of gathering up the command line arguments &#x2013; minus the first
three arguments. It does this because at this point the first three arguments
will be `/proc/self/exe`, `runner`, and the name set in the `Job` struct passed into
`RegisterJobber`.

The library keeps a hold of the `*exec.Cmd` it creates when starting the process
of launching a job so that it can be used to kill a job process later if
required. Additionally, the standard out & standard error fields on the
`*exec.Cmd` will be assigned objects that collect the log lines, add metadata,
then sends the line to a collector that puts all the lines into a single
in-memory buffer.

However, to code using the library to manage jobs, this is all hidden behind
this function:

    StartJob(args JobData) (*JobInfo,error)

Where the `JobInfo` struct that gets returned contains useful information such as
the ID of the job.

An error will be returned only if the data in `args` contains an invalid job, or
incorrect arguments for the job.


##### CGroups & Namespaces - Resource Control and Isolation

When starting a job, `workernator` does more than just launch a goroutine and call
it a day.

Using the namespaces & cgroups built into modern Linux kernels, we're able to
build something similar to a Docker container that the job runs inside. This is
accomplished using the methods detailed in [this series of articles](https://medium.com/@teddyking/linux-namespaces-850489d3ccf) and also in
[this article](https://www.infoq.com/articles/build-a-container-golang/).

Basically, this method boils down to using the special file `/proc/self/exe` which
is a special link that points to the currently running binary. By using
`exec.Command` from the [exec package](https://pkg.go.dev/os/exec) we can re-run the `workernator` binary with
special arguments that enable the creation of new namespaces. This is also what
allows us to configure cgroups so that we can limit the amount of available RAM
or CPU to a running job.


#### Stopping Jobs

Using the `exec.Cmd` pointer that was created in the process of starting a job, we
can use `exec.Cmd.Process.Kill()` to force the job to stop.

However, like the other library methods, the implementation details are hidden
from the world at large behind this function:

    StopJob(id string) (*JobInfo, error)

If the `id` contains the ID of a current or past job in `workernator`, it will
attempt to stop that job. If the ID doesn't map to such a job, the function will
return an error.

This function is idempotent, if `StopJob` is called with the ID of a job that has
already been stopped, the function will simply return the `JobInfo` pointer.


#### Querying Job Status

The library will provide the following function:

    JobStatus(id string) (*JobInfo, error)  

If the `id` parameter contains the ID of a current or past job, the function will
return the `JobInfo` for that job. Otherwise it will return an error.


#### Get Job Output

An important part of running a job is being able to get the output of the
job. Similar to being able to use the command line tool `tail`, the library
provides a method that streams the output of a running job to any client that
wishes to receive that output.

The library will provide a function that allows clients to get the output logs
of a running or completed job:

    TailJob(ctx context.Context, id string, output chan<- OutputLine) error

The provided `context.Context` is used for cancellation, as this function will
most likely be run as a goroutine while some other part of the code reads the
data from the `output` channel. This context should be one generated by
`context.WithCancel`, as you should use the `CancelFunc` returned from `WithCancel` as
soon as you no longer wish to receive data from the `output` channel.

If `id` doesn't contain the ID of a job that is currently running or has run in
the past, the function will return an error.

`TailJob` expects to be the one to close the `output` channel. If it is closed
elsewhere, `TailJob` *will* panic and throw an error.

`OutputLine` is a struct that contains each line of output from a job, with
additional metadata such as timestamps.

Once `TailJob` has read and sent all lines from a job, it closes the channel. This
means that as long as the job is running, the channel stays open.


##### Storing Job Output

As part of launching a job, we are able to set the `Stdout` and `Stderr` of a
`exec.Cmd` to any `io.Writer` of our choosing. This will be used to capture the
output of a job and store it in memory while the job is running.

For this challenge, that's where storing the output stops &#x2013; it'll just stay in
memory, and will be lost once the `workernator` binary is stopped.

For a real-world service, we'd have to look into flushing the output to a file
on disk once a job is complete. There would also have to be a way to keep that
output in-memory for a short period of time, to account for other clients
potentially asking for the same output log without ballooning the amount of
memory being used. While this does mean that job info is lost when the service
shuts down, doing anything more is out of scope for this exercise.


##### Concurrency

The library will support multiple clients requesting the output of a single job
at once. The hard part for getting the output logs concurrently would probably
actually be determining when to free the buffer used to store the output, rather
than the mechanism to allow multiple clients to read concurrently. This is
because the actual "read from a file" part would pretty much just feed data into
the same mechanism used by clients to get the output of a job while it's running.

Managing when to flush the in-memory buffer so that we're not creating bugs for
currently connected clients, and also doing so in a way that avoids deadlocks or
resource contention *feels* tricky. Then again, Go has made lots of concurrency
stuff I never thought I'd even understand pretty straightforward to use, so this
may be something where the scope changes drastically as actual code starts
getting written. However, as we're sticking with simple and small scope, the
library will simply keep all the output in memory for now. 


### API

GRPC API to start/stop/get status/stream output of a running process.
Use mTLS authentication and verify client certificate.
  Set up strong set of cipher suites for TLS and good crypto setup for certificates.
  Do not use any other authentication protocols on top of mTLS.
Use a simple authorization scheme.


#### GRPC API Definition

We're not going to go over the entire protobuf definition here, instead we'll
cover some of the design decisions so we're all on the same page. However,
please do check out [workernator.proto](file:///proto/workernator.proto) to see the entire protobuf definition.


##### Job Type

As part of the definition of a job, each job has a 'type'. This type defines
what the job does, as well as what arguments it expects.

In addition to the three pre-defined jobs ( "Fibonacci", "Expression Evaluator",
and "Wait Then Send" ), there is also a '0-th' job type: `Noop`. This is because
in Go, the default value for a variable with type `JobType` is 0. Rather than have
this be the value for an "actual" job, instead this is assigned to a job that
does nothing and doesn't print anything. This way, a configuration, programmer,
or simple clumsy-fingered mistake won't start the wrong job.


##### Job Request Messages

There are two potential messages that each of `Stop`, `Status`, and `Tail` could
have used:

-   a generic `Id` message that simply contained the job ID, OR
-   a method-specific message that contains the job ID

The first variation is a bit nicer; instead of three different message types
that contain the same data you just have one. So you'd get this:

    service Service {
      rpc Start(JobStartRequest) returns (Job){}
      rpc Stop(JobId) returns (Job){}
      rpc Status(JobId) returns (Job) {}
      rpc Tail(JobId) returns (stream TailJobResponse){}
    }

However, there is a somewhat large drawback to this. 

For example, what happens if we want to add a timeout field to the request we
send to `Stop`? Or if we want `Status` to additionally return all of the current log
lines for the job? Maybe we want to be able to have `Tail` only start from the
most recent message and then continue from there &#x2013; or to only show the last N
log lines.

Each of these would require one of two things. Either the `JobId` message gets
overloaded to the point of being nearly useless &#x2013; or each method gets its own
message type.

This is the route I chose, as I can see lots of potential functionality
requiring expanding each of the request messages for `Stop`, `Status`, and `Tail`.


##### The "Arguments" Message Type

Not a lot to say about this one, but just in case you were curious: this message
type is here so that there's no chance that the `args` field in the `Job` message
type and the `args` field in the `JobStartRequest` message type don't accidentally
diverge.


##### Separate Folders

This one is mostly a personal preference thing, but I prefer to keep the
protobuf definition files separate from the code generated from those
files. This is mostly so that if there's a need to generate code for other
languages that there's already a clear pattern as to how that should work and
where files should go.


#### Authentication

The GRPC service will use mTLS for authentication. A unique certificate will be
generated for each client.

The server and client libraries will be configured to use TLS v1.3, with only
these two ciphers:

-   `tls.TLS_CHACHA20_POLY1305_SHA256`
-   `tls.TLS_AES_128_GCM_SHA256`


##### NOTE: Clarification Required

Ask for more detail on what they mean by "good crypto setup for certificates".


#### Authorization

Rather than using JWT or something else to authorize users, instead we'll use
some of the features of TLS!

One of the things that can be configured while generating a TLS certificate is
the 'distinguished names', or subjects. These are things like country, state or
province, locality ( ie, city ) &#x2013; as well as organization & common name. These
values are usually used to verify that a TLS certificate is the right one for
the site you're navigating to; your browser checks the common name to see if it
matches the domain you're on.

However, we can use it for other things; things like authorization!

The client certificate that is generated will contain a few subjects with
slightly different meanings.

Below is each subject key, the 'proper' name, and what we're using it for ( if
we're using it differently than the name would suggest ).


##### Keys


###### Organization Name

**Key:** O

Using this basically as intended, putting 'Teleport' as the value.


###### Organizational Unit Name

**Key:** OU

I'm putting `workernator`, with the idea that this could be used to put the name
of the service the certificate is meant to be used with.


###### Common Name

**Key:** CN

Typically used for the name of the person "responsible" for the TLS certificate
on the server, we're using it to identify whether the certificate is meant to be
used by a server or a client. Handy for when things get mis-named!


###### Locality Name

**Key:** L

This is normally used to name the city or local region where the server or
server admin is located.

Here we're going to use it to identify the user making a request. This will be
used to look up what permissions and abilities the user has.


##### Usage

The **O**, **ON**, and **CN** keys are the "core" keys, and should be present regardless of
whether the certificate is meant to be used by a server or a client. Both
clients and servers will use those three keys when validating a certificate.

As for the **L** key, only the servers will pay attention and use that key. Clients
will ignore this key if it's in a server certificate.


### Command-Line Client

The client is going to be built using [cobra](https://cobra.dev/).

If called with no arguments, it will print out some basic information and some
usage hints:

    $ workernator 
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

From here on, the output of the command will be truncated for clarity and
comprehension.


#### Starting, Stopping, & Getting Job Status

All the sub-commands for managing jobs are available under 'jobs':

    --- truncated ---
    Usage:
      workernator jobs [command]
    
    Available Commands:
      start       Start a job in the server
      status      Get the status of a job
      stop        Stop a running job
      tail        View the output of a command
    --- truncated ---

The client knows what jobs can be run, and will list them when you call
`workernator jobs start` without any further arguments:

    --- truncated ---
    Available Commands:
      eval        Evaluate a mathmatical formula
      fib         Calculate the value of a position in the Fibonacci sequence
      noop        A job that does nothing
      wait        Wait for a set number of seconds before sending an empty HTTP POST request to a URL
    --- truncated ---  

Each job has it's own arguments, and `workernator` will let you know what's
required if you run `workernator jobs start <name>` without any further arguments,
or if you use `workernator jobs start <name> --help` to view the built-in help
docs.

Once you've filled out all the required arguments, if the job is started
successfully the ID of the newly created job will be printed out before the
command exits:

    $ workernator jobs start fib 3
    Contacting server...
    Starting job...
    
    Job started, ID is 'XE38YM'

This ID can then be used to get the status of a job:

    $ workernator jobs status XE38YM
    Contacting server...
    Getting info for job XE38YM...
    
    Job Status:
    ID: XE38YM
    Name: Fibonacci
    Arguments:
      - Number:   3
    Status:     Complete
    Started:    2022-07-07 16:34:03
    Finished:   2022-07-07 16:34:04
    Duration:   1 second

This is the same way stopping a job works:

    $ workernator jobs stop XE38YM
    Contacting server...
    Stopping job XE38YM...
    
    Done, job stopped.

As a note, if the job has already finished, the `stop` command will still report
the job is stopped &#x2013; no complaints about "job already completed" or
anything. The `stop` and `status` commands ( and the `tail` command ) will only return
an error if the ID given doesn't match the ID of a running or completed job.

Tailing output is also as simple as getting the status or stopping a job:

    $ workernator jobs tail XE38YM
    2022-07-07 12:34:54 [JOB] Starting job 'Fibonacci'
    2022-07-07 12:34:55 [FIB] Calculating the value of the 3rd value in the Fibonacci sequence
    2022-07-07 12:34:55 [FIB] Using lookup; value is '1'.
    2022-07-07 12:34:55 [RESULT] 1
    2022-07-07 12:34:56 [JOB] Complete
    
    Job finished, no more output, exiting tail!

As you can see, once a job has stopped `workernator` will exit.


### Security

