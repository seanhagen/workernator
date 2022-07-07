---
authors: Sean Hagen (sean.hagen@gmail.com)
state: draft
---

# RFD 78 - Workernator

## From Challenge 
Library

    Worker library with methods to start/stop/query status and get the output of a job.
    Library should be able to stream the output of a running job.
        Output should be from start of process execution.
        Multiple concurrent clients should be supported.
    Add resource control for CPU, Memory and Disk IO per job using cgroups.
    Add resource isolation for using PID, mount, and networking namespaces.

API

    GRPC API to start/stop/get status/stream output of a running process.
    Use mTLS authentication and verify client certificate. Set up strong set of cipher suites for TLS and good crypto setup for certificates. Do not use any other authentication protocols on top of mTLS.
    Use a simple authorization scheme.

Client

    CLI should be able to connect to worker service and start, stop, get status, and stream output of a job.


## What 

A simple, bare-bones worker manager, with a core library, GRPC API, and CLI client.

## Why 

To have a worker manager that can use cGroups & namespaces to manage compute
resources and namespaces for PID, networking, & mount isolation.

## Details 

### Library

### API 

### CLI Client 

 - a few examples of what it's like to use the command; starting, stoping,
   querying status, and tailing the output

### Security 

### UX
