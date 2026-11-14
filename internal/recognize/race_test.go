//go:build race

package recognize

// raceEnabled tells tests the race detector is on: it makes code many
// times slower, by how much depends on the machine's load, so time budgets
// are not checked.
const raceEnabled = true
