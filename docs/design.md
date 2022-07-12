---
authors: Sean Hagen (sean.hagen@gmail.com)
state: draft
---


# RFD 78 - Workernator


## What

A simple, bare-bones worker manager, with a core library, GRPC API, and CLI client.


## Why

To have a worker manager that can use cGroups & namespaces to manage compute resources and namespaces for PID, networking, & mount isolation.


## Details

Right off the bat: this isn't meant to be a generic job-running service. If that was what was needed, we could use something like Faktory to host a job server. There are also cloud services that provide similar functionality. Instead, the library is designed to know what jobs it's able to run; including what arguments are required. The main reason for this is that this way we get to take full advantage of Go's static typing so that we can catch certain kinds of errors or bugs at compile time, instead of runtime!

Other job-runner services are built to allow *any* kind of job arguments, which means often relying on `interface{}/any` or JSON strings to provide data to a job when it starts. This is not great; the conversion and manual type checking add effort better spent elsewhere, and additionally adds a potential bug-prone section to the code.

Also, if we were just going to send JSON-in-a-string &#x2013; why use GRPC? Let's take advantage of all that type checking goodness.

As an additional side-benefit, this means that the API ( both the external GRPC API and the code-library API ) should provide enough information about what jobs are available and what arguments they take. This should help devs rely on their IDE auto-completion and documentation tooling rather than a browser &#x2013; at least that's the hope!

This service will be comprised of three parts:

-   a library that contains all of the job-management stuff
-   a GRPC API service that is basically a thin wrapper around the library
-   a GRPC API command-line client that can be used to start, stop, query status, and tail the output of jobs

Also, as this is not meant to be a real-world system (&#x2026;yet?), there are some things that won't be implemented. These kinds of things will get pointed out where relevant, but will mostly have to do with pointing out features that won't be implemented because of scope.

The list of requirements used to create this design doc can be found [here](https://github.com/gravitational/careers/blob/main/challenges/systems/challenge.md#level-5).


### Library

The library is made up of two parts; the job manager, and the jobs.

The job manager is the workhorse; it is what launches jobs when a user makes a request, keeps track of running jobs, stops jobs when a user requests, and keeps track of the output so users can request that as well.

So, to break down the core bits of functionality, we're going to need:

-   a way to **register** a job
-   a way to **start** a job
-   a way to **stop** a job
-   a way to get the **status** of a job
-   a way to **tail the output** of a job

This will be accomplished using some types that will probably look something like this:

```go
// Interactor allows a job to create output logs, log the results of a job, or report an error
// in the format expected by the job manager so it can parse the logs properly.
type Interactor interface {
  Log(string) // prints the string with timestamp & tag
  Result(any) // prints the result, with timestamp & tag
  Error(err) // prints out the error, with timestamp & tag
}

// JobData contains 
type JobData struct {
  Arguments []string
}

type JobFunction func(JobData,Interactor) error

type Job struct {
  Name string
  Run  JobFunction
}
```


#### Registering Jobs

The library will provide a function that can be used to register a job function:

```go
RegisterJob(job Job) error
```

The `Job` struct should contain the name of the job that will be used later when starting a job; the `JobFunction` function is the actual "worker".


#### Starting Jobs

Another function will be provided so users can start a job and provide any required arguments:

```go
StartJob(name string, args JobData) (JobInfo,error)
```

The `JobData` type will most likely be a simple-data-object struct that contains the arguments for the job. The `JobInfo` struct that gets returned will have some information about the job such as the ID that's required to stop the job, get it's status, or tail the output. An error will be returned only if the data in `args` contains an invalid job, or incorrect arguments for the job.


##### CGroups & Namespaces - Resource Control and Isolation

When starting a job, `workernator` does more than just launch a goroutine and call it a day.

Using the namespaces & cgroups built into modern Linux kernels, we're able to build something similar to a Docker container that the job runs inside. This is accomplished using the methods detailed in [this series of articles](https://medium.com/@teddyking/linux-namespaces-850489d3ccf) and also in [this article](https://www.infoq.com/articles/build-a-container-golang/).

Basically, this method boils down to using the special file `/proc/self/exe` which is a special link that points to the currently running binary. By using `exec.Command` from the [exec package](https://pkg.go.dev/os/exec) we can re-run the `workernator` binary with special arguments that tell the OS to run the binary in a new namespace for resource isolation. This also allows us to configure cgroups for resource management.


#### Stopping Jobs

Using the `exec.Cmd` pointer that was created in the process of starting a job, we can use `exec.Cmd.Process.Kill()` to force the job to stop.

However, like the other library methods, the implementation details are hidden from the world at large behind this function:

```go
StopJob(id string) (*JobInfo, error)
```

If the `id` contains the ID of a current or past job in `workernator`, it will attempt to stop that job. If the ID doesn't map to such a job, the function will return an error.

This function is idempotent, if `StopJob` is called with the ID of a job that has already been stopped, the function will simply return the `JobInfo` pointer.


#### Querying Job Status

The library will provide the following function:

```go
JobStatus(id string) (*JobInfo, error)  
```

If the `id` parameter contains the ID of a current or past job, the function will return the `JobInfo` for that job. Otherwise it will return an error.


#### Get Job Output

An important part of running a job is being able to get the output of the job. Similar to being able to use the command line tool `tail`, the library provides a method that streams the output of a running job to any client that wishes to receive that output.

The library will provide a function that allows clients to get the output logs of a running or completed job:

```go
TailJob(ctx context.Context, id string, output chan<- OutputLine) error
```

The provided `context.Context` is used for cancellation, as this function will most likely be run as a goroutine while some other part of the code reads the data from the `output` channel. This context should be one generated by `context.WithCancel`, as you should use the `CancelFunc` returned from `WithCancel` as soon as you no longer wish to receive data from the `output` channel.

If `id` doesn't contain the ID of a job that is currently running or has run in the past, the function will return an error.

`TailJob` expects to be the one to close the `output` channel. Once `TailJob` has read and sent all lines from a job, it closes the channel. This means that as long as the job is running, the channel stays open. If it is closed elsewhere, `TailJob` *will* panic and throw an error &#x2013; cause that's what happens when you try to write to a closed channel in Go!

`OutputLine` is a struct that contains each line of output from a job, that may contain additional metadata such as timestamps or log type.


##### Storing Job Output

As part of launching a job, we are able to set the `Stdout` and `Stderr` of a `exec.Cmd` to any `io.Writer` of our choosing. This will be used to capture the output of a job and store it in memory while the job is running.

For this challenge, that's where storing the output stops &#x2013; it'll just stay in memory, and will be lost once the `workernator` binary is stopped.

For a real-world service, we'd have to look into flushing the output to a file on disk once a job is complete. There would also have to be a way to keep that output in-memory for a short period of time, to account for other clients potentially asking for the same output log without ballooning the amount of memory being used. While this does mean that job info is lost when the service shuts down, doing anything more is out of scope for this exercise.


##### Concurrency

The library will support multiple processes requesting the output of a single job at once. When a process calls the `TailJob` function, it will first send all of the `OutputLine` objects stored in the buffer before then switching over to "live" messages.

There may be some trickiness in managing that switch-over, but this will definitely be covered by unit tests to ensure it's working properly.


### API

The API is going to use GRPC rather than HTTP, as set out in the challenge requirements.


#### GRPC API Definition

We're not going to go over the entire protobuf definition here, instead we'll cover some of the design decisions so we're all on the same page. However, please do check out [workernator.proto](file:///proto/workernator.proto) to see the entire protobuf definition.


##### Job Type

As part of the definition of a job, each job has a 'type'. This type defines what the job does, as well as what arguments it expects.

In addition to the three pre-defined jobs ( "Fibonacci", "Expression Evaluator", and "Wait Then Send" ), there is also a '0-th' job type: `Noop`. This is because in Go, the default value for a variable with type `JobType` is 0. Rather than have this be the value for an "actual" job, instead this is assigned to a job that does nothing and doesn't print anything. This way a configuration error, programmer flub, or simple clumsy-fingered mistake won't start the wrong job.


##### Job Request Messages

There are two potential messages that each of `Stop`, `Status`, and `Tail` could have used:

-   a generic `Id` message that simply contained the job ID, OR
-   a method-specific message that contains the job ID

The first variation is a bit nicer; instead of three different message types that contain the same data you just have one. So you'd get this:

```protobuf
service Service {
  rpc Start(JobStartRequest) returns (Job){}
  rpc Stop(JobId) returns (Job){}
  rpc Status(JobId) returns (Job) {}
  rpc Tail(JobId) returns (stream TailJobResponse){}
}
```

However, there is a somewhat large drawback to this.

For example, what happens if we want to add a timeout field to the request we send to `Stop`? Or if we want `Status` to additionally return all of the current log lines for the job? Maybe we want to be able to have `Tail` only start from the most recent message and then continue from there &#x2013; or to only show the last N log lines.

Each of these would require one of two things. Either the `JobId` message gets overloaded to the point of being nearly useless &#x2013; or each method gets its own message type.

I decided to just go with each method getting it's own type. It might be redundant right now, but it gives each method control over what it accepts without causing breaking API changes later on down the line. Also, I can already see lots of potential functionality requiring expanding each of the request messages for `Stop`, `Status`, and `Tail`.


##### The "Arguments" Message Type

Not a lot to say about this one, but just in case you were curious: this message type is here so that there's no chance that the `args` field in the `Job` message type and the `args` field in the `JobStartRequest` message type accidentally start diverging.


##### Separate Folders

This one is mostly a personal preference thing, but I prefer to keep the protobuf definition files separate from the code generated from those files. This is mostly so that if there's a need to generate code for other languages that there's already a clear pattern as to how that should work and where files should go.

Additionally, I prefer to keep the Go code generated from the protobuf definition in `internal`. The main reason is that if there is a need for outside developers ( either external to my team or external to the company ) need to build their own clients I'd rather give them a more thoughtfully designed API than what GRPC usually generates.

Also, this ensures that things like mTLS don't get forgotten. This is because I'm able to design the client SDK/API/whatever so that stuff like "provide a client mTLS certificate here" really explicit and hard to miss.

Lastly, it also allows me to wrap some of the GRPC weirdness in a more "Go-like" wrapper. The best example is streaming API methods that can be presented as a method on the client struct that takes or produces a channel. For example, using the streaming API you can write methods that allow you to upload files &#x2013; but it's much nicer to provide an API client that fulfills the `io.Writer`, `io.Reader`, and `io.Closer` interfaces so that your API slots seamlessly into code that already uses those interfaces.


#### Authentication

The GRPC service will use mTLS for authentication. A unique certificate will be generated for each client.

The server and client libraries will be configured to use TLS v1.3. Starting in Go 1.17, when TLS v1.3 is chosen while configuring TLS you are not able to select the cipher suite. [This is because this decision isn't easy, and many devs often get it wrong &#x2013; so the Go team has gone with a sensible & secure default.](https://tip.golang.org/blog/tls-cipher-suites)

The exact suite of ciphers used may depend on what hardware is available at runtime, as certain ciphers are only used when *both* sides have the appropriate hardware &#x2013; see AES-GCM, for example.


#### Authorization

Rather than using JWT or something else to authorize users, instead we'll use some of the features of TLS!

One of the things that can be configured while generating a TLS certificate is the 'distinguished names', or subjects. These are things like country, state or province, locality ( ie, city ) &#x2013; as well as organization & common name. These values are usually used to verify that a TLS certificate is the right one for the site you're navigating to; your browser checks the common name to see if it matches the domain you're on.

However, we can use it for other things; things like authorization!

The client certificate that is generated will contain a few subjects with slightly different meanings.

Below is each subject key, the 'proper' name, and what we're using it for ( if we're using it differently than the name would suggest ).


##### Keys


###### Organization Name

**Key:** O

Using this basically as intended, putting 'Teleport' as the value.


###### Organizational Unit Name

**Key:** OU

I'm defaulting to `workernator`, with the idea that this field could be used for the name of the service the certificate is meant to be used with.


###### Common Name

**Key:** CN

Typically used for the name of the person "responsible" for the TLS certificate on the server, we're using it to identify whether the certificate is meant to be used by a server or a client. Handy for when things get mis-named and you forget which is which! It also means that users can't set up their own server if they get their hands on the code; they still need a proper 'server' certificate.


###### Locality Name

**Key:** L

This is normally used to name the city or local region where the server or server admin is located.

Here we're going to use it to identify the user making a request. This will be used to look up what permissions and abilities the user has.


##### Usage

The **O**, **ON**, and **CN** keys are the "core" keys, and should be present regardless of whether the certificate is meant to be used by a server or a client. Both clients and servers will use those three keys when validating a certificate.

As for the **L** key, only the servers will pay attention and use that key. Clients will ignore this key if it's in a server certificate. This opens up the possibility of using the **L** key for something else later, but that is outside the scope of this project so we're just going to leave it at that.

For now the list of users and their permissions will be hard-coded into the server. There are packages like `viper` we could use to manage configurations, but it's outside the scope of this exercise.


### Command-Line Client

The client is going to be built using [cobra](https://cobra.dev/).

If called with no arguments, it will print out some basic information and some usage hints:

```
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
```

From here on, the output of the command will be truncated for clarity and comprehension.


#### Starting, Stopping, & Getting Job Status

All the sub-commands for managing jobs are available under 'jobs':

```
--- truncated ---
Usage:
  workernator jobs [command]

Available Commands:
  start       Start a job in the server
  status      Get the status of a job
  stop        Stop a running job
  tail        View the output of a command
--- truncated ---
```

The client knows what jobs can be run, and will list them when you call `workernator jobs start` without any further arguments:

```
--- truncated ---
Available Commands:
  eval        Evaluate a mathmatical formula
  fib         Calculate the value of a position in the Fibonacci sequence
  noop        A job that does nothing
  wait        Wait for a set number of seconds before sending an empty HTTP POST request to a URL
--- truncated ---  
```

Each job has it's own arguments, and `workernator` will let you know what's required if you run `workernator jobs start <name>` without any further arguments, or if you use `workernator jobs start <name> --help` to view the built-in help docs.

Once you've filled out all the required arguments, if the job is started successfully the ID of the newly created job will be printed out before the command exits:

```
$ workernator jobs start fib 3
Contacting server...
Starting job...

Job started, ID is 'XE38YM'

```

This ID can then be used to get the status of a job:

```
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

```

This is the same way stopping a job works:

```
$ workernator jobs stop XE38YM
Contacting server...
Stopping job XE38YM...

Done, job stopped.

```

As a note, if the job has already finished, the `stop` command will still report the job is stopped &#x2013; no complaints about "job already completed" or anything. The `stop` and `status` commands ( and the `tail` command ) will only return an error if the ID given doesn't match the ID of a running or completed job.

Tailing output is also as simple as getting the status or stopping a job:

```
$ workernator jobs tail XE38YM
2022-07-07 12:34:54 [JOB] Starting job 'Fibonacci'
2022-07-07 12:34:55 [FIB] Calculating the value of the 3rd value in the Fibonacci sequence
2022-07-07 12:34:55 [FIB] Using lookup; value is '1'.
2022-07-07 12:34:55 [RESULT] 1
2022-07-07 12:34:56 [JOB] Complete

Job finished, no more output, exiting tail!

```

As you can see, once a job has stopped `workernator` will exit.


### Security