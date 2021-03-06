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

First thing: this library is going to be designed with the idea that what jobs can be executed ( including any required arguments ) will be part of the API of the library & service. This is mostly for type-checking, but for a few other reasons too:

-   no JSON! Part of this is a web-facing service, use GRPC instead of stuffing JSON into string fields in the messages!
-   API is hopefully easier to use, as it's closer to being truly self-documenting

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

-   a way to **start** a job
-   a way to **stop** a job
-   a way to get the **status** of a job
-   a way to **tail the output** of a job


#### Starting Jobs

Something like the following function will be provided so users can start a job and provide any required arguments:

```go
StartJob(name string, args JobData) (JobInfo,error)
```

The `JobData` type will most likely be a simple-data-object struct that contains the arguments for the job. The `JobInfo` struct that gets returned will have some information about the job such as the ID that's required to stop the job, get it's status, or tail the output. An error will be returned only if the data in `args` contains an invalid job, or incorrect arguments for the job.

The ID for a job will be generated using [xid](https://github.com/rs/xid). From the \`xid\` README:

> Xid uses the Mongo Object ID algorithm to generate globally unique ids with a different serialization (base64) to make it shorter when transported as a string: <https://docs.mongodb.org/manual/reference/object-id/>
> 
> -   4-byte value representing the seconds since the Unix epoch,
> -   3-byte machine identifier,
> -   2-byte process id, and
> -   3-byte counter, starting with a random value.
> 
> The binary representation of the id is compatible with Mongo 12 bytes Object IDs. The string representation is using base32 hex (w/o padding) for better space efficiency when stored in that form (20 bytes). The hex variant of base32 is used to retain the sortable property of the id.

When starting a job, the library does more than just launch a goroutine and call it a day.

Using the namespaces & cgroups built into modern Linux kernels, we're able to build something similar to a Docker container that the job runs inside. This is accomplished using the methods detailed in [this series of articles](https://medium.com/@teddyking/linux-namespaces-850489d3ccf) and also in [this article](https://www.infoq.com/articles/build-a-container-golang/).

Basically, this method boils down to using the special file `/proc/self/exe` which is a special link that points to the currently running binary. By using `exec.Command` from the [exec package](https://pkg.go.dev/os/exec) we can re-run the `workernator` binary with special arguments that tell the OS to run the binary in a new namespace for resource isolation. This also allows us to configure cgroups for resource management.


##### Namespace & CGroups Closer Look

When using `/proc/self/exe` to launch our subprocess, we can modify the `*exec.Cmd` we get from calling `exec.Command("/proc/self/exe")`. This is how we tell Linux what new namespaces the process should use.

These are the namespaces that will be used:

-   `CLONE_NEWUTS` - isolate the host & domain name
-   `CLONE_NEWPID` - isolate the PID number space for our jobs
-   `CLONE_NEWNS` - isolate our filesystem ( takes effect once we use `PivotRoot` )
-   `CLONE_NEWNET` - isolate our networking
-   `CLONE_NEWUSER` - isolate the UID/GID number spaces

As for cgroups, we'll be using cgroups version 2, with the following cgroups & settings:

-   `pids`
    -   `max`: 10
    -   `notify_on_release`: 1
-   `cpu`
    -   `max`: 200000 1000000 (1/5 of each second)
-   `memory`
    -   `limit_in_bites`: 10485760 (10M)
-   `io`
    -   `max`: 10485760 (10M)


#### Stopping Jobs

Using the `exec.Cmd` pointer that was created in the process of starting a job, we can use `exec.Cmd.Process.Kill()` to force the job to stop. The job/worker runner code will also be set up to capture the signal used to kill it and ensure any child processes are terminated before exiting.

However, like the other library methods, the implementation details are hidden from the world at large behind this function:

```go
StopJob(id string) (*JobInfo, error)
```

Job IDs are returned by `StartJob` method; it returns `JobInfo` struct that will contain the ID for the job that was started.

If the `id` contains the ID of a current or past job in `workernator`, it will attempt to stop that job. If the ID doesn't map to such a job, the function will return an error.

This function is idempotent, if `StopJob` is called with the ID of a job that has already been stopped, the function will simply return the `JobInfo` pointer.


#### Querying Job Status

The library will provide the following function:

```go
JobStatus(id string) (*JobInfo, error)  
```

Job IDs are returned by `StartJob` method; it returns `JobInfo` struct that will contain the ID for the job that was started.

If the `id` parameter contains the ID of a current or past job, the function will return the `JobInfo` for that job. Otherwise it will return an error.

If a job has failed to complete, the `error_msg` field in the `JobInfo` struct will contain the error message from the job.


#### Get Job Output

An important part of running a job is being able to get the output of the job. Because the jobs can output binary data, we have to design this API to be easy to use but allow for the flexibility of the user getting plain text **or** binary data.

The library will provide a function that allows clients to get the output data of a running or completed job:

```go
TailJob(ctx context.Context, id string) (io.Reader, error)
```

Job IDs are returned by `StartJob` method; it returns `JobInfo` struct that will contain the ID for the job that was started.

The struct returned by this method will fulfill the `io.Reader` interface. So long as the job is still running calling `Read` on the reader will either return data or block until there is data to return. Behind the scenes this is because the `io.Reader`-fulfilling struct will have a channel it's reading the output from; if a job hasn't output anything there won't be any data in the channel to read.

Once all the data has been read from the `io.Reader`, the next call will return `io.EOF`. If the job encounters an error while it's running the call to `Read` will return an error describing what occurred.

If a job is complete, one of two things will happen when `TailJob` is called:

-   If the job **completed** successfully, a call to `TailJob` will return an `io.Reader` and a nil `error`. The `io.Reader` can then be used to get the entire output of the job the same way you would with a file or data from a network request by calling `Read([]byte)`. That call will return an `io.EOF` once it's reached the end of the output.

-   If the job **didn't** **complete** successfully, `TailJob` will return a nil `io.Reader` and a non-nil `error` that gives some insight into what happened.

If `id` doesn't contain the ID of a job that is currently running or has run in the past, the function will return an error.

The library will support multiple processes requesting the output of a single job at once. A client connecting shouldn't interrupt or cause issues for any other connected client.


##### Where Is Job Output Saved?

Storing the output of the job so it can be sent to clients later is an important piece of this library. There are a few things to be concerned about, such as:

-   very verbose jobs or long-running jobs ( or both ) that generate **lots** of data
-   sending the output to multiple clients, including slow or misconfigured clients
-   handling going from old output to new output seamlessly

Rather than storing the output in memory, we'll be saving the output of a job to a file. This will provide a number of benefits:

-   jobs are now bound by disk space instead of how much RAM they have
-   each client will get its own file handle, so a client reading slowly won't impact other clients
-   using a package like [nxadm/tail](https://pkg.go.dev/github.com/nxadm/tail#section-documentation) the library can read the output and wait for new lines


##### Potential Issues

There are two main potential issues I can foresee:

-   jobs that run forever could potentially fill up the filesystem
-   too many clients could cause the OS to return a 'too many open file handles' error


###### Filling Up The Filesystem

This issue is one that could be solved with a `tmpfs` or `ramfs` file system. Both are temporary file systems that can be created with a limited amount of size. `tmpfs` is newer, and would also allow us to set a limit so that it behaves the same as a physical disk with limited space.

*However*, managing the creation, mounting, un-mounting, and moving data to & from a `tmpfs` is outside the scope of this challenge. So is any kind of watchdog to kill jobs that start output too much data. For now, managing out-of-control jobs is left up to the admin of the system.


###### Concurrency & File Handles

There are probably a few ways that this problem could be handled, but the ones that come to mind feel like they're out of scope for this challenge.

One potential solution would be to create a kind of 'buffer manager' class that could be given a file handle when creating an instance, and would internally buffer the file. This buffer would be used as a simple FIFO queue that fans out to the connected clients. However, once enough clients have reached the end of the buffer the instance would start returning an error when the system would attempt to call the function to add another reader.

This is because after a certain point the instance would have to start discarding the oldest data from the buffer so it can read in new data from the file. Once the first parts of a file have been discarded, new clients wouldn't be able to be sent that data from the buffer. So after a certain point this 'buffer manager' instance would have to stop accepting new reader clients, at which point the system would just create a new 'buffer manager' and start using that for new clients.

Alternatively, there could be a configurable cutoff for output size. This cutoff would be a size in bytes; any output files smaller than this buffer could just be kept in memory, larger files would always be read from disk.

These are just two potential solutions for dealing with too many clients connecting to the service to get the output. However, the maximum number of open files in Linux is configurable &#x2013; on my system the default reported by \`ulimit -Hn\` is 524288. For this challenge, that feels like plenty of open files!


### API

The API is going to use GRPC rather than HTTP, as set out in the challenge requirements.


#### GRPC API Definition

We're not going to go over the entire protobuf definition here, instead we'll cover some of the design decisions so we're all on the same page. However, please do check out [workernator.proto](../proto/workernator.proto) to see the entire protobuf definition.


##### Job Request Messages

There are two potential messages that each of `Stop`, `Status`, and `Tail` could have used:

-   a generic `Id` message that simply contained the job ID, OR
-   a method-specific message that contains the job ID

The first variation is a bit nicer; instead of three different message types that contain the same data you just have one:

```protobuf
service Service {
  rpc Start(JobStartRequest) returns (Job){}
  rpc Stop(JobId) returns (Job){}
  rpc Status(JobId) returns (Job) {}
  rpc Tail(JobId) returns (stream TailJobResponse){}
}
```

The second variation looks like this:

```protobuf
service Service {
  rpc Start(JobStartRequest) returns (Job){}
  rpc Stop(JobStopRequest) returns (Job){}
  rpc Status(JobStatusRequest) returns (Job) {}
  rpc Tail(JobTailRequest) returns (stream TailJobResponse){}
}
```

There is a downside to the first version: we'd always be risking backwards compatibility.

Take a look at the following potential feature requests we could get for this service:

-   add a timeout field to the message sent to `Stop`, so that users can define a grace period before the job is killed
-   add a way to stop jobs that have been running for longer than N seconds
-   add a flag to the message sent to `Status` that includes all current log lines
-   add a boolean flag to the message sent to `Status` that controls the verbosity of what's returned ( optionally showing things like memory usage, bytes sent/received over the network, etc )
-   add a flag to the message sent to `Tail` so it doesn't output past messages, just new ones
-   add a flag to the message sent to `Tail` so it only prints the last N lines of output before continuing with live messages
-   allow `Tail` to return log messages by job *type*, instead of just specific jobs

Each of these would require one of two things. Either the `JobId` message gets overloaded to the point of being nearly useless &#x2013; or each method gets its own message type. Switching the message type a method takes is not backwards compatible in GRPC, but changing the fields of a message is.

I decided that instead of overloading a `JobId` message type with fields specific to each of the routes, we'll start with each API method having it's own unique argument message type.


##### Separate Folders

This one is mostly a personal preference thing, but I prefer to keep the protobuf definition files separate from the code generated from those files. This is mostly so that if there's a need to generate code for other languages that there's already a clear pattern as to how that should work and where files should go.

Additionally, I prefer to keep the Go code generated from the protobuf definition in `internal`. The main reason is that if there is a need for outside developers ( either external to my team or external to the company ) need to build their own clients I'd rather give them a more thoughtfully designed API than what GRPC usually generates.

Also, this ensures that things like mTLS don't get forgotten. This is because I'm able to design the client SDK/API/whatever so that stuff like "provide a client mTLS certificate here" really explicit and hard to miss. Also, it allows us to wrap any potentially "generic" error messages with ones that are hopefully more useful.

Lastly, it also allows me to wrap some of the GRPC-specific weirdness in a more "Go-like" wrapper. For example, take a GRPC service that defines a method like this:

```protobuf
DownloadFile(stream FilePart) returns (FileInfo) {}
```

If we just export the GRPC-generated code, this is what users would have to do in order to use our 'download file' method:

```go
// create handle for output file
f, err := os.OpenFile(OUTPUT_PATH, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
if err != nil {
  log.Fatalf("unable to open output file: %v", err)
}

// dial the grpc server
conn, err := grpc.Dial(":50005", grpc.WithCredentialsBundle(bundle))
if err != nil {
  log.Fatalf("can not connect with server %v", err)
}

// create stream
client := pb.NewStreamServiceClient(conn)
in := &pb.Request{Id: 1}
stream, err := client.DownloadFile(ctx, in)
if err != nil {
  log.Fatalf("open stream error %v", err)
}

// read all the bytes
for {
  resp, err := stream.Recv()
  if err == io.EOF {
    break
  }
  if err != nil {
    log.Fatalf("cannot receive %v", err)
  }

  _, err = f.Write(resp.FileBytes)
  if err != nil {
    log.Fatalf("unable to write to file: %v", err)
  }
}
```

But isn't this much nicer?

```go
// create handle for output file
f, err := os.OpenFile(OUTPUT_PATH, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
if err != nil {
  log.Fatalf("unable to open output file: %v", err)
}

// create client, pointed at the right server
client, err := file_client.New(":50005")
if err != nil {
  log.Fatalf("unable to create file upload client: %v", err)
}

// download file ID 1 to our file
err = client.DownloadTo(ctx, f, 1)
if err != nil {
  log.Fatalf("unable to download file: %v", err)
}
```


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

| Key | Name                     | Using For                                                                                                                                                                                                                                       |
|--- |------------------------ |----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| O   | Organization Name        | Using this basically as intended, putting 'Teleport' as the value.                                                                                                                                                                              |
| OU  | Organizational Unit Name | This will be either 'server' or 'client', to identify who should use the certificate. This way users can't set up their own server if they get their hands on the code; they still need a proper 'server' certificate.                          |
| CN  | Common Name              | Typically used for the host + domain name for a service, we'll be using it to store the name of the service the certificate can be used with.                                                                                                   |
| L   | Locality Name            | This is normally used to name the city or local region where the server or server admin is located. Here we're going to use it to identify the user making a request. This will be used to look up what permissions and abilities the user has. |

The **O**, **ON**, and **CN** keys are the "core" keys, and should be present regardless of whether the certificate is meant to be used by a server or a client. Both clients and servers will use those three keys when validating a certificate.

As for the **L** key, only the servers will pay attention and use that key. Clients will ignore this key if it's in a server certificate. This opens up the possibility of using the **L** key for something else later, but that is outside the scope of this project so we're just going to leave it at that.

For now the list of users and their permissions will be hard-coded into the server. There are packages like `viper` we could use to manage configurations, but this is also outside the scope of this exercise so we won't be doing it.

On the server, a few map data structure will be used to define our permission structure:

```go
type permission int32

const (
  none permission = iota 
  own 
  super 
)

type rpcPermissions map[string]permission

type userPermissions map[string]rpcPermissions

var superUser = rpcPermissions{
  "start": super,
  "stop": super,
  "status": super,
  "output": super,
}

var statusReader = rpcPermissions{
  "status": super,
}

var outputReader = rpcPermisisons{
  "output": super,
}

var permissionConfig = userPermissions{}
```

Starting from the top, we have our `permission` type. Each of the constants that use this type have a specific meaning; `none` means "not allowed to use this RPC", `own` means "can use this RPC but only to interact with their own jobs", and `super` means "can use this RPC to interact with *any* job".

Next up is `rpcPermissions`, which maps RPC names to a permission. The RPC names are down-cased when comparing, so 'start', 'Start', & 'sTaRt' are all equivalent. So something like `"stop": own` would mean the user can only stop their **own** jobs, `"stop": super` would allow the user to stop **any** job.

After that we've got `userPermissions`, which uses a string key that represents the user name to give each user their set of permissions.


##### Auth Example

For example, say we have the following users we want to set up:

-   `admin`, who can do anything, to any job
-   `alice`, who can start jobs and get their status
-   `bob`, who can start jobs and get the output of them
-   `charlie`, who can see the status of any job

The first step would be to create the TLS certificates for each user, setting the `L` key in each to the username.

On the server side, we'd configure the authorization like so:

```go
var permissionConfig = userPermissions{
  "admin": superUser, // use our pre-configured 'super user' value
  "alice": rpcPermissions{
    "start": own,
    "status": own,
  },
  "bob": rpcPermissions{
    "start": own, // least amount of trust possible, even if right now 'start' only starts a job
    "output": own,
  },
  "charlie": rpcPermissions{
    "status": super,
  },
}
```

Then in a GRPC interceptor, the data from the TLS certificate ( after the certificate is validated, of course ) will be checked against that ACL. If the user has the proper permissions, the interceptor will pass the request along the chain. However, if they don't have the correct permissions the interceptor will return a [PermissionDenied](https://pkg.go.dev/google.golang.org/grpc/codes) error code.


#### Running The API

The service that provides this API will be a single binary that can be started from the command line.

It will have, at minimum, the following flags that will be used to configure aspects of the service:

-   a `--debug` flag, that will turn on the outputting of debug messages
-   a `--host` flag, that tells the service what address to bind to
-   a `--port` flag, that tells the service what port to bind to
-   a `--hostCert` flag, that tells the service the path to the TLS certificate it should use for the service
-   a `--hostKey` flag, that tells the service the pat to the TLS key it should use for the service
-   a `--rootCert` flag, that tells the service the path to the TLS CA Root certificate that was used to sign the client certificates.

**Important Note!**

Without a properly configured root certificate, no clients will be able to connect!


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