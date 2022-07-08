# Workernator

Very very simple worker manager built in Go.

#### Available Jobs

Workernator ships with a number of built-in jobs; these are mostly to show off
how to write jobs and register them.

The jobs are:

**Fibonacci**

Takes one argument: the Fibonacci number to compute. So an argument of 1 would
produce a result of 0, an argument of 2 would produce 1, etc.

**Expression Evaluator**

Takes minimum one argument, and potentially more.

The first argument is the expression to evaluate -- this is a mathmatical
formula that you wish to have computed. For example, you could send `1 + 2`, `4
/ 2`, `2.3 * 1.2`, or `3 - 1`. Additionally, you can also include variables,
such as `2x * 3 + y`. However, when you put variables into an expression to get
evaluated, you must include the values of those variables in your request.

**Wait Then Send**

This takes two arguments:

 - `url_to_post`, which must be a valid HTTP URL reachable from workernator, and
 - `wait_seconds`, which is how long the worker will wait before sending a bare
   HTTP POST request to the URL in `url_to_post`


