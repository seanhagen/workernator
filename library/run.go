package workernator

import "context"

//  when calling (*Job).Run()':
//     - setup new network namespace [^1]
//     - setup container network inetrface ( 2 parts? )
//     - run /proc/self/exe in 'child-mode' ( or whatever we call 'getting inside the container
//       so we can run the command')
//     - prep an *exec.Cmd that's the /actual/ command we're going to run
//     - setup networking ( set hostname, join namespace from ^1 above )
//     - setup cgroups ( create cgroups, then set limits )
//     - copy nameserver ( so dns works )
//     - chroot & chdir '/'
//     - mount special dirs (/dev, /proc, etc)
//     - setup local loopback net interface
//     - RUN THE COMMAND
//

// Run ...
func (c *Job) Run(ctx context.Context) error {
	if err := c.Start(ctx); err != nil {
		return err
	}

	if err := c.Wait(ctx); err != nil {
		return err
	}

	if err := c.Stop(ctx); err != nil {
		return err
	}

	return nil
}

// Run ...
func (c *Job) Start(ctx context.Context) error {
	err := c.cmd.Start()
	// use := c.cmd.ProcessState.SysUsage()
	// spew.Dump(use)
	return err
}

//  when calling '(*Job).Stop()':
//     - unmount directories
//     - umount network namespace
//     - umount container namespace
//     - remove cgroups
//     - remove any remaining container files/data

// Stop ...
func (c *Job) Stop(ctx context.Context) error {
	return c.cmd.Process.Kill()
}

// Wait ...
func (c *Job) Wait(ctx context.Context) error {
	return c.cmd.Wait()
}
