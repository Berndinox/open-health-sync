// SPDX-License-Identifier: AGPL-3.0-only

package main

import "io"

// runUp is the container entrypoint: `ohs migrate up && ohs serve`. The runtime
// image is distroless and has no shell to chain the two commands, so the binary
// does it itself. Extra arguments go to serve.
func runUp(args []string, out io.Writer) error {
	return up(args,
		func() error { return run([]string{"migrate", "up"}, out) },
		func(a []string) error { return run(append([]string{"serve"}, a...), out) })
}

// up runs migrate, and serve only when migrate succeeded.
func up(args []string, migrate func() error, serve func([]string) error) error {
	if err := migrate(); err != nil {
		return err
	}
	return serve(args)
}
