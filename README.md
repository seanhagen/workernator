# Workernator

Very very simple worker manager built in Go.

Runs arbitrary Linux processes as jobs, where the output of the job is captured
and saved to a file. This output can be streamed to clients as binary data,
allowing jobs to output whatever they want as output.

