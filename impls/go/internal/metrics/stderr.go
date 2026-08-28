package metrics

import "os"

// stderrFile returns os.Stderr — split into its own file so tests can
// swap writeStderr without pulling os into the main file.
func stderrFile() *os.File { return os.Stderr }
